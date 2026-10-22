package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sandbox points every directory the package derives at a throwaway tree, so
// no test can read or write the real user's settings.
func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("STEAM_ROOT", filepath.Join(root, "no-steam"))
	t.Setenv(FileEnv, "")
	t.Setenv(GameDirEnv, "")
	return root
}

func TestLoadFrom_MissingFileIsDefaultsNotError(t *testing.T) {
	root := sandbox(t)
	path := filepath.Join(root, "nowhere", "config.json")

	c, err := LoadFrom(path)
	require.NoError(t, err)

	data := filepath.Join(root, "data", "enshroudedbonker")
	assert.Equal(t, path, c.Path())
	assert.Equal(t, "", c.GameDir)
	assert.Equal(t, filepath.Join(data, "library"), c.LibraryDir)
	assert.Equal(t, filepath.Join(data, "tools"), c.ToolsDir)
	assert.Equal(t, filepath.Join(data, "workspace"), c.WorkspaceDir)
	assert.NotNil(t, c.Params, "a nil Params map would panic on the first SetParam")
	assert.Empty(t, c.Mods)
	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, os.ErrNotExist), "loading must not create the file")
}

func TestLoadFrom_EmptyDirsInFileFallBackToDefaults(t *testing.T) {
	root := sandbox(t)
	path := filepath.Join(root, "c.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"library_dir":"","params":null}`), 0o600))

	c, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "data", "enshroudedbonker", "library"), c.LibraryDir)
	assert.NotNil(t, c.Params)
}

func TestLoadFrom_CorruptFileIsAnError(t *testing.T) {
	root := sandbox(t)
	path := filepath.Join(root, "c.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"game_dir":`), 0o600))
	_, err := LoadFrom(path)
	require.Error(t, err, "a corrupt file must not silently become the defaults and be overwritten")
}

func TestSetGet_RoundTripsEveryKey(t *testing.T) {
	root := sandbox(t)
	c, err := LoadFrom(filepath.Join(root, "c.json"))
	require.NoError(t, err)

	for _, k := range Keys() {
		want := filepath.Join(root, "set", k)
		require.NoError(t, c.Set(k, "  "+want+"  "), k)
		got, err := c.Get(k)
		require.NoError(t, err, k)
		assert.Equal(t, want, got, "%s: value must be trimmed and stored", k)
	}
	require.NoError(t, c.Save())

	again, err := LoadFrom(c.Path())
	require.NoError(t, err)
	for _, k := range Keys() {
		got, err := again.Get(k)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "set", k), got, "%s did not survive Save/Load", k)
	}
}

func TestSet_UnknownKeyIsRejected(t *testing.T) {
	sandbox(t)
	c := Defaults()
	require.Error(t, c.Set("mods", "x"))
	_, err := c.Get("params")
	require.Error(t, err)
}

func TestSet_EmptyRestoresDirectoryDefault(t *testing.T) {
	root := sandbox(t)
	c := Defaults()
	require.NoError(t, c.Set("library_dir", filepath.Join(root, "elsewhere")))
	require.NoError(t, c.Set("library_dir", ""))
	assert.Equal(t, filepath.Join(root, "data", "enshroudedbonker", "library"), c.LibraryDir)

	require.NoError(t, c.Set("game_dir", filepath.Join(root, "g")))
	require.NoError(t, c.Set("game_dir", ""))
	assert.Equal(t, "", c.GameDir, "an empty game_dir means discover from Steam, not a default path")
}

func TestSet_ExpandsLeadingTilde(t *testing.T) {
	root := sandbox(t)
	c := Defaults()
	require.NoError(t, c.Set("emm_path", "~/bin/emm"))
	assert.Equal(t, filepath.Join(root, "home", "bin", "emm"), c.EmmPath)
}

func TestExpandPath_OnlyLeadingTilde(t *testing.T) {
	root := sandbox(t)
	home := filepath.Join(root, "home")
	assert.Equal(t, home, ExpandPath("~"))
	assert.Equal(t, filepath.Join(home, "x"), ExpandPath("~/x"))
	assert.Equal(t, "a/~/b", ExpandPath("a/~/b"))
	assert.Equal(t, "~user/x", ExpandPath("~user/x"))
}

func TestMkdirAll_RefusesBareTildeComponent(t *testing.T) {
	root := sandbox(t)
	bad := filepath.Join(root, "x", "~", "y")
	err := MkdirAll(bad)
	require.ErrorIs(t, err, ErrTildeComponent)
	_, statErr := os.Stat(filepath.Join(root, "x"))
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "nothing may be created on the way")

	require.NoError(t, MkdirAll(filepath.Join(root, "x", "~y")), "~ inside a name is fine")
}

func TestSave_PreservesUnknownKeys(t *testing.T) {
	root := sandbox(t)
	path := filepath.Join(root, "c.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "game_dir": "",
  "from_a_newer_binary": {"nested": [1, 2, 3]},
  "theme": "dark"
}`), 0o600))

	c, err := LoadFrom(path)
	require.NoError(t, err)
	c.SetEnabled("big_stacks", true)
	require.NoError(t, c.Save())

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &doc))
	assert.JSONEq(t, `{"nested":[1,2,3]}`, string(doc["from_a_newer_binary"]))
	assert.JSONEq(t, `"dark"`, string(doc["theme"]))
	assert.JSONEq(t, `[{"name":"big_stacks","enabled":true}]`, string(doc["mods"]))
}

func TestSave_ReplacesByRenameAndLeavesNoTemp(t *testing.T) {
	root := sandbox(t)
	path := filepath.Join(root, "deep", "dir", "c.json")
	c, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, c.Save(), "Save must create the parent directory")

	before, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), before.Mode().Perm())

	c.SetParam("big_stacks", "STACK_MULTIPLIER", 20)
	require.NoError(t, c.Save())
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, os.SameFile(before, after),
		"Save must write a new file and rename it over the old one, not truncate in place")
	_, err = os.Stat(path + ".tmp")
	assert.True(t, errors.Is(err, os.ErrNotExist), "the temporary file must not be left behind")

	again, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"STACK_MULTIPLIER": 20}, again.ParamsFor("big_stacks"))
}

func TestSave_WithoutPathUsesFilePath(t *testing.T) {
	root := sandbox(t)
	c := Defaults()
	require.NoError(t, c.Save())
	_, err := os.Stat(filepath.Join(root, "config", "enshroudedbonker", "config.json"))
	require.NoError(t, err)
}

func TestSetEnabled_UpdatesInPlaceWithoutDuplicates(t *testing.T) {
	sandbox(t)
	c := Defaults()
	c.SetEnabled("b", true)
	c.SetEnabled("a", false)
	c.SetEnabled("b", false)
	c.SetEnabled("b", true)
	assert.Equal(t, []ModEntry{{Name: "b", Enabled: true}, {Name: "a", Enabled: false}}, c.Mods)

	m, ok := c.Mod("a")
	assert.True(t, ok)
	assert.False(t, m.Enabled)
	_, ok = c.Mod("zzz")
	assert.False(t, ok)
}

func TestEnabledNames_SortedAndOnlyEnabled(t *testing.T) {
	sandbox(t)
	c := Defaults()
	c.SetEnabled("zeta", true)
	c.SetEnabled("alpha", true)
	c.SetEnabled("mid", false)
	c.SetEnabled("beta", true)
	assert.Equal(t, []string{"alpha", "beta", "zeta"}, c.EnabledNames())
}

func TestRemoveMod_ForgetsParams(t *testing.T) {
	sandbox(t)
	c := Defaults()
	c.SetEnabled("a", true)
	c.SetEnabled("b", true)
	c.SetParam("a", "X", 1)
	c.RemoveMod("a")
	assert.Equal(t, []ModEntry{{Name: "b", Enabled: true}}, c.Mods)
	assert.NotContains(t, c.Params, "a", "a removed mod's overrides would come back if re-imported")
	c.RemoveMod("not-there") // no panic
}

func TestClearParam_DropsEmptyModMap(t *testing.T) {
	sandbox(t)
	c := Defaults()
	c.SetParam("m", "X", 1)
	c.SetParam("m", "Y", 2)
	c.ClearParam("m", "X")
	assert.Equal(t, map[string]float64{"Y": 2}, c.Params["m"])
	c.ClearParam("m", "Y")
	assert.NotContains(t, c.Params, "m", "an emptied mod map must not linger as {} in the file")

	c.SetParam("m", "X", 1)
	c.SetParam("m", "Y", 2)
	c.ClearParam("m", "")
	assert.NotContains(t, c.Params, "m")
	c.ClearParam("never", "X") // no panic on a missing mod
}

func TestParamsFor_IsACopy(t *testing.T) {
	sandbox(t)
	c := Defaults()
	c.SetParam("m", "X", 1)
	p := c.ParamsFor("m")
	p["X"] = 99
	p["NEW"] = 1
	assert.Equal(t, map[string]float64{"X": 1}, c.Params["m"], "callers must not be able to mutate the settings")
	assert.Nil(t, c.ParamsFor("none"))
}

func TestFilePath_EnvOverride(t *testing.T) {
	root := sandbox(t)
	assert.Equal(t, filepath.Join(root, "config", "enshroudedbonker", "config.json"), FilePath())
	t.Setenv(FileEnv, "~/custom.json")
	assert.Equal(t, filepath.Join(root, "home", "custom.json"), FilePath())

	c, err := Load()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "home", "custom.json"), c.Path())
}

func TestXDGUnset_FallsBackUnderHome(t *testing.T) {
	root := sandbox(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	home := filepath.Join(root, "home")
	assert.Equal(t, filepath.Join(home, ".config", "enshroudedbonker", "config.json"), FilePath())
	assert.Equal(t, filepath.Join(home, ".local", "share", "enshroudedbonker", "library"), Defaults().LibraryDir)
}

func TestResolvedGameDir_EnvOutranksSetting(t *testing.T) {
	root := sandbox(t)
	c := Defaults()
	dir, src := c.ResolvedGameDir()
	assert.Equal(t, "", dir)
	assert.Equal(t, "Steam manifests", src)

	c.GameDir = filepath.Join(root, "from-config")
	dir, src = c.ResolvedGameDir()
	assert.Equal(t, filepath.Join(root, "from-config"), dir)
	assert.Equal(t, "config game_dir", src)

	t.Setenv(GameDirEnv, filepath.Join(root, "from-env"))
	dir, src = c.ResolvedGameDir()
	assert.Equal(t, filepath.Join(root, "from-env"), dir)
	assert.Equal(t, "$"+GameDirEnv, src)
}

func TestPaths_AllUnderWorkspace(t *testing.T) {
	root := sandbox(t)
	c, err := LoadFrom(filepath.Join(root, "c.json"))
	require.NoError(t, err)
	require.NoError(t, c.Set("workspace_dir", filepath.Join(root, "ws")))
	p := c.Paths()
	ws := filepath.Join(root, "ws")
	for name, got := range map[string]string{
		"pristine": p.Pristine, "builds": p.Builds, "shadow": p.Shadow,
		"archive": p.Archive, "deployed": p.Deployed, "compat": p.Compat,
	} {
		assert.Equal(t, ws, filepath.Dir(got), "%s must live directly under the workspace", name)
	}
	assert.Equal(t, filepath.Join(root, "c.json"), p.Config)
}
