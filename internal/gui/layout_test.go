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

// stripButtons is what Advanced's strip holds, in order (spec 010 R12).
var stripButtons = []string{"Build", "Deploy…", "Refresh", "Build without a dry run"}

// separateButtons are the controls that moved off Overview (spec 010 AC15).
var separateButtons = []string{"Build", "Deploy…", "Rollback…", "Build without a dry run",
	"Capture from the install", "Import a backup…", "Run the compatibility check", "Get emm"}

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
		Enabled:     []string{"big_stacks"},
		Mods:        core.ModsInfo{State: core.ModsApplied, Sentence: "Your mod is in the game."},
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

// Overview's only job is the one press. Everything else lives on Advanced.
func TestOverviewHasOneMainButtonAndNoSeparateSteps(t *testing.T) {
	for _, verdict := range []string{compat.Derived, compat.Unchecked} {
		for _, present := range []bool{true, false} {
			u := testUI(t)
			st := readyStatus()
			st.Gate.Verdict, st.Emm.Installed, st.Pristine.Present = verdict, present, present
			withStatus(u, st)
			overview := drawSection(t, u, "Overview")

			for _, name := range separateButtons {
				require.Nilf(t, fynetest.FindButton(overview, name), "%s is on Overview", name)
			}
			var high []string
			for _, b := range fynetest.All[*widget.Button](overview) {
				if b.Importance == widget.HighImportance {
					high = append(high, b.Text)
				}
			}
			require.Equal(t, []string{"Apply mods"}, high)
			button(t, overview, "Remove mods…")
			button(t, overview, "Refresh")
			require.False(t, button(t, overview, "Apply mods").Disabled(), "Apply gets what is missing itself")
			for _, name := range []string{"Apply mods", "Remove mods…", "Refresh"} {
				require.NotContains(t, fynetest.ScrolledButtons(overview), name)
			}
		}
	}
}

// One button, one place: the row is the same size whatever the button says.
func TestMainButtonKeepsItsPlace(t *testing.T) {
	u := testUI(t)
	withStatus(u, readyStatus())
	applyRow := u.mainActions()
	require.Equal(t, "Apply mods", fynetest.All[*widget.Button](applyRow)[0].Text)

	st := readyStatus()
	st.Mods.State = core.ModsNeedsVerify
	withStatus(u, st)
	verifyRow := u.mainActions()
	verify := fynetest.All[*widget.Button](verifyRow)[0]
	require.Equal(t, "Verify in Steam", verify.Text)
	require.False(t, verify.Disabled())
	require.Equal(t, applyRow.MinSize(), verifyRow.MinSize())
	require.Nil(t, fynetest.FindButton(verifyRow, "Apply mods"))
}

func TestMainButtonIsDisabledWhenItCouldNotRun(t *testing.T) {
	for name, change := range map[string]func(st *core.StatusResult){
		"game running": func(st *core.StatusResult) { st.Install.RunningKnown, st.Install.Running = true, true },
		"no game": func(st *core.StatusResult) {
			st.Install.Found = false
			st.Mods.State = core.ModsNoGame
		},
		"nothing enabled": func(st *core.StatusResult) { st.Mods.State = core.ModsNoneEnabled },
	} {
		u := testUI(t)
		st := readyStatus()
		change(&st)
		withStatus(u, st)
		require.Truef(t, fynetest.All[*widget.Button](u.mainActions())[0].Disabled(), name)
	}
}

func TestRemoveModsIsDisabledWhenThereIsNothingToRemove(t *testing.T) {
	for _, c := range []struct {
		state    string
		present  bool
		disabled bool
	}{{"deployed", true, false}, {"vanilla", true, true}, {"unknown", false, true}} {
		u := testUI(t)
		st := readyStatus()
		st.State, st.Pristine.Present = c.state, c.present
		withStatus(u, st)
		require.Equal(t, c.disabled, button(t, u.mainActions(), "Remove mods…").Disabled(), c.state)
	}
}

func TestAdvancedHoldsTheSeparateSteps(t *testing.T) {
	require.Contains(t, SectionNames(), "Advanced")
	require.NotContains(t, SectionNames(), "Build")
	for _, verdict := range []string{compat.Derived, compat.Unchecked} {
		u := testUI(t)
		st := readyStatus()
		st.Gate.Verdict = verdict
		withStatus(u, st)
		adv := drawSection(t, u, "Advanced")

		want := stripButtons[:3]
		if verdict == compat.Unchecked {
			want = stripButtons
		}
		require.Equal(t, want, stripIn(adv), verdict)
		for _, name := range stripButtons {
			require.NotContains(t, fynetest.ScrolledButtons(adv), name)
		}
		texts := fynetest.Texts(adv)
		for _, card := range []string{"Game", "emm", "Vanilla copy", "Compatibility", "Deployed", "Last build"} {
			require.Contains(t, texts, card)
		}
	}
}

// Every operation a press can start is a button in some state of some section;
// the parity test holds the same list against the CLI.
func TestOperationsAreReachable(t *testing.T) {
	st := readyStatus()
	st.Emm.Installed, st.Pristine.Present = false, false
	st.Gate.Verdict = compat.Unchecked
	u := testUI(t)
	withStatus(u, st)
	missing := drawSection(t, u, "Advanced")
	for _, name := range []string{"Get emm", "Capture from the install", "Import a backup…"} {
		button(t, missing, name)
	}
	u = testUI(t)
	st = readyStatus()
	st.Gate.Verdict = compat.Unchecked
	withStatus(u, st)
	adv := drawSection(t, u, "Advanced")
	for _, name := range []string{"Build", "Deploy…", "Run the compatibility check"} {
		button(t, adv, name)
	}
	overview := drawSection(t, u, "Overview")
	for _, name := range []string{"Apply mods", "Remove mods…"} {
		button(t, overview, name)
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
		require.Equal(t, !ready, button(t, drawSection(t, u, "Advanced"), "Build without a dry run").Disabled())
	}
}

// A run's Cancel is on the busy popup, which covers everything while it runs; a
// second one in a section could never be pressed.
func TestNoSectionHasACancelButton(t *testing.T) {
	u := testUI(t)
	withStatus(u, readyStatus())
	require.Nil(t, fynetest.FindButton(drawSection(t, u, "Overview"), "Cancel"))
	require.Nil(t, fynetest.FindButton(drawSection(t, u, "Advanced"), "Cancel"))
}

func TestLastBuildIsUnderADividerThatKeepsItsPlace(t *testing.T) {
	u := testUI(t)
	withStatus(u, readyStatus())

	split, ok := fynetest.First[*container.Split](drawSection(t, u, "Advanced"))
	require.True(t, ok, "Advanced has no divider")
	_, hasTable := fynetest.First[*widget.Table](split.Trailing)
	require.True(t, hasTable, "the Last build table is not below the divider")

	split.SetOffset(0.3)
	again, ok := fynetest.First[*container.Split](drawSection(t, u, "Advanced"))
	require.True(t, ok)
	require.InDelta(t, 0.3, again.Offset, 1e-9, "a rebuild moved the divider")
}

func TestCompatibilityCheckComesAfterWhatItNeeds(t *testing.T) {
	u := testUI(t)
	st := readyStatus()
	st.Gate.Verdict = compat.Unchecked
	withStatus(u, st)
	require.Nil(t, fynetest.FindButton(u.gameCard(), "Run the compatibility check"))

	texts := fynetest.Texts(drawSection(t, u, "Advanced"))
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
