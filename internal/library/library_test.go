package library

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("STEAM_ROOT", filepath.Join(root, "no-steam"))
	t.Setenv("ENSHROUDEDBONKER_CONFIG", "")
	t.Setenv("ENSHROUDEDBONKER_GAME_DIR", "")
	tmp := filepath.Join(root, "tmp")
	require.NoError(t, os.MkdirAll(tmp, 0o750))
	t.Setenv("TMPDIR", tmp)
	return root
}

const synthLua = `-- a synthetic test mod
local items = game.assets.get_resources_by_type("test::Widget")
local one = game.assets.get_resource_by_type( "test::Gadget" )
local r = game.assets.get_resource("guid-not-real", "test::Gizmo")
print("synthetic: " .. #items)
`

func manifest(id, caps string) string {
	return `{"id":"` + id + `","name":"Synthetic ` + id + `","version":"1.0.0","capabilities":[` + caps +
		`],"authors":["tester"],"license":"MIT"}`
}

func writeMod(t *testing.T, dir, id, caps string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "lib"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mod.json"), []byte(manifest(id, caps)), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "mod.lua"), []byte(synthLua), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "lib", "extra.lua"),
		[]byte(`game.assets.get_resources_by_type("test::Extra")`), 0o600))
	return dir
}

func writeZip(t *testing.T, path string, files map[string]string) string {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for name, body := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return path
}

func TestImport_FromDirectory(t *testing.T) {
	root := sandbox(t)
	src := writeMod(t, filepath.Join(root, "incoming", "whatever-name"), "synth_mod", `"patch"`)
	lib := filepath.Join(root, "lib")

	m, err := Import(lib, src, []string{"big_stacks"}, false)
	require.NoError(t, err)
	assert.Equal(t, "synth_mod", m.ID)
	assert.Equal(t, filepath.Join(lib, "synth_mod"), m.Dir, "the library directory is named after the id")
	assert.Empty(t, m.Unsupported)
	assert.Equal(t, []string{"test::Extra", "test::Gadget", "test::Gizmo", "test::Widget"}, m.Types)

	b, err := os.ReadFile(filepath.Join(lib, "synth_mod", "src", "lib", "extra.lua"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "test::Extra", "nested sources are copied")

	mods, problems, err := List(lib)
	require.NoError(t, err)
	assert.Empty(t, problems)
	require.Len(t, mods, 1)
	assert.Equal(t, "synth_mod", mods[0].ID)
}

func TestImport_ExistingNeedsReplace(t *testing.T) {
	root := sandbox(t)
	src := writeMod(t, filepath.Join(root, "in"), "synth_mod", `"patch"`)
	lib := filepath.Join(root, "lib")
	_, err := Import(lib, src, nil, false)
	require.NoError(t, err)
	stale := filepath.Join(lib, "synth_mod", "src", "stale.lua")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o600))

	_, err = Import(lib, src, nil, false)
	require.Error(t, err)
	assert.FileExists(t, stale, "a refused import must not touch the existing copy")

	_, err = Import(lib, src, nil, true)
	require.NoError(t, err)
	assert.NoFileExists(t, stale, "replace must not merge old files into the new copy")
}

func TestImport_FromZipWithTopLevelFolder(t *testing.T) {
	root := sandbox(t)
	z := writeZip(t, filepath.Join(root, "synth.ZIP"), map[string]string{
		"synth-1.0/mod.json":    manifest("zipped", `"patch","export"`),
		"synth-1.0/src/mod.lua": synthLua,
		"synth-1.0/README.txt":  "hi",
	})
	lib := filepath.Join(root, "lib")
	m, err := Import(lib, z, nil, false)
	require.NoError(t, err)
	assert.Equal(t, "zipped", m.ID)
	assert.FileExists(t, filepath.Join(lib, "zipped", "src", "mod.lua"))
	assert.FileExists(t, filepath.Join(lib, "zipped", "README.txt"))

	left, err := os.ReadDir(filepath.Join(root, "tmp"))
	require.NoError(t, err)
	assert.Empty(t, left, "the unpack directory must be cleaned up")
}

func TestImport_FromZipAtRoot(t *testing.T) {
	root := sandbox(t)
	z := writeZip(t, filepath.Join(root, "flat.zip"), map[string]string{
		"mod.json": manifest("flat", `"patch"`), "src/mod.lua": synthLua,
	})
	m, err := Import(filepath.Join(root, "lib"), z, nil, false)
	require.NoError(t, err)
	assert.Equal(t, "flat", m.ID)
}

func TestImport_ZipSlipIsRefused(t *testing.T) {
	root := sandbox(t)
	z := writeZip(t, filepath.Join(root, "evil.zip"), map[string]string{
		"mod.json":    manifest("evil_mod", `"patch"`),
		"src/mod.lua": synthLua,
		"../evil":     "pwned",
	})
	lib := filepath.Join(root, "lib")
	_, err := Import(lib, z, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes")
	assert.NoFileExists(t, filepath.Join(root, "tmp", "evil"), "the escaping entry must never be written")
	assert.NoDirExists(t, filepath.Join(lib, "evil_mod"))
}

func TestImport_ZipWithTwoModsIsRefused(t *testing.T) {
	root := sandbox(t)
	z := writeZip(t, filepath.Join(root, "two.zip"), map[string]string{
		"a/mod.json": manifest("a", `"patch"`), "a/src/mod.lua": synthLua,
		"b/mod.json": manifest("b", `"patch"`), "b/src/mod.lua": synthLua,
	})
	_, err := Import(filepath.Join(root, "lib"), z, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 mods")

	empty := writeZip(t, filepath.Join(root, "empty.zip"), map[string]string{"readme": "x"})
	_, err = Import(filepath.Join(root, "lib"), empty, nil, false)
	require.ErrorIs(t, err, ErrNotAMod)
}

func TestImport_RejectsBadManifests(t *testing.T) {
	for name, tc := range map[string]struct {
		mod    string
		noLua  bool
		reason string
	}{
		"uppercase id":     {mod: manifest("Bad", `"patch"`)},
		"path id":          {mod: manifest("../x", `"patch"`)},
		"leading digit id": {mod: manifest("1x", `"patch"`)},
		"no name":          {mod: `{"id":"x","version":"1","capabilities":["patch"]}`},
		"no version":       {mod: `{"id":"x","name":"X","capabilities":["patch"]}`},
		"no capabilities":  {mod: `{"id":"x","name":"X","version":"1","capabilities":[]}`},
		"not json":         {mod: `{`},
		"no src/mod.lua":   {mod: manifest("x", `"patch"`), noLua: true},
	} {
		t.Run(name, func(t *testing.T) {
			root := sandbox(t)
			src := filepath.Join(root, "in")
			require.NoError(t, os.MkdirAll(filepath.Join(src, "src"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(src, "mod.json"), []byte(tc.mod), 0o600))
			if !tc.noLua {
				require.NoError(t, os.WriteFile(filepath.Join(src, "src", "mod.lua"), []byte(synthLua), 0o600))
			}
			lib := filepath.Join(root, "lib")
			_, err := Import(lib, src, nil, false)
			require.ErrorIs(t, err, ErrNotAMod)
			_, statErr := os.Stat(lib)
			assert.True(t, errors.Is(statErr, os.ErrNotExist), "nothing may be copied for a rejected mod")
		})
	}
}

func TestImport_NoManifestAtAll(t *testing.T) {
	root := sandbox(t)
	_, err := Import(filepath.Join(root, "lib"), root, nil, false)
	require.ErrorIs(t, err, ErrNotAMod)
}

func TestImport_ReservedBuiltinIDIsRefused(t *testing.T) {
	root := sandbox(t)
	src := writeMod(t, filepath.Join(root, "in"), "big_stacks", `"patch"`)
	lib := filepath.Join(root, "lib")
	_, err := Import(lib, src, []string{"big_stacks", "more_loot"}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
	// --replace is no way around it: a library mod must never shadow a built-in.
	_, err = Import(lib, src, []string{"big_stacks"}, true)
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(lib, "big_stacks"))
}

func TestCapabilities_RuntimeAndUnknownAreUnsupported(t *testing.T) {
	root := sandbox(t)
	lib := filepath.Join(root, "lib")
	rt, err := Import(lib, writeMod(t, filepath.Join(root, "a"), "rt", `"patch","runtime"`), nil, false)
	require.NoError(t, err, "a runtime mod is imported, then refused at build time")
	assert.Contains(t, rt.Unsupported, "runtime")

	odd, err := Import(lib, writeMod(t, filepath.Join(root, "b"), "odd", `"teleport"`), nil, false)
	require.NoError(t, err)
	assert.Contains(t, odd.Unsupported, "teleport")

	ok, err := Import(lib, writeMod(t, filepath.Join(root, "c"), "ok", `"export"`), nil, false)
	require.NoError(t, err)
	assert.Empty(t, ok.Unsupported)

	mods, _, err := List(lib)
	require.NoError(t, err)
	require.Len(t, mods, 3)
	assert.Equal(t, []string{"odd", "ok", "rt"}, []string{mods[0].ID, mods[1].ID, mods[2].ID}, "listing is sorted by id")
	assert.NotEmpty(t, mods[2].Unsupported)
}

func TestList_SkipsNonModsAndMisnamedDirs(t *testing.T) {
	root := sandbox(t)
	lib := filepath.Join(root, "lib")
	writeMod(t, filepath.Join(lib, "good"), "good", `"patch"`)
	writeMod(t, filepath.Join(lib, "renamed"), "actual_id", `"patch"`)
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "junk"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "stray.txt"), nil, 0o600))

	mods, problems, err := List(lib)
	require.NoError(t, err)
	require.Len(t, mods, 1)
	assert.Equal(t, "good", mods[0].ID)
	assert.Len(t, problems, 2)

	mods, problems, err = List(filepath.Join(root, "none"))
	require.NoError(t, err, "a library that does not exist yet is empty, not an error")
	assert.Empty(t, mods)
	assert.Empty(t, problems)
}

func TestRemove(t *testing.T) {
	root := sandbox(t)
	lib := filepath.Join(root, "lib")
	writeMod(t, filepath.Join(lib, "gone"), "gone", `"patch"`)
	require.NoError(t, Remove(lib, "gone"))
	assert.NoDirExists(t, filepath.Join(lib, "gone"))

	require.Error(t, Remove(lib, "gone"), "removing twice reports it")
	require.Error(t, Remove(lib, "../lib"), "an id that is a path must never reach RemoveAll")
	require.Error(t, Remove(lib, ""))

	// A directory without mod.json is not a mod and is left alone.
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "notamod"), 0o750))
	require.Error(t, Remove(lib, "notamod"))
	assert.DirExists(t, filepath.Join(lib, "notamod"))
}

func TestTypesInSource(t *testing.T) {
	assert.Equal(t, []string{"test::Gadget", "test::Gizmo", "test::Widget"}, TypesInSource([]byte(synthLua)))
	assert.Equal(t, []string{"a::B"}, TypesInSource([]byte(
		`get_resources_by_type("a::B") get_resource_by_type("a::B")`)), "duplicates are reported once")
	assert.Empty(t, TypesInSource([]byte(`get_resources_by_type(name) -- not a literal`)))
	assert.Empty(t, TypesInSource(nil))
}

func TestReferencedTypes_OnlyLuaUnderSrc(t *testing.T) {
	root := sandbox(t)
	dir := writeMod(t, filepath.Join(root, "m"), "m", `"patch"`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "notes.txt"),
		[]byte(`get_resources_by_type("txt::Ignored")`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outside.lua"),
		[]byte(`get_resources_by_type("root::Ignored")`), 0o600))
	assert.Equal(t, []string{"test::Extra", "test::Gadget", "test::Gizmo", "test::Widget"}, ReferencedTypes(dir))
	assert.Empty(t, ReferencedTypes(filepath.Join(root, "missing")))
}

func TestCopyTree_SkipsSymlinks(t *testing.T) {
	root := sandbox(t)
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "d"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(src, "d", "f"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret"), []byte("s"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(root, "secret"), filepath.Join(src, "link")))

	dst := filepath.Join(root, "dst")
	require.NoError(t, CopyTree(src, dst))
	assert.FileExists(t, filepath.Join(dst, "d", "f"))
	_, err := os.Lstat(filepath.Join(dst, "link"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "a symlink in an imported mod must not be followed or copied")
}
