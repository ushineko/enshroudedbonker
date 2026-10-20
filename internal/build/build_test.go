package build

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
	"github.com/ushineko/enshroudedbonker/internal/steam"
)

// Synthetic game files: a made-up build string in the archive index and a
// made-up commit blob in the executable. Nothing here comes from the game.
const (
	synthBuild  = "7654321|^/game99/branches/test_branch|2026-01-01T00:00:00.000000Z"
	synthCommit = "fedcba9876543210fedcba9876543210fedcba98"
	synthRes    = "ORIGINAL-RESOURCE-BYTES"
)

type fixture struct {
	root    string
	install *steam.Install
	fp      compat.Fingerprint
	ws      string
	emm     string
}

func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("STEAM_ROOT", filepath.Join(root, "no-steam"))
	t.Setenv("ENSHROUDEDBONKER_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("ENSHROUDEDBONKER_GAME_DIR", "")
	t.Setenv("FAKE_EMM_MODE", "")
	t.Setenv("FAKE_EMM_FAIL_WITH", "")
	t.Setenv("FAKE_EMM_SILENT", "")
	return root
}

func writeGame(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	files := map[string]string{
		steam.ExeName:        "MZ\x90\x00synthetic exe\x00" + synthCommit + "\x00\x00\x000.0.1.0\x00tail",
		steam.KfcName:        "KFC2\x00\x00" + synthBuild + "\x00index-body",
		steam.ResourcesName:  synthRes,
		"enshrouded_000.dat": "container zero",
		"enshrouded_001.dat": "container one",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)) //nolint:gosec // test fixture
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := sandbox(t)
	game := filepath.Join(root, "game")
	writeGame(t, game)
	in, err := steam.Locate(game)
	require.NoError(t, err)
	fp, err := compat.Read(in.Exe(), in.Kfc(), in.BuildID)
	require.NoError(t, err)
	return &fixture{root: root, install: in, fp: fp, ws: filepath.Join(root, "ws")}
}

// installFakeEmm puts the synthetic emm script where a test can run it.
func installFakeEmm(t *testing.T, dir string) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not on PATH")
	}
	body, err := os.ReadFile(filepath.Join("testdata", "fake-emm.sh"))
	require.NoError(t, err)
	p := filepath.Join(dir, "fake-emm")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(p, append([]byte("#!"+bash+"\n"), body...), 0o755)) //nolint:gosec // test executable
	return p
}

func hashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		h, err := gamefiles.HashFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		out[e.Name()] = h
	}
	return out
}

func notRunning() (bool, bool) { return false, true }

func TestPatchedSigns(t *testing.T) {
	f := newFixture(t)
	signs, err := PatchedSigns(f.install)
	require.NoError(t, err)
	assert.Empty(t, signs)

	require.NoError(t, os.WriteFile(filepath.Join(f.install.Dir, "enshrouded_055.dat"), nil, 0o644)) //nolint:gosec // fixture
	require.NoError(t, os.WriteFile(f.install.Kfc()+".bak", []byte("x"), 0o644))                     //nolint:gosec // fixture
	signs, err = PatchedSigns(f.install)
	require.NoError(t, err)
	require.Len(t, signs, 2)
	assert.Contains(t, signs[0], "enshrouded_055.dat")
	assert.Contains(t, signs[1], ".bak")
}

func TestCapturePristine_RefusesPatchedLookingInstall(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.install.Dir, "enshrouded_060.dat"), nil, 0o644)) //nolint:gosec // fixture
	_, err := CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, "")
	require.ErrorIs(t, err, ErrLooksPatched)
	_, err = LoadPristine(filepath.Join(f.ws, "pristine"), f.fp)
	require.ErrorIs(t, err, ErrNoPristine, "a refused capture must leave nothing behind")
}

func TestCapturePristine_RefusesOurOwnDeployedArchive(t *testing.T) {
	f := newFixture(t)
	h, err := gamefiles.HashFile(f.install.Kfc())
	require.NoError(t, err)
	_, err = CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, h)
	require.ErrorIs(t, err, ErrLooksPatched)
}

func TestCapturePristine_RecordsHashesSizesAndRealContainers(t *testing.T) {
	f := newFixture(t)
	root := filepath.Join(f.ws, "pristine")
	p, err := CapturePristine(root, f.install, f.fp, "")
	require.NoError(t, err)
	assert.Equal(t, int64(len(synthRes)), p.ResSize)
	want, _ := gamefiles.HashFile(f.install.Resources())
	assert.Equal(t, want, p.ResSHA256)
	require.Len(t, p.Dats, 2)
	assert.Equal(t, "enshrouded_000.dat", p.Dats[0].Name)

	again, err := LoadPristine(root, f.fp)
	require.NoError(t, err)
	assert.Equal(t, p.KfcSHA256, again.KfcSHA256)
	assert.Equal(t, p.Dir, again.Dir)
	eq, err := gamefiles.Equal(again.Kfc(), f.install.Kfc())
	require.NoError(t, err)
	assert.True(t, eq)
}

func TestImportPristine_RequiresSameBuildString(t *testing.T) {
	f := newFixture(t)
	backup := filepath.Join(f.root, "backup")
	require.NoError(t, os.MkdirAll(backup, 0o750))
	other := strings.Replace(synthBuild, "7654321", "7654322", 1)
	require.NoError(t, os.WriteFile(filepath.Join(backup, steam.KfcName), []byte("KFC2\x00"+other), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(backup, steam.ResourcesName), []byte(synthRes), 0o600))
	_, err := ImportPristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, backup)
	require.Error(t, err, "a backup of another build would make every build wrong")

	require.NoError(t, os.WriteFile(filepath.Join(backup, steam.KfcName), []byte("KFC2\x00"+synthBuild), 0o600))
	p, err := ImportPristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, backup)
	require.NoError(t, err)
	assert.Contains(t, p.Source, "imported")
}

func TestState(t *testing.T) {
	f := newFixture(t)
	s, err := State(f.install, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, StateUnknown, s)

	p, err := CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, "")
	require.NoError(t, err)
	s, _ = State(f.install, p, nil)
	assert.Equal(t, StateVanilla, s)

	require.NoError(t, os.WriteFile(f.install.Kfc(), []byte("KFC2\x00"+synthBuild+"changed"), 0o600))
	s, _ = State(f.install, p, nil)
	assert.Equal(t, StateOther, s)
	h, _ := gamefiles.HashFile(f.install.Kfc())
	s, _ = State(f.install, p, &Deployed{Files: []File{{Name: steam.KfcName, SHA256: h}}})
	assert.Equal(t, StateDeployed, s)
}

// built runs a full build into ws/builds/latest with the fake emm.
func built(t *testing.T, f *fixture, mods []ModSpec) (*Pristine, *Manifest) {
	t.Helper()
	f.emm = installFakeEmm(t, filepath.Join(f.root, "bin"))
	p, err := CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, "")
	require.NoError(t, err)
	man, _, err := Run(context.Background(), Options{Install: f.install, Pristine: p, Emm: f.emm, Mods: mods,
		Shadow: filepath.Join(f.ws, "shadow"), Out: filepath.Join(f.ws, "builds", "latest")})
	require.NoError(t, err)
	return p, man
}

func TestRun_CollectsWhatEmmWroteAndLeavesGameAlone(t *testing.T) {
	f := newFixture(t)
	before := hashes(t, f.install.Dir)
	_, man := built(t, f, []ModSpec{{Name: "big_stacks", Builtin: true, Overrides: map[string]float64{"STACK_MULTIPLIER": 20}}})
	assert.Equal(t, before, hashes(t, f.install.Dir), "a build must never write into the game directory")

	var names []string
	for _, fl := range man.Files {
		names = append(names, fl.Name)
	}
	assert.Contains(t, names, steam.KfcName)
	assert.Contains(t, names, steam.ResourcesName)
	assert.Contains(t, names, "enshrouded_055.dat")
	assert.Contains(t, names, "enshrouded_063.dat")
	assert.NotContains(t, names, "enshrouded_000.dat", "the shadow's links to the game's own containers are not output")
	assert.Len(t, names, 11)
	assert.Equal(t, 3, man.Applied)
	assert.Equal(t, []string{"big_stacks"}, man.Order)
	require.Len(t, man.Mods, 1)
	assert.True(t, man.Mods[0].Ran)

	staged, err := os.ReadFile(filepath.Join(f.ws, "shadow", "mods", "big_stacks", "src", "mod.lua"))
	require.NoError(t, err)
	assert.Contains(t, string(staged), "\nlocal STACK_MULTIPLIER = 20\n")

	loaded, err := LoadManifest(filepath.Join(f.ws, "builds", "latest"))
	require.NoError(t, err)
	assert.Equal(t, man.Files, loaded.Files)
	assert.FileExists(t, filepath.Join(f.ws, "builds", "latest", "emm.log"))
	v, err := gamefiles.ReadVersion(filepath.Join(f.ws, "builds", "latest", steam.KfcName), 1<<20)
	require.NoError(t, err)
	assert.Equal(t, synthBuild, v.Raw)
}

func TestRun_RefusesOutputThatRewroteThePristinePrefix(t *testing.T) {
	f := newFixture(t)
	f.emm = installFakeEmm(t, filepath.Join(f.root, "bin"))
	p, err := CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, "")
	require.NoError(t, err)
	t.Setenv("FAKE_EMM_MODE", "rewrite")
	out := filepath.Join(f.ws, "builds", "latest")
	_, _, err = Run(context.Background(), Options{Install: f.install, Pristine: p, Emm: f.emm,
		Mods: []ModSpec{{Name: "big_stacks", Builtin: true}}, Shadow: filepath.Join(f.ws, "shadow"), Out: out})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rewrote")
	assert.NoDirExists(t, out, "output that failed the check must not be published")
	eq, _ := gamefiles.Equal(p.Resources(), f.install.Resources())
	assert.True(t, eq, "the pristine copy must not be modified through the shadow")
}

func TestRun_FailedRebuildKeepsPreviousBuild(t *testing.T) {
	f := newFixture(t)
	p, first := built(t, f, []ModSpec{{Name: "big_stacks", Builtin: true}})
	t.Setenv("FAKE_EMM_MODE", "fail")
	_, _, err := Run(context.Background(), Options{Install: f.install, Pristine: p, Emm: f.emm,
		Mods: []ModSpec{{Name: "more_loot", Builtin: true}}, Shadow: filepath.Join(f.ws, "shadow"),
		Out: filepath.Join(f.ws, "builds", "latest")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "synthetic failure")
	kept, err := LoadManifest(filepath.Join(f.ws, "builds", "latest"))
	require.NoError(t, err)
	assert.Equal(t, first.Files, kept.Files)
	assert.Equal(t, "big_stacks", kept.Mods[0].Name)
}

func TestRun_DryRunPublishesNothing(t *testing.T) {
	f := newFixture(t)
	f.emm = installFakeEmm(t, filepath.Join(f.root, "bin"))
	p, err := CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, "")
	require.NoError(t, err)
	man, res, err := Run(context.Background(), Options{Install: f.install, Pristine: p, Emm: f.emm,
		Mods: []ModSpec{{Name: "big_stacks", Builtin: true}}, Shadow: filepath.Join(f.ws, "shadow")})
	require.NoError(t, err)
	assert.Empty(t, man.Files)
	assert.Equal(t, []string{"big_stacks: changed 5 things"}, res.ModLines["big_stacks"])
	assert.NoDirExists(t, filepath.Join(f.ws, "builds"))
}

func TestDeployRollback_RoundTrip(t *testing.T) {
	f := newFixture(t)
	original := hashes(t, f.install.Dir)
	p, man := built(t, f, []ModSpec{{Name: "big_stacks", Builtin: true}})
	record := filepath.Join(f.ws, "deployed.json")
	archive := filepath.Join(f.ws, "archive")

	_, err := Deploy(DeployOptions{Install: f.install, Fingerprint: f.fp, Pristine: p, Build: man,
		ArchiveRoot: archive, RecordPath: record, Running: func() (bool, bool) { return true, true }})
	require.ErrorIs(t, err, ErrGameRunning)
	assert.Equal(t, original, hashes(t, f.install.Dir), "a refused deploy writes nothing")

	rec, err := Deploy(DeployOptions{Install: f.install, Fingerprint: f.fp, Pristine: p, Build: man,
		ArchiveRoot: archive, RecordPath: record, Running: notRunning})
	require.NoError(t, err)
	assert.Empty(t, rec.Archive, "the pristine pair needs no archiving")
	assert.NoDirExists(t, archive)
	assert.Len(t, rec.Added, 9)
	assert.Equal(t, "enshrouded_055.dat", rec.Added[0])
	kfcNow, _ := gamefiles.HashFile(f.install.Kfc())
	assert.Equal(t, rec.KfcSHA256(), kfcNow)
	assert.Equal(t, []string{"big_stacks"}, rec.Mods)
	s, _ := State(f.install, p, rec)
	assert.Equal(t, StateDeployed, s)

	_, err = Rollback(RollbackOptions{Install: f.install, Pristine: p, RecordPath: record,
		Running: func() (bool, bool) { return true, true }})
	require.ErrorIs(t, err, ErrGameRunning)

	res, err := Rollback(RollbackOptions{Install: f.install, Pristine: p, RecordPath: record, Running: notRunning})
	require.NoError(t, err)
	assert.Equal(t, []string{steam.KfcName, steam.ResourcesName}, res.Restored)
	assert.Len(t, res.Removed, 9)
	assert.Equal(t, original, hashes(t, f.install.Dir), "rollback must restore exactly the vanilla files")
	assert.NoFileExists(t, record)
}

func TestDeploy_RefusesBuildForAnotherFingerprint(t *testing.T) {
	f := newFixture(t)
	p, man := built(t, f, []ModSpec{{Name: "big_stacks", Builtin: true}})
	other := f.fp
	other.ExeSHA256 = strings.Repeat("0", 64)
	before := hashes(t, f.install.Dir)
	_, err := Deploy(DeployOptions{Install: f.install, Fingerprint: other, Pristine: p, Build: man,
		ArchiveRoot: filepath.Join(f.ws, "archive"), RecordPath: filepath.Join(f.ws, "d.json"), Running: notRunning})
	require.Error(t, err)
	assert.Equal(t, before, hashes(t, f.install.Dir))
}

func TestDeploy_ArchivesForeignArchiveAndPrunes(t *testing.T) {
	f := newFixture(t)
	p, man := built(t, f, []ModSpec{{Name: "big_stacks", Builtin: true}})
	foreign := []byte("KFC2\x00" + synthBuild + "patched-by-something-else")
	require.NoError(t, os.WriteFile(f.install.Kfc(), foreign, 0o600))
	archive := filepath.Join(f.ws, "archive")
	for _, old := range []string{"20000101T000000Z", "20000102T000000Z", "20000103T000000Z"} {
		require.NoError(t, os.MkdirAll(filepath.Join(archive, old), 0o750))
	}

	rec, err := Deploy(DeployOptions{Install: f.install, Fingerprint: f.fp, Pristine: p, Build: man,
		ArchiveRoot: archive, RecordPath: filepath.Join(f.ws, "d.json"), Running: notRunning})
	require.NoError(t, err)
	require.NotEmpty(t, rec.Archive)
	kept, err := os.ReadFile(filepath.Join(rec.Archive, steam.KfcName))
	require.NoError(t, err)
	assert.Equal(t, foreign, kept, "what the deploy replaced must be kept")
	entries, err := os.ReadDir(archive)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "only the newest two archive entries are kept")
	assert.DirExists(t, rec.Archive, "pruning must never remove the copy just made")
}

func TestDeploy_RedeployRemovesContainersThePreviousDeployAdded(t *testing.T) {
	f := newFixture(t)
	p, man := built(t, f, []ModSpec{{Name: "big_stacks", Builtin: true}})
	record := filepath.Join(f.ws, "d.json")
	_, err := Deploy(DeployOptions{Install: f.install, Fingerprint: f.fp, Pristine: p, Build: man,
		ArchiveRoot: filepath.Join(f.ws, "archive"), RecordPath: record, Running: notRunning})
	require.NoError(t, err)

	// A later build that adds fewer containers.
	trimmed := *man
	trimmed.Files = nil
	for _, fl := range man.Files {
		if fl.Name != "enshrouded_063.dat" {
			trimmed.Files = append(trimmed.Files, fl)
		}
	}
	rec, err := Deploy(DeployOptions{Install: f.install, Fingerprint: f.fp, Pristine: p, Build: &trimmed,
		ArchiveRoot: filepath.Join(f.ws, "archive"), RecordPath: record, Running: notRunning})
	require.NoError(t, err)
	assert.Empty(t, rec.Archive, "our own last deploy is not archived")
	assert.NoFileExists(t, filepath.Join(f.install.Dir, "enshrouded_063.dat"))
	assert.FileExists(t, filepath.Join(f.install.Dir, "enshrouded_062.dat"))
}

func TestRollback_RemovesStrayEmptyContainersWithoutRecord(t *testing.T) {
	f := newFixture(t)
	p, err := CapturePristine(filepath.Join(f.ws, "pristine"), f.install, f.fp, "")
	require.NoError(t, err)
	stray := filepath.Join(f.install.Dir, "enshrouded_070.dat")
	require.NoError(t, os.WriteFile(stray, nil, 0o600))
	require.NoError(t, os.WriteFile(f.install.Kfc()+".bak", []byte("b"), 0o600))
	res, err := Rollback(RollbackOptions{Install: f.install, Pristine: p,
		RecordPath: filepath.Join(f.ws, "none.json"), Running: notRunning})
	require.NoError(t, err)
	assert.Equal(t, []string{"enshrouded_070.dat"}, res.Removed)
	assert.NoFileExists(t, f.install.Kfc()+".bak")
	assert.FileExists(t, filepath.Join(f.install.Dir, "enshrouded_000.dat"), "the game's own containers stay")
}

func TestLoadDeployed_MissingIsNil(t *testing.T) {
	d, err := LoadDeployed(filepath.Join(t.TempDir(), "none.json"))
	require.NoError(t, err)
	assert.Nil(t, d)
	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{"), 0o600))
	_, err = LoadDeployed(bad)
	require.Error(t, err)
	assert.Equal(t, "", (&Deployed{}).KfcSHA256())
}

func TestCopyFile_ReplacesAtomicallyAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(dst, []byte("old-and-longer"), 0o600))
	before, _ := os.Stat(dst)
	require.NoError(t, CopyFile(src, dst))
	b, _ := os.ReadFile(dst)
	assert.Equal(t, "new", string(b))
	after, _ := os.Stat(dst)
	assert.False(t, os.SameFile(before, after), "the destination is replaced by rename, never truncated in place")
	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 2)

	err := CopyFile(filepath.Join(dir, "missing"), dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}
