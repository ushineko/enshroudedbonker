package gui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/core"
)

// testUI is the program's state over a headless shell, with HOME, XDG and the
// Steam root pointed at throwaway paths. Sections load inline.
func testUI(t *testing.T) *ui {
	t.Helper()
	fynetest.Sandbox(t)
	t.Setenv("STEAM_ROOT", filepath.Join(t.TempDir(), "no-steam-here"))
	t.Setenv("ENSHROUDEDBONKER_GAME_DIR", "")
	app := test.NewApp()
	t.Cleanup(app.Quit)
	u := &ui{version: "test", commit: "0000000"}
	u.run.init()
	u.sh = shell.Headless(app, u.shellOptions(Options{}))
	win := test.NewWindow(widget.NewLabel(""))
	u.sh.Window = win
	t.Cleanup(win.Close)
	return u
}

func TestSectionNamesHaveBuilders(t *testing.T) {
	names := SectionNames()
	for _, n := range names {
		require.Containsf(t, sectionBuilders(), n, "%q is advertised but has no section", n)
	}
	require.Len(t, sectionBuilders(), len(names))
	require.NotEmpty(t, SchemeNames())
}

// With no game installed every section still draws and says why, rather than
// panicking on a nil fingerprint.
func TestEverySectionBuildsWithNoGame(t *testing.T) {
	u := testUI(t)
	u.loadStatus()
	require.True(t, u.statusOK)
	require.False(t, u.status.Install.Found)
	for _, sec := range sections(u) {
		require.NotNilf(t, sec.Build(u.sh), "section %s", sec.Title())
	}
	require.NotEmpty(t, u.statusSegments())
}

func TestTweaksLoadEveryGroup(t *testing.T) {
	u := testUI(t)
	u.loadTweaks()
	require.True(t, u.tweaksOK)
	seen := map[string]bool{}
	for _, tw := range u.tweaks.Tweaks {
		seen[tw.Group] = true
	}
	for _, g := range groupsInOrder() {
		require.Truef(t, seen[g], "group %s has no tweaks", g)
		require.NotEmptyf(t, tweakPageBlurbs[g], "group %s has no blurb", g)
	}
}

func TestStepForCoversEveryBuildPhase(t *testing.T) {
	for what, want := range map[string]int{
		"checking the game build":             stepGate,
		"preparing the shadow game directory": stepShadow,
		"running emm":                         stepEmm,
		"done":                                stepCollect,
	} {
		got, ok := stepFor(what)
		require.True(t, ok, what)
		require.Equal(t, want, got, what)
	}
	_, ok := stepFor("something else")
	require.False(t, ok)
}

func TestBuildSummaryWarnsWhenAModDidNotRun(t *testing.T) {
	_, st := buildSummary(core.BuildResult{Manifest: &build.Manifest{
		Mods: []build.ModOutcome{{Name: "a", Ran: true}}, Applied: 3,
	}})
	require.Equal(t, fd.StatusGood, st)
	msg, st := buildSummary(core.BuildResult{Manifest: &build.Manifest{
		Mods: []build.ModOutcome{{Name: "a", Ran: true}, {Name: "b"}},
	}})
	require.Equal(t, fd.StatusWarn, st)
	require.Contains(t, msg, "b")
	_, st = buildSummary(core.BuildResult{})
	require.Equal(t, fd.StatusWarn, st)
}
