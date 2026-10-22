package steam

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	t.Setenv("STEAM_ROOT", "")
	t.Setenv("ENSHROUDEDBONKER_CONFIG", "")
	t.Setenv("ENSHROUDEDBONKER_GAME_DIR", "")
	return root
}

func write(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// gameFiles creates the three files that make a directory an install, empty.
func gameFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	if names == nil {
		names = []string{ExeName, KfcName, ResourcesName}
	}
	for _, n := range names {
		write(t, filepath.Join(dir, n), "")
	}
}

const manifestACF = `"AppState"
{
	"appid"		"1203620"
	"name"		"Enshrouded"
	"StateFlags"		"4"
	"installdir"		"Enshrouded"
	"LastUpdated"		"1700000000"
	"buildid"		"1000001"
	"InstalledDepots"
	{
		"1203621"
		{
			"manifest"		"1"
		}
	}
}
`

func libraryFoldersVDF(paths ...string) string {
	var b strings.Builder
	b.WriteString("\"libraryfolders\"\n{\n")
	for i, p := range paths {
		b.WriteString("\t\"" + string(rune('0'+i)) + "\"\n\t{\n")
		b.WriteString("\t\t\"path\"\t\t\"" + strings.ReplaceAll(p, `\`, `\\`) + "\"\n")
		b.WriteString("\t\t\"label\"\t\t\"\"\n\t\t\"apps\"\n\t\t{\n\t\t\t\"1203620\"\t\t\"1\"\n\t\t}\n\t}\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func TestParseVDF_LibraryFoldersKeepsOrderAndUnescapesPaths(t *testing.T) {
	doc := parseVDF(libraryFoldersVDF("/first/lib", `D:\SteamLibrary`, "/third"))
	lf := doc.child("LIBRARYFOLDERS")
	require.NotNil(t, lf, "block keys are matched case-insensitively, as Steam does")
	require.Len(t, lf.children, 3)
	assert.Equal(t, "/first/lib", lf.children[0].node.values["path"])
	assert.Equal(t, `D:\SteamLibrary`, lf.children[1].node.values["path"],
		`"D:\\SteamLibrary" must not be read with a doubled backslash`)
	assert.Equal(t, "/third", lf.children[2].node.values["path"])
	apps := lf.children[0].node.child("apps")
	require.NotNil(t, apps)
	assert.Equal(t, "1", apps.values[AppID])
}

func TestParseVDF_AppManifest(t *testing.T) {
	app := parseVDF(manifestACF).child("AppState")
	require.NotNil(t, app)
	assert.Equal(t, "Enshrouded", app.values["installdir"])
	assert.Equal(t, "1000001", app.values["buildid"])
	assert.Equal(t, "1700000000", app.values["LastUpdated"])
	assert.NotNil(t, app.child("InstalledDepots").child("1203621"))
}

func TestParseVDF_MalformedNeverPanics(t *testing.T) {
	for _, in := range []string{
		"", "}", "}}}\n{", `"unterminated`, `"k" "v`, `"a\`, "{\n{\n{",
		"// comment only\n", `"k" "esc\"aped" // trailing`,
	} {
		assert.NotPanics(t, func() { parseVDF(in) }, "%q", in)
	}
	n := parseVDF(`"k"  "a\"b\tc"`)
	assert.Equal(t, "a\"b\tc", n.values["k"])
}

func TestParseVDF_LaterDuplicateWins(t *testing.T) {
	n := parseVDF("\"k\" \"one\"\n\"k\" \"two\"\n")
	assert.Equal(t, "two", n.values["k"])
}

func TestLibraries_RootFirstAndDeduplicatedThroughSymlink(t *testing.T) {
	root := sandbox(t)
	steamRoot := filepath.Join(root, "steam")
	lib2 := filepath.Join(root, "lib2")
	require.NoError(t, os.MkdirAll(lib2, 0o750))
	require.NoError(t, os.MkdirAll(steamRoot, 0o750))
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(steamRoot, alias))
	write(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		libraryFoldersVDF(alias, lib2, lib2, ""))

	assert.Equal(t, []string{steamRoot, lib2}, Libraries(steamRoot),
		"the root via a symlink and a repeated library must each be examined once")
}

func TestLibraries_NoFileIsJustRoot(t *testing.T) {
	root := sandbox(t)
	assert.Equal(t, []string{root}, Libraries(root))
}

func TestRoots_SteamRootFirst(t *testing.T) {
	root := sandbox(t)
	t.Setenv("STEAM_ROOT", filepath.Join(root, "custom"))
	r := Roots()
	require.NotEmpty(t, r)
	assert.Equal(t, filepath.Join(root, "custom"), r[0])
	for _, p := range r[1:] {
		assert.True(t, strings.HasPrefix(p, filepath.Join(root, "home")), "%s is not under HOME", p)
	}
}

// steamLayout builds root/steamapps/libraryfolders.vdf pointing at a second
// library that holds the game.
func steamLayout(t *testing.T, root string) (steamRoot, lib, gameDir string) {
	t.Helper()
	steamRoot = filepath.Join(root, "steam")
	lib = filepath.Join(root, "games")
	require.NoError(t, os.MkdirAll(filepath.Join(steamRoot, "steamapps"), 0o750))
	write(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"), libraryFoldersVDF(steamRoot, lib))
	write(t, filepath.Join(lib, "steamapps", "appmanifest_"+AppID+".acf"), manifestACF)
	gameDir = filepath.Join(lib, "steamapps", "common", "Enshrouded")
	return steamRoot, lib, gameDir
}

func TestLocate_FindsSyntheticInstallViaSteamRoot(t *testing.T) {
	root := sandbox(t)
	steamRoot, lib, gameDir := steamLayout(t, root)
	gameFiles(t, gameDir)
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "steamapps", "compatdata", AppID), 0o750))
	t.Setenv("STEAM_ROOT", steamRoot)

	in, err := Locate("")
	require.NoError(t, err)
	assert.Equal(t, gameDir, in.Dir)
	assert.Equal(t, lib, in.LibraryDir)
	assert.Equal(t, "1000001", in.BuildID)
	assert.Equal(t, "Enshrouded", in.Name)
	assert.Equal(t, "1700000000", in.LastUpdated)
	assert.Equal(t, filepath.Join(lib, "steamapps", "compatdata", AppID), in.CompatDataDir)
	assert.Equal(t, filepath.Join(gameDir, ExeName), in.Exe())
	assert.Equal(t, filepath.Join(gameDir, KfcName), in.Kfc())
	assert.Equal(t, filepath.Join(gameDir, ResourcesName), in.Resources())

	require.Len(t, in.Candidates, 2, "the root library is examined and kept as a candidate before the hit")
	assert.Equal(t, steamRoot, in.Candidates[0].LibraryDir)
	assert.Contains(t, in.Candidates[0].Reason, "appmanifest")
	assert.Equal(t, "", in.Candidates[1].Reason)
}

func TestLocate_RejectsInstallMissingAFile(t *testing.T) {
	for _, missing := range []string{ExeName, KfcName, ResourcesName} {
		t.Run(missing, func(t *testing.T) {
			root := sandbox(t)
			steamRoot, _, gameDir := steamLayout(t, root)
			var present []string
			for _, n := range []string{ExeName, KfcName, ResourcesName} {
				if n != missing {
					present = append(present, n)
				}
			}
			gameFiles(t, gameDir, present...)
			t.Setenv("STEAM_ROOT", steamRoot)

			in, err := Locate("")
			require.ErrorIs(t, err, ErrNotFound)
			require.NotNil(t, in, "a failed search still returns the candidates for `detect`")
			var reasons []string
			for _, c := range in.Candidates {
				reasons = append(reasons, c.Reason)
			}
			assert.Contains(t, reasons, missing+" is missing")
		})
	}
}

func TestLocate_RejectsDirectoryInPlaceOfFile(t *testing.T) {
	root := sandbox(t)
	dir := filepath.Join(root, "g")
	gameFiles(t, dir, ExeName, ResourcesName)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, KfcName), 0o750))
	_, err := Locate(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), KfcName+" is a directory")
}

func TestLocate_ManifestWithoutInstallDir(t *testing.T) {
	root := sandbox(t)
	steamRoot := filepath.Join(root, "steam")
	write(t, filepath.Join(steamRoot, "steamapps", "appmanifest_"+AppID+".acf"), "\"AppState\"\n{\n\"name\" \"x\"\n}\n")
	t.Setenv("STEAM_ROOT", steamRoot)
	in, err := Locate("")
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, "appmanifest has no installdir", in.Candidates[0].Reason)
}

func TestLocate_ExplicitOverrideSkipsDiscoveryButValidates(t *testing.T) {
	root := sandbox(t)
	// A Steam root that would also match: the override must win, not fall back.
	steamRoot, _, steamGame := steamLayout(t, root)
	gameFiles(t, steamGame)
	t.Setenv("STEAM_ROOT", steamRoot)

	loose := filepath.Join(root, "loose", "copy")
	gameFiles(t, loose)
	in, err := Locate(loose)
	require.NoError(t, err)
	assert.Equal(t, loose, in.Dir)
	assert.Equal(t, "", in.LibraryDir, "a directory outside Steam's layout has no library")
	assert.Equal(t, "", in.BuildID)
	require.Len(t, in.Candidates, 1)

	broken := filepath.Join(root, "broken")
	gameFiles(t, broken, ExeName)
	_, err = Locate(broken)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
	assert.Contains(t, err.Error(), KfcName+" is missing",
		"a bad --game-dir is named, not replaced by whatever Steam has")
}

func TestLocate_OverrideInsideSteamLayoutReadsManifest(t *testing.T) {
	root := sandbox(t)
	_, lib, gameDir := steamLayout(t, root)
	gameFiles(t, gameDir)
	in, err := Locate(gameDir)
	require.NoError(t, err)
	assert.Equal(t, lib, in.LibraryDir)
	assert.Equal(t, "1000001", in.BuildID)
}

func TestDatFiles_SortedByIndexWithSizes(t *testing.T) {
	root := sandbox(t)
	dir := filepath.Join(root, "g")
	write(t, filepath.Join(dir, "enshrouded_010.dat"), "0123456789")
	write(t, filepath.Join(dir, "enshrouded_002.dat"), "ab")
	write(t, filepath.Join(dir, "enshrouded_055.dat"), "")
	write(t, filepath.Join(dir, "enshrouded_1.dat"), "x")      // wrong width
	write(t, filepath.Join(dir, "enshrouded_003.dat.bak"), "") // not a container
	write(t, filepath.Join(dir, "other.dat"), "")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "enshrouded_004.dat"), 0o750))

	in := &Install{Dir: dir}
	got, err := in.DatFiles()
	require.NoError(t, err)
	assert.Equal(t, []DatFile{
		{Name: "enshrouded_002.dat", Index: 2, Size: 2},
		{Name: "enshrouded_010.dat", Index: 10, Size: 10},
		{Name: "enshrouded_055.dat", Index: 55, Size: 0},
	}, got)
}

func TestDatFiles_MissingDirIsAnError(t *testing.T) {
	root := sandbox(t)
	_, err := (&Install{Dir: filepath.Join(root, "nope")}).DatFiles()
	require.Error(t, err)
}

func TestLibraryOf(t *testing.T) {
	assert.Equal(t, "/lib", libraryOf("/lib/steamapps/common/Enshrouded"))
	assert.Equal(t, "/lib", libraryOf("/lib/steamapps/common/Enshrouded/"))
	assert.Equal(t, "", libraryOf("/lib/common/Enshrouded"))
	assert.Equal(t, "", libraryOf("/lib/other/common/Enshrouded"))
}
