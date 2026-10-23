package gui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/core"
)

// stripButtons is what the action strip holds, in order, and what must stay
// affixed in every section that draws it (spec 008).
var stripButtons = []string{"Build", "Deploy…", "Rollback…", "Refresh", "Build without a dry run"}

// readyStatus is a found game whose build passed the gate, with emm, a vanilla
// copy and one build. Each test changes what it is about.
func readyStatus() core.StatusResult {
	return core.StatusResult{
		Install:     core.InstallInfo{Found: true, Dir: "/game"},
		Fingerprint: &compat.Fingerprint{ExeCommit: "abc"},
		Gate:        compat.Decision{Verdict: compat.Derived, Reason: "dry run passed."},
		Emm:         core.EmmInfo{Installed: true},
		Pristine:    core.PristineInfo{Present: true},
		State:       "deployed",
		LastBuild:   core.BuildInfo{Present: true, Current: true},
	}
}

func withStatus(u *ui, st core.StatusResult) {
	u.status, u.statusOK = st, true
	u.report = core.ReportResult{Manifest: &build.Manifest{Mods: []build.ModOutcome{{Name: "stack_sizes", Ran: true}}}}
	u.reportOK = true
}

func drawSection(t *testing.T, u *ui, title string) fyne.CanvasObject {
	t.Helper()
	for _, sec := range sections(u) {
		if sec.Title() == title {
			return sec.Build(u.sh)
		}
	}
	t.Fatalf("no section %q", title)
	return nil
}

// stripIn is the strip buttons a section draws, in the order it draws them.
func stripIn(o fyne.CanvasObject) []string {
	var out []string
	for _, b := range fynetest.All[*widget.Button](o) {
		if slices.Contains(stripButtons, b.Text) {
			out = append(out, b.Text)
		}
	}
	return out
}

func button(t *testing.T, o fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	b := fynetest.FindButton(o, text)
	require.NotNilf(t, b, "no %q button", text)
	return b
}

func TestOverviewAndBuildDrawTheSameAffixedStrip(t *testing.T) {
	for _, verdict := range []string{compat.Derived, compat.Unchecked} {
		u := testUI(t)
		st := readyStatus()
		st.Gate.Verdict = verdict
		withStatus(u, st)
		overview, buildSec := drawSection(t, u, "Overview"), drawSection(t, u, "Build")

		want := stripButtons[:4]
		if verdict == compat.Unchecked {
			want = stripButtons
		}
		require.Equal(t, want, stripIn(overview), verdict)
		require.Equal(t, want, stripIn(buildSec), verdict)
		for _, name := range stripButtons {
			require.NotContains(t, fynetest.ScrolledButtons(overview), name)
			require.NotContains(t, fynetest.ScrolledButtons(buildSec), name)
		}
		for _, name := range want {
			require.Equalf(t, button(t, overview, name).Disabled(), button(t, buildSec, name).Disabled(),
				"%s is enabled differently in the two sections (%s)", name, verdict)
		}
	}
}

// Skipping the dry run skips only the dry run: it still needs emm and the
// vanilla copy, as Build does.
func TestBuildWithoutADryRunNeedsWhatBuildNeeds(t *testing.T) {
	for _, ready := range []bool{true, false} {
		u := testUI(t)
		st := readyStatus()
		st.Gate.Verdict = compat.Unchecked
		st.Pristine.Present = ready
		withStatus(u, st)
		require.Equal(t, !ready, button(t, drawSection(t, u, "Overview"), "Build without a dry run").Disabled())
	}
}

// The build's Cancel is on the busy popup, which covers the strip while a
// build runs; a second one in the section could never be pressed.
func TestBuildSectionHasNoCancelButton(t *testing.T) {
	u := testUI(t)
	withStatus(u, readyStatus())
	require.Nil(t, fynetest.FindButton(drawSection(t, u, "Build"), "Cancel"))
}

func TestLastBuildIsUnderADividerThatKeepsItsPlace(t *testing.T) {
	u := testUI(t)
	withStatus(u, readyStatus())

	split, ok := fynetest.First[*container.Split](drawSection(t, u, "Build"))
	require.True(t, ok, "the Build section has no divider")
	_, hasTable := fynetest.First[*widget.Table](split.Trailing)
	require.True(t, hasTable, "the Last build table is not below the divider")

	split.SetOffset(0.3)
	again, ok := fynetest.First[*container.Split](drawSection(t, u, "Build"))
	require.True(t, ok)
	require.InDelta(t, 0.3, again.Offset, 1e-9, "a rebuild moved the divider")
}

func TestCompatibilityCheckComesAfterWhatItNeeds(t *testing.T) {
	u := testUI(t)
	st := readyStatus()
	st.Gate.Verdict = compat.Unchecked
	withStatus(u, st)
	require.Nil(t, fynetest.FindButton(u.gameCard(), "Run the compatibility check"))

	texts := fynetest.Texts(drawSection(t, u, "Overview"))
	vanilla := slices.Index(texts, "Vanilla copy")
	check := slices.Index(texts, "Run the compatibility check")
	require.Positive(t, vanilla)
	require.Greater(t, check, vanilla)
}

func TestDisabledCompatibilityCheckSaysWhatItNeeds(t *testing.T) {
	for _, c := range []struct {
		emm, vanilla bool
		note         string
	}{
		{false, true, "Needs emm, above."},
		{true, false, "Needs the vanilla copy, above."},
		{false, false, "Needs emm and the vanilla copy, above."},
		{true, true, ""},
	} {
		u := testUI(t)
		st := readyStatus()
		st.Gate.Verdict = compat.Unchecked
		st.Emm.Installed, st.Pristine.Present = c.emm, c.vanilla
		withStatus(u, st)
		card := u.compatCard()

		require.Equal(t, c.note != "", button(t, card, "Run the compatibility check").Disabled(), c.note)
		var needs []string
		for _, s := range fynetest.Texts(card) {
			if len(s) > 6 && s[:6] == "Needs " {
				needs = append(needs, s)
			}
		}
		if c.note == "" {
			require.Empty(t, needs)
		} else {
			require.Equal(t, []string{c.note}, needs)
		}
	}
}
