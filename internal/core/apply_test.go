package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/core"
	"github.com/ushineko/enshroudedbonker/internal/steam"
)

func enable(t *testing.T, e *env, names ...string) {
	t.Helper()
	_, err := core.SetModsEnabled(context.Background(), core.SetModsEnabledRequest{Request: e.req, Names: names, Enabled: true})
	require.NoError(t, err)
}

func status(t *testing.T, e *env) core.StatusResult {
	t.Helper()
	st, err := core.Status(context.Background(), core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	return st
}

func apply(t *testing.T, e *env) core.ApplyResult {
	t.Helper()
	res, err := core.Apply(context.Background(), core.ApplyRequest{Request: e.req})
	require.NoError(t, err)
	return res
}

// underSteam moves the install into a Steam library, so it has a manifest, as a
// player's does; --game-dir has none.
func underSteam(t *testing.T, e *env) {
	t.Helper()
	root := filepath.Join(e.root, "steam")
	dir := filepath.Join(root, "steamapps", "common", "Enshrouded")
	require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o750))
	require.NoError(t, os.Rename(e.game, dir))
	e.game = dir
	e.req.GameDir = ""
	t.Setenv("STEAM_ROOT", root)
	vdf := "\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t\"" + root +
		"\"\n\t\t\"apps\"\n\t\t{\n\t\t\t\"" + steam.AppID + "\"\t\t\"1\"\n\t\t}\n\t}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(vdf), 0o600))
	writeManifest(t, e, time.Now().Add(-48*time.Hour))
}

func writeManifest(t *testing.T, e *env, updated time.Time) {
	t.Helper()
	acf := "\"AppState\"\n{\n\t\"appid\"\t\t\"" + steam.AppID + "\"\n\t\"name\"\t\t\"Enshrouded\"\n" +
		"\t\"installdir\"\t\t\"Enshrouded\"\n\t\"StateFlags\"\t\t\"4\"\n\t\"LastUpdated\"\t\t\"" +
		strconv.FormatInt(updated.Unix(), 10) + "\"\n\t\"buildid\"\t\t\"1000001\"\n}\n"
	p := filepath.Join(filepath.Dir(filepath.Dir(e.game)), "appmanifest_"+steam.AppID+".acf")
	require.NoError(t, os.WriteFile(p, []byte(acf), 0o600))
}

// steamUpdate does what Steam did to build 1287369: a new executable and index,
// the resource file and the containers left where they were.
func steamUpdate(t *testing.T, e *env) {
	t.Helper()
	const newBuild = "7654322|^/game99/branches/test_branch|2026-02-01T00:00:00.000000Z"
	const commit = "aaaaaaa876543210fedcba9876543210fedcba98"
	for name, body := range map[string]string{
		steam.ExeName: "MZ\x90\x00synthetic\x00" + commit + "\x00\x00\x000.0.2.0\x00tail",
		steam.KfcName: "KFC2\x00\x00" + newBuild + "\x00new-index",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(e.game, name), []byte(body), 0o644)) //nolint:gosec // fixture
	}
	writeManifest(t, e, time.Now())
}

func TestApply_FromNothingToDeployed(t *testing.T) {
	e := newEnv(t)
	vanilla := gameHashes(t, e.game)
	enable(t, e, "big_stacks", "fast_travel_markers")
	assert.Equal(t, core.ModsNotApplied, status(t, e).Mods.State)

	res := apply(t, e)

	assert.ElementsMatch(t, []string{"big_stacks", "fast_travel_markers"}, res.Deployed.Mods)
	assert.Empty(t, res.Skipped)
	assert.NotEqual(t, vanilla, gameHashes(t, e.game))
	st := status(t, e)
	assert.Equal(t, build.StateDeployed, st.State)
	assert.True(t, st.Pristine.Present, "the vanilla copy was captured")
	assert.Equal(t, compat.Derived, st.Gate.Verdict, "the compatibility check ran")
	assert.Equal(t, core.ModsApplied, st.Mods.State)
	assert.Equal(t, "Your 2 mods are in the game.", st.Mods.Sentence)
}

func TestApply_GetsEmmWhenItIsMissing(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	vanilla := gameHashes(t, e.game)
	_, err := core.ConfigSet(context.Background(), core.ConfigSetRequest{Request: e.req, Key: "emm_path", Value: ""})
	require.NoError(t, err)
	asked := errors.New("the download was asked for")
	core.StubGetEmm(t, func() error { return asked })

	_, err = core.Apply(context.Background(), core.ApplyRequest{Request: e.req})

	require.ErrorIs(t, err, asked)
	assert.Equal(t, vanilla, gameHashes(t, e.game))
}

func TestApply_WhenUpToDateSkipsPreparationAndStillDeploys(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	first := apply(t, e)
	prepared := filepath.Join(e.ws, "pristine")
	before := gameHashes(t, firstDir(t, prepared))
	compatBefore := fileTime(t, firstDir(t, filepath.Join(e.ws, "compat")))
	pristineBefore := fileTime(t, filepath.Join(firstDir(t, prepared), "pristine.json"))
	time.Sleep(10 * time.Millisecond)
	// Any download would fail this test.
	core.StubGetEmm(t, func() error { t.Error("emm is installed; nothing to download"); return nil })

	second := apply(t, e)

	assert.True(t, second.Deployed.When.After(first.Deployed.When), "it deploys again")
	assert.Equal(t, before, gameHashes(t, firstDir(t, prepared)))
	assert.Equal(t, pristineBefore, fileTime(t, filepath.Join(firstDir(t, prepared), "pristine.json")), "no second capture")
	assert.Equal(t, compatBefore, fileTime(t, firstDir(t, filepath.Join(e.ws, "compat"))), "no second check")
}

func TestApply_RefusesAtOnceWhileTheGameRuns(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	vanilla := gameHashes(t, e.game)
	e.running = true

	_, err := core.Apply(context.Background(), core.ApplyRequest{Request: e.req})

	require.ErrorIs(t, err, build.ErrGameRunning)
	assert.Equal(t, vanilla, gameHashes(t, e.game))
	assert.NoDirExists(t, filepath.Join(e.ws, "pristine"), "it stopped before doing any work")
}

func TestApply_SkipsABrokenTweakAndAppliesTheRest(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks", "fast_travel_markers")
	t.Setenv("FAKE_EMM_SILENT", "big_stacks")

	res := apply(t, e)

	assert.Equal(t, []string{"big_stacks"}, res.Skipped)
	assert.Equal(t, []string{"fast_travel_markers"}, res.Deployed.Mods)
	st := status(t, e)
	assert.Equal(t, core.ModsApplied, st.Mods.State, "a tweak this build cannot run is not a settings change")
	assert.Equal(t, []string{"big_stacks"}, st.Mods.Skipped)
	assert.Equal(t, "1 of 2 mods are in the game. big_stacks does not work on this game build.", st.Mods.Sentence)
}

func TestApply_StopsWhenEveryEnabledModIsBroken(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	t.Setenv("FAKE_EMM_SILENT", "big_stacks")
	vanilla := gameHashes(t, e.game)

	_, err := core.Apply(context.Background(), core.ApplyRequest{Request: e.req})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "big_stacks")
	assert.Equal(t, vanilla, gameHashes(t, e.game))
}

func TestApply_AFailingLibraryModStopsBeforeTheGameIsWritten(t *testing.T) {
	e := newEnv(t)
	src := filepath.Join(e.root, "src-mod")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "src"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(src, "mod.json"), []byte(`{"id":"third_party","name":"Third party",`+
		`"version":"1.0.0","capabilities":["patch"],"authors":["tester"],"license":"MIT"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "src", "mod.lua"), []byte(`print("x")`), 0o600))
	_, err := core.ImportMod(context.Background(), core.ImportModRequest{Request: e.req, Path: src})
	require.NoError(t, err)
	enable(t, e, "big_stacks", "third_party")
	t.Setenv("FAKE_EMM_FAIL_WITH", "third_party")
	vanilla := gameHashes(t, e.game)

	_, err = core.Apply(context.Background(), core.ApplyRequest{Request: e.req})

	require.Error(t, err)
	assert.Equal(t, vanilla, gameHashes(t, e.game))
}

func TestApply_AModEmmDidNotRunStopsBeforeTheGameIsWritten(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks", "fast_travel_markers")
	vanilla := gameHashes(t, e.game)
	_, err := core.PristineCapture(context.Background(), core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	_, err = core.CompatCheck(context.Background(), core.CompatCheckRequest{Request: e.req})
	require.NoError(t, err)
	t.Setenv("FAKE_EMM_SKIP", "big_stacks")

	_, err = core.Apply(context.Background(), core.ApplyRequest{Request: e.req})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "big_stacks")
	assert.Equal(t, vanilla, gameHashes(t, e.game))
}

func TestApply_RunsAFailedCompatibilityCheckAgain(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	_, err := core.PristineCapture(context.Background(), core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	t.Setenv("FAKE_EMM_MODE", "fail")
	_, err = core.CompatCheck(context.Background(), core.CompatCheckRequest{Request: e.req})
	require.NoError(t, err)
	require.Equal(t, compat.Unsupported, status(t, e).Gate.Verdict)
	t.Setenv("FAKE_EMM_MODE", "ok")

	assert.Equal(t, []string{"big_stacks"}, apply(t, e).Deployed.Mods)
}

func TestApply_CancelledBeforeTheDeployLeavesTheGameAlone(t *testing.T) {
	for _, at := range []string{"getting ready", "done"} {
		t.Run(at, func(t *testing.T) {
			e := newEnv(t)
			enable(t, e, "big_stacks")
			vanilla := gameHashes(t, e.game)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.req.Events.Progress = func(p core.Progress) {
				if p.What == at {
					cancel()
				}
			}

			_, err := core.Apply(ctx, core.ApplyRequest{Request: e.req})

			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, vanilla, gameHashes(t, e.game))
		})
	}
}

func TestApply_RecoversFromAnUpdateThatLeftTheResourceFile(t *testing.T) {
	e := newEnv(t)
	underSteam(t, e)
	enable(t, e, "big_stacks")
	vanillaRes := gameHashes(t, e.game)[steam.ResourcesName]
	apply(t, e)
	steamUpdate(t, e)
	updated := gameHashes(t, e.game)

	st := status(t, e)
	assert.Equal(t, core.ModsUpdateRemoved, st.Mods.State)
	assert.Empty(t, st.Pristine.Patched)
	assert.Len(t, st.Pristine.Leftover, 9)

	res := apply(t, e)

	assert.Equal(t, []string{"big_stacks"}, res.Deployed.Mods)
	st = status(t, e)
	assert.Equal(t, core.ModsApplied, st.Mods.State)
	assert.Contains(t, st.Pristine.Source, "resource file held for build")

	_, err := core.Rollback(context.Background(), core.RollbackRequest{Request: e.req})
	require.NoError(t, err)
	after := gameHashes(t, e.game)
	assert.Equal(t, vanillaRes, after[steam.ResourcesName], "the appended records are gone")
	assert.Equal(t, updated[steam.KfcName], after[steam.KfcName], "the index is Steam's new one")
	assert.Len(t, after, 4, "no containers left over")
}

func TestStatus_SaysWhetherTheModsAreInTheGame(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		setup    func(t *testing.T, e *env)
		state    string
		sentence string
	}{
		"nothing enabled": {func(*testing.T, *env) {}, core.ModsNoneEnabled, "No mods are enabled. Choose some under Tweaks or Mods."},
		"not applied": {func(t *testing.T, e *env) { enable(t, e, "big_stacks") },
			core.ModsNotApplied, "Your mods are not in the game."},
		"applied": {func(t *testing.T, e *env) {
			enable(t, e, "big_stacks")
			apply(t, e)
		}, core.ModsApplied, "Your mod is in the game."},
		"parameter changed": {func(t *testing.T, e *env) {
			enable(t, e, "big_stacks")
			apply(t, e)
			_, err := core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req, Name: "big_stacks",
				Param: "STACK_MULTIPLIER", Value: 20})
			require.NoError(t, err)
		}, core.ModsSettingsChanged, "Your mod settings changed since the last deploy."},
		"mod added": {func(t *testing.T, e *env) {
			enable(t, e, "big_stacks")
			apply(t, e)
			enable(t, e, "fast_travel_markers")
		}, core.ModsSettingsChanged, "Your mod settings changed since the last deploy."},
		"another tool": {func(t *testing.T, e *env) {
			enable(t, e, "big_stacks")
			apply(t, e)
			kfc := filepath.Join(e.game, steam.KfcName)
			b, err := os.ReadFile(kfc)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(kfc, append(b, "other"...), 0o644)) //nolint:gosec // fixture
		}, core.ModsModified, "Another tool changed the game's archive. A copy is kept when you apply."},
		"patched and unrecoverable": {func(t *testing.T, e *env) {
			enable(t, e, "big_stacks")
			require.NoError(t, os.WriteFile(filepath.Join(e.game, "enshrouded_060.dat"), nil, 0o644)) //nolint:gosec // fixture
		}, core.ModsNeedsVerify, "The game's files need restoring in Steam."},
		"updated": {func(t *testing.T, e *env) {
			underSteam(t, e)
			enable(t, e, "big_stacks")
			apply(t, e)
			steamUpdate(t, e)
		}, core.ModsUpdateRemoved, "A game update removed your mods."},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			c.setup(t, e)
			m := status(t, e).Mods
			assert.Equal(t, c.state, m.State)
			assert.Equal(t, c.sentence, m.Sentence)
		})
	}
}

func TestStatus_NoGameAndRunningGameSayWhatToDo(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	e.running = true
	m := status(t, e).Mods
	assert.Equal(t, core.ModsNotApplied, m.State, "running changes the sentence, not the state")
	assert.Equal(t, "Quit Enshrouded to apply your mods.", m.Sentence)

	e.running = false
	apply(t, e)
	e.running = true
	assert.Equal(t, "Your mod is in the game.", status(t, e).Mods.Sentence, "nothing to quit for while playing with them")

	e.req.GameDir = filepath.Join(e.root, "nowhere")
	assert.Equal(t, core.ModsNoGame, status(t, e).Mods.State)
}

// A record from before parameters were kept compares names only: without this,
// every existing deploy would read as changed after upgrading.
func TestStatus_OldDeployRecordComparesNamesOnly(t *testing.T) {
	e := newEnv(t)
	enable(t, e, "big_stacks")
	apply(t, e)
	_, err := core.SetTweakParam(context.Background(), core.SetTweakParamRequest{Request: e.req, Name: "big_stacks",
		Param: "STACK_MULTIPLIER", Value: 20})
	require.NoError(t, err)
	rec := filepath.Join(e.ws, "deployed.json")
	b, err := os.ReadFile(rec)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	delete(doc, "params")
	require.NoError(t, os.WriteFile(rec, mustJSON(t, doc), 0o600))

	assert.Equal(t, core.ModsApplied, status(t, e).Mods.State)
}

func firstDir(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	return filepath.Join(root, entries[0].Name())
}

func fileTime(t *testing.T, p string) time.Time {
	t.Helper()
	fi, err := os.Stat(p)
	require.NoError(t, err)
	return fi.ModTime()
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
