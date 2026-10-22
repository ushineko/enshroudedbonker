package wine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tree is a synthetic Steam layout: a Proton install and the game's Proton
// prefix, whose config_info has the shape Proton writes (a version, then the
// fonts and lib paths of the Proton that last ran it).
type tree struct {
	proton, compat string
}

func newTree(t *testing.T) tree {
	t.Helper()
	root := t.TempDir()
	tr := tree{
		proton: filepath.Join(root, "steamapps", "common", "Proton - Test"),
		compat: filepath.Join(root, "steamapps", "compatdata", "1203620"),
	}
	writeExe(t, filepath.Join(tr.proton, "files", "bin", "wine"))
	writeExe(t, filepath.Join(tr.proton, "files", "bin", "wineserver"))
	tr.lastRanWith(t, tr.proton)
	return tr
}

func (tr tree) lastRanWith(t *testing.T, proton string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(tr.compat, 0o750))
	info := "11.0-100\n" + proton + "/files/share/fonts/\n" + proton + "/files/lib/\n"
	require.NoError(t, os.WriteFile(filepath.Join(tr.compat, "config_info"), []byte(info), 0o600))
}

func writeExe(t *testing.T, p string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test executable
}

// systemWine puts a wine on a PATH of its own, or empties PATH.
func systemWine(t *testing.T, present bool) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if !present {
		return ""
	}
	p := filepath.Join(dir, "wine")
	writeExe(t, p)
	return p
}

func TestFind_PrefersTheProtonThatLastRanTheGame(t *testing.T) {
	tr := newTree(t)
	systemWine(t, true)
	w, err := Find(tr.compat)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tr.proton, "files", "bin", "wine"), w.Bin)
	assert.Contains(t, w.Source, "Proton")
	assert.Equal(t, filepath.Join(tr.proton, "files", "bin", "wineserver"), w.Server(),
		"the wineserver beside it, not the system's")
}

func TestFind_FallsBackToSystemWine(t *testing.T) {
	cases := map[string]func(t *testing.T, tr tree){
		"the Proton was uninstalled": func(t *testing.T, tr tree) {
			require.NoError(t, os.RemoveAll(tr.proton))
		},
		"the game never ran under Proton": func(t *testing.T, tr tree) {
			require.NoError(t, os.Remove(filepath.Join(tr.compat, "config_info")))
		},
		"config_info names no Proton": func(t *testing.T, tr tree) {
			require.NoError(t, os.WriteFile(filepath.Join(tr.compat, "config_info"), []byte("11.0-100\n"), 0o600))
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newTree(t)
			breakIt(t, tr)
			sys := systemWine(t, true)
			w, err := Find(tr.compat)
			require.NoError(t, err)
			assert.Equal(t, sys, w.Bin)
			assert.Equal(t, "the system's wine", w.Source)
		})
	}
	t.Run("the game's prefix is not known", func(t *testing.T) {
		sys := systemWine(t, true)
		w, err := Find("")
		require.NoError(t, err)
		assert.Equal(t, sys, w.Bin)
	})
}

func TestFind_NoWineNamesWhatWasTried(t *testing.T) {
	tr := newTree(t)
	require.NoError(t, os.RemoveAll(tr.proton))
	systemWine(t, false)
	_, err := Find(tr.compat)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Contains(t, err.Error(), "no longer installed")
	assert.Contains(t, err.Error(), "wine is not on PATH")
}

func TestEnv_SetsThePrefixAndDropsTheDisplay(t *testing.T) {
	base := []string{"HOME=/h", "DISPLAY=:0", "WAYLAND_DISPLAY=wayland-0", "WINEPREFIX=/game/pfx",
		"WINEDEBUG=+all", "WINEDLLOVERRIDES=x=n", "PATH=/bin"}
	assert.Equal(t, []string{"HOME=/h", "PATH=/bin", "WINEPREFIX=/tools/wineprefix", "WINEDEBUG=-all",
		"WINEDLLOVERRIDES=mscoree,mshtml="}, Env(base, "/tools/wineprefix"))
}

func TestWinPath(t *testing.T) {
	assert.Equal(t, `Z:\home\player\game dir\x.kfc`, WinPath("/home/player/game dir/x.kfc"))
	rel := WinPath("relative")
	assert.Regexp(t, `^Z:\\.+\\relative$`, rel, "relative paths are made absolute first")
}
