package core_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/core"
	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
	"github.com/ushineko/enshroudedbonker/internal/steam"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// Synthetic game files; nothing here comes from the game.
const (
	synthBuild  = "7654321|^/game99/branches/test_branch|2026-01-01T00:00:00.000000Z"
	synthCommit = "fedcba9876543210fedcba9876543210fedcba98"
)

type env struct {
	root, game, ws string
	req            core.Request
	logMu          sync.Mutex
	log            []string
	running        bool
}

func (e *env) logs() string {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	return strings.Join(e.log, "\n")
}

// newEnv builds a synthetic install, a fake emm and a settings file pointing
// at both, with HOME, XDG and Steam all aimed at throwaway paths.
func newEnv(t *testing.T) *env {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not on PATH")
	}
	root := t.TempDir()
	for k, v := range map[string]string{
		"HOME": filepath.Join(root, "home"), "XDG_CONFIG_HOME": filepath.Join(root, "xdg-config"),
		"XDG_DATA_HOME": filepath.Join(root, "xdg-data"), "XDG_CACHE_HOME": filepath.Join(root, "xdg-cache"),
		"STEAM_ROOT": filepath.Join(root, "no-steam"), "ENSHROUDEDBONKER_CONFIG": "",
		"ENSHROUDEDBONKER_GAME_DIR": "", "FAKE_EMM_MODE": "", "FAKE_EMM_FAIL_WITH": "", "FAKE_EMM_SILENT": "",
	} {
		t.Setenv(k, v)
	}
	e := &env{root: root, game: filepath.Join(root, "game"), ws: filepath.Join(root, "ws")}

	require.NoError(t, os.MkdirAll(e.game, 0o750))
	for name, body := range map[string]string{
		steam.ExeName:        "MZ\x90\x00synthetic\x00" + synthCommit + "\x00\x00\x000.0.1.0\x00tail",
		steam.KfcName:        "KFC2\x00\x00" + synthBuild + "\x00index",
		steam.ResourcesName:  "ORIGINAL-RESOURCE-BYTES",
		"enshrouded_000.dat": "container zero",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(e.game, name), []byte(body), 0o644)) //nolint:gosec // fixture
	}

	script, err := os.ReadFile(filepath.Join("..", "build", "testdata", "fake-emm.sh"))
	require.NoError(t, err)
	emmPath := filepath.Join(root, "bin", "emm")
	require.NoError(t, os.MkdirAll(filepath.Dir(emmPath), 0o750))
	require.NoError(t, os.WriteFile(emmPath, append([]byte("#!"+bash+"\n"), script...), 0o755)) //nolint:gosec // test executable

	cfgPath := filepath.Join(root, "config.json")
	cfg, err := json.Marshal(map[string]any{
		"emm_path": emmPath, "workspace_dir": e.ws,
		"library_dir": filepath.Join(root, "library"), "tools_dir": filepath.Join(root, "tools"),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, cfg, 0o600))

	e.req = core.Request{
		ConfigPath: cfgPath,
		GameDir:    e.game,
		Running:    func() (bool, bool) { return e.running, true },
		Events: core.Events{Log: func(_ core.Level, msg string) {
			e.logMu.Lock()
			e.log = append(e.log, msg)
			e.logMu.Unlock()
		}},
	}
	return e
}

func gameHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, en := range entries {
		h, err := gamefiles.HashFile(filepath.Join(dir, en.Name()))
		require.NoError(t, err)
		out[en.Name()] = h
	}
	return out
}

func TestEndToEnd_CaptureCheckBuildDeployRollback(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	vanilla := gameHashes(t, e.game)

	st, err := core.Status(ctx, core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	assert.True(t, st.Install.Found, st.Install.Error)
	assert.Equal(t, "--game-dir", st.Install.Source)
	require.NotNil(t, st.Fingerprint)
	assert.Equal(t, "7654321", st.Fingerprint.Kfc.Revision)
	assert.Equal(t, synthCommit, st.Fingerprint.ExeCommit)
	assert.Equal(t, compat.Unchecked, st.Gate.Verdict, "a synthetic build is never in the ledger")
	assert.False(t, st.Pristine.Present)
	assert.Equal(t, build.StateUnknown, st.State)
	assert.True(t, st.Emm.Installed)
	assert.Equal(t, "emm 0.0.0-fake", st.Emm.Version)
	assert.Equal(t, len(tweaks.Names()), st.Tweaks)

	_, err = core.Build(ctx, core.BuildRequest{Request: e.req})
	require.Error(t, err, "an unchecked install must not build without a dry run")
	assert.Contains(t, err.Error(), "compatibility check")

	_, err = core.CompatCheck(ctx, core.CompatCheckRequest{Request: e.req})
	require.ErrorIs(t, err, build.ErrNoPristine, "the dry run needs the pristine pair")

	pr, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	require.NotNil(t, pr.Pristine)

	cc, err := core.CompatCheck(ctx, core.CompatCheckRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, compat.Derived, cc.Decision.Verdict)
	assert.Empty(t, cc.Decision.Disabled)
	assert.True(t, cc.DryRun.OK)
	assert.Equal(t, "emm 0.0.0-fake", cc.DryRun.EmmVersion)
	require.Len(t, cc.DryRun.Tweaks, len(tweaks.Names()))
	for _, tr := range cc.DryRun.Tweaks {
		assert.True(t, tr.OK, "%s: %s", tr.Name, tr.Problem)
		assert.Equal(t, "changed 5 things", tr.Summary)
	}
	assert.Equal(t, vanilla, gameHashes(t, e.game), "the dry run must not touch the game")

	st, err = core.Status(ctx, core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, compat.Derived, st.Gate.Verdict, "the dry run is recorded per fingerprint")
	assert.True(t, st.Pristine.Present)
	assert.Equal(t, build.StateVanilla, st.State)

	_, err = core.Build(ctx, core.BuildRequest{Request: e.req})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no mods are enabled")

	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req,
		Names: []string{"big_stacks", "fast_travel_markers"}, Enabled: true})
	require.NoError(t, err)
	_, err = core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req,
		Name: "big_stacks", Param: "STACK_MULTIPLIER", Value: 20})
	require.NoError(t, err)

	br, err := core.Build(ctx, core.BuildRequest{Request: e.req})
	require.NoError(t, err)
	require.NotNil(t, br.Manifest)
	assert.Equal(t, 6, br.Manifest.Applied)
	assert.ElementsMatch(t, []string{"big_stacks", "fast_travel_markers"}, br.Manifest.Order)
	assert.Len(t, br.Manifest.Files, 11)
	assert.Equal(t, map[string]float64{"STACK_MULTIPLIER": 20}, br.Manifest.Mods[0].Params)
	assert.Equal(t, vanilla, gameHashes(t, e.game), "Build must not write into the game directory")
	assert.Contains(t, e.logs(), "big_stacks: changed 5 things")

	e.running = true
	_, err = core.Deploy(ctx, core.DeployRequest{Request: e.req})
	require.ErrorIs(t, err, build.ErrGameRunning)
	assert.Equal(t, vanilla, gameHashes(t, e.game))
	e.running = false

	dr, err := core.Deploy(ctx, core.DeployRequest{Request: e.req})
	require.NoError(t, err)
	assert.Len(t, dr.Deployed.Added, 9)
	assert.Equal(t, []string{"big_stacks", "fast_travel_markers"}, dr.Deployed.Mods)

	st, err = core.Status(ctx, core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, build.StateDeployed, st.State)
	assert.True(t, st.LastBuild.Present)
	assert.True(t, st.LastBuild.Current)
	require.NotNil(t, st.Deployed)

	_, err = core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.ErrorIs(t, err, build.ErrLooksPatched, "a patched install must never become the pristine copy")

	e.running = true
	_, err = core.Rollback(ctx, core.RollbackRequest{Request: e.req})
	require.ErrorIs(t, err, build.ErrGameRunning)
	e.running = false

	rb, err := core.Rollback(ctx, core.RollbackRequest{Request: e.req})
	require.NoError(t, err)
	assert.Len(t, rb.Removed, 9)
	assert.Equal(t, vanilla, gameHashes(t, e.game), "rollback restores the original hashes and removes added containers")

	st, err = core.Status(ctx, core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, build.StateVanilla, st.State)
	assert.Nil(t, st.Deployed)

	rep, err := core.Report(ctx, core.ReportRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, br.Manifest.Files, rep.Manifest.Files)
}

func TestCompatCheck_OneFailingTweakIsDisabledNotFatal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)

	t.Setenv("FAKE_EMM_FAIL_WITH", "more_loot")
	cc, err := core.CompatCheck(ctx, core.CompatCheckRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, compat.Derived, cc.Decision.Verdict)
	assert.Equal(t, []string{"more_loot"}, cc.Decision.Disabled)
	assert.True(t, cc.DryRun.OK)
	t.Setenv("FAKE_EMM_FAIL_WITH", "")

	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req,
		Names: []string{"more_loot", "big_stacks"}, Enabled: true})
	require.NoError(t, err)
	_, err = core.Build(ctx, core.BuildRequest{Request: e.req})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more_loot", "a tweak the dry run disabled must not be built")

	lt, err := core.ListTweaks(ctx, core.ListTweaksRequest{Request: e.req})
	require.NoError(t, err)
	for _, tw := range lt.Tweaks {
		if tw.Name == "more_loot" {
			assert.NotEmpty(t, tw.Disabled)
		} else {
			assert.Empty(t, tw.Disabled, tw.Name)
		}
	}
}

func TestCompatCheck_SilentTweakIsDisabled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	t.Setenv("FAKE_EMM_SILENT", "big_stacks")
	cc, err := core.CompatCheck(ctx, core.CompatCheckRequest{Request: e.req})
	require.NoError(t, err)
	assert.Equal(t, []string{"big_stacks"}, cc.Decision.Disabled,
		"a tweak that ran but reported nothing did not apply")
}

func TestCompatCheck_EverythingFailsIsUnsupported(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	t.Setenv("FAKE_EMM_MODE", "fail")
	cc, err := core.CompatCheck(ctx, core.CompatCheckRequest{Request: e.req})
	require.NoError(t, err, "a failed dry run is a verdict, not an error")
	assert.Equal(t, compat.Unsupported, cc.Decision.Verdict)
	assert.False(t, cc.DryRun.OK)
	t.Setenv("FAKE_EMM_MODE", "")

	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req, Names: []string{"big_stacks"}, Enabled: true})
	require.NoError(t, err)
	_, err = core.Build(ctx, core.BuildRequest{Request: e.req, Force: true})
	require.Error(t, err, "Force must not override an unsupported verdict")
	assert.Contains(t, err.Error(), "refusing")
}

func TestBuild_ForceSkipsDryRunForUnchecked(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req, Names: []string{"big_stacks"}, Enabled: true})
	require.NoError(t, err)
	br, err := core.Build(ctx, core.BuildRequest{Request: e.req, Force: true})
	require.NoError(t, err)
	assert.Equal(t, 3, br.Manifest.Applied)
}

// A saved value for a parameter the tweak no longer declares -- one a newer
// version removed -- is ignored with a warning, and the build still runs with
// the values that do apply (issue #20).
func TestBuild_IgnoresAValueForARemovedParameter(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req, Names: []string{"big_stacks"}, Enabled: true})
	require.NoError(t, err)
	_, err = core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req, Name: "big_stacks",
		Param: "STACK_MULTIPLIER", Value: 20})
	require.NoError(t, err)

	// What an older version left in the settings file.
	raw, err := os.ReadFile(e.req.ConfigPath)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(raw, &cfg))
	cfg["params"].(map[string]any)["big_stacks"].(map[string]any)["REMOVED_PARAM"] = 10
	raw, err = json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(e.req.ConfigPath, raw, 0o600))

	br, err := core.Build(ctx, core.BuildRequest{Request: e.req, Force: true})
	require.NoError(t, err)
	require.Len(t, br.Warnings, 1)
	assert.Contains(t, br.Warnings[0], "REMOVED_PARAM")
	assert.Contains(t, e.logs(), "REMOVED_PARAM")
	assert.Equal(t, map[string]float64{"STACK_MULTIPLIER": 20}, br.Manifest.Mods[0].Params)
	assert.Empty(t, br.Manifest.Overlaps, "a stale value is not an overlap between mods")
}

func TestStatus_NoGameIsAFactNotAnError(t *testing.T) {
	e := newEnv(t)
	req := e.req
	req.GameDir = ""
	st, err := core.Status(context.Background(), core.StatusRequest{Request: req})
	require.NoError(t, err)
	assert.False(t, st.Install.Found)
	assert.NotEmpty(t, st.Install.Error)
	assert.Equal(t, "Steam manifests", st.Install.Source)
	assert.Nil(t, st.Fingerprint)

	det, err := core.Detect(context.Background(), core.DetectRequest{Request: req})
	require.NoError(t, err)
	assert.False(t, det.Found)
	assert.NotEmpty(t, det.Candidates)
}

func TestSetTweakParam_ClampsAndDefaultClears(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, err := core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req, Name: "big_stacks",
		Param: "STACK_MULTIPLIER", Value: 1000})
	require.NoError(t, err)
	assert.True(t, r.Clamped)
	assert.Equal(t, 100.0, r.New)

	_, err = core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req, Name: "big_stacks",
		Param: "STACK_MULTIPLIER", Value: 10})
	require.NoError(t, err)
	lt, err := core.ListTweaks(ctx, core.ListTweaksRequest{Request: e.req})
	require.NoError(t, err)
	for _, tw := range lt.Tweaks {
		for _, p := range tw.Params {
			assert.False(t, p.Overridden, "setting the default must drop the override (%s.%s)", tw.Name, p.Name)
		}
	}

	_, err = core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req, Name: "big_stacks", Param: "NOPE", Value: 1})
	require.Error(t, err)
	_, err = core.SetTweakParam(ctx, core.SetTweakParamRequest{Request: e.req, Name: "nope", Param: "X", Value: 1})
	require.Error(t, err)
}

func TestMods_ImportEnableRemove(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := filepath.Join(e.root, "incoming")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "src"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(src, "mod.json"),
		[]byte(`{"id":"synth","name":"Synth","version":"1","capabilities":["runtime"]}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "src", "mod.lua"), []byte("print('x')"), 0o600))

	im, err := core.ImportMod(ctx, core.ImportModRequest{Request: e.req, Path: src})
	require.NoError(t, err)
	assert.NotEmpty(t, im.Mod.Unsupported)

	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req, Names: []string{"synth"}, Enabled: true})
	require.Error(t, err, "a runtime mod cannot be enabled")
	_, err = core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: e.req, Names: []string{"ghost"}, Enabled: true})
	require.Error(t, err)

	_, err = core.RemoveMod(ctx, core.RemoveModRequest{Request: e.req, Name: "big_stacks"})
	require.Error(t, err, "built-ins cannot be removed")
	_, err = core.RemoveMod(ctx, core.RemoveModRequest{Request: e.req, Name: "synth"})
	require.NoError(t, err)
	lm, err := core.ListMods(ctx, core.ListModsRequest{Request: e.req})
	require.NoError(t, err)
	for _, m := range lm.Mods {
		assert.NotEqual(t, "synth", m.Name)
		assert.True(t, m.Builtin)
	}
}

func TestPristineCapture_AcceptsContainersASteamUpdateLeftBehind(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// The game moves into a Steam library, whose manifest says when Steam
	// last updated it.
	lib := filepath.Join(e.root, "lib")
	game := filepath.Join(lib, "steamapps", "common", "Enshrouded")
	require.NoError(t, os.MkdirAll(filepath.Dir(game), 0o750))
	require.NoError(t, os.Rename(e.game, game))
	e.req.GameDir = game
	update := time.Now().Add(-time.Hour).Truncate(time.Second)
	acf := fmt.Sprintf("\"AppState\"\n{\n\"appid\" %q\n\"installdir\" \"Enshrouded\"\n\"LastUpdated\" \"%d\"\n\"buildid\" \"1\"\n}\n",
		steam.AppID, update.Unix())
	require.NoError(t, os.WriteFile(filepath.Join(lib, "steamapps", "appmanifest_"+steam.AppID+".acf"), []byte(acf), 0o600))

	// emm's containers from a patch three days earlier; Steam has since
	// replaced the archive pair.
	before := update.Add(-72 * time.Hour)
	for i := 55; i <= 63; i++ {
		p := filepath.Join(game, fmt.Sprintf("enshrouded_%03d.dat", i))
		require.NoError(t, os.WriteFile(p, nil, 0o644)) //nolint:gosec // fixture
		require.NoError(t, os.Chtimes(p, before, before))
	}
	for _, name := range []string{steam.KfcName, steam.ResourcesName} {
		require.NoError(t, os.Chtimes(filepath.Join(game, name), update, update))
	}

	st, err := core.Status(ctx, core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	assert.Empty(t, st.Pristine.Patched)
	assert.Len(t, st.Pristine.Leftover, 9)

	pr, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: e.req})
	require.NoError(t, err)
	assert.Len(t, pr.Leftover, 9)
	assert.Contains(t, core.LeftoverNote(pr.Leftover), "9 empty containers")

	st, err = core.Status(ctx, core.StatusRequest{Request: e.req})
	require.NoError(t, err)
	assert.True(t, st.Pristine.Present)
	assert.Equal(t, build.StateVanilla, st.State)
}
