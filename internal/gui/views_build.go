package gui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/enshroudedbonker/internal/compat"
)

// Fixed geometry: the run writes into these several times a second.
const (
	stepColumnWidth = 330
	logPaneHeight   = 380
	totalsHeight    = 74
	reportHeight    = 180
)

/*
buildBuild is the step list and the live log, then the last build's manifest.
Building and deploying are separate buttons so the moment between them -- the
one where the output says a mod did not apply -- is not skipped.
*/
func (u *ui) buildBuild() fyne.CanvasObject {
	u.loadStatus()
	u.loadReport()
	head := widgets.Heading("Build",
		"Run every enabled mod through emm against a copy of the vanilla archive. Nothing reaches "+
			"the game until you press Deploy.")

	build := widget.NewButtonWithIcon("Build", theme.MediaPlayIcon(), func() { u.startBuild(false) })
	build.Importance = widget.HighImportance
	cancel := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() { u.cancelBuild() })
	cancel.Importance = widget.DangerImportance
	deploy := widget.NewButtonWithIcon("Deploy…", theme.DownloadIcon(), func() { u.deployLast() })
	deploy.Importance = widget.DangerImportance
	u.sh.Gate(build)
	if u.sh.Working() || !u.canDeploy() {
		deploy.Disable()
	}
	if !u.run.running || u.run.cancelled {
		cancel.Disable()
	}
	toolbar := container.NewHBox(build, cancel, deploy)
	if u.statusOK && u.status.Gate.Verdict == compat.Unchecked {
		force := widget.NewButtonWithIcon("Build without a dry run", theme.WarningIcon(), func() { u.startBuild(true) })
		u.sh.Gate(force)
		toolbar.Add(force)
	}

	totals := widget.NewLabel("")
	totals.Wrapping = fyne.TextWrapWord
	u.run.totals = totals
	stepCol := container.NewBorder(
		widget.NewLabelWithStyle("Steps", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widgets.FixedHeight(totals, totalsHeight), nil, nil, u.run.steps.Widget())
	u.drawTotals()

	log := u.run.pane.Widget(logpane.Options{
		Title: "Output", Height: logPaneHeight, Clipboard: u.sh.App.Clipboard(), Flash: u.sh.Flash,
	})
	return container.NewBorder(container.NewVBox(head, toolbar), u.reportBlock(), nil, nil,
		container.NewBorder(nil, nil, widgets.FixedWidth(stepCol, stepColumnWidth), nil, log))
}

func (u *ui) drawTotals() {
	if u.run.totals != nil {
		u.run.totals.SetText(u.buildTotals())
	}
}

func (u *ui) buildTotals() string {
	switch {
	case u.run.running:
		return fmt.Sprintf("Running. %d line(s) of output so far.", u.run.pane.Model().Len())
	case u.run.finished:
		return u.run.summary
	case u.report.Manifest != nil:
		m := u.report.Manifest
		return fmt.Sprintf("Last build %s: %d mod(s), %d patches.", widgets.HumanAgo(m.Created), len(m.Mods), m.Applied)
	}
	return "Nothing has been built yet."
}

// reportBlock is the last build's per-mod outcome: what each mod said it did.
func (u *ui) reportBlock() fyne.CanvasObject {
	m := u.report.Manifest
	if m == nil {
		return widget.NewLabel("")
	}
	t := table.New()
	t.Header("Mod", "Ran", "Settings", "What it reported")
	for _, mo := range m.Mods {
		st, ran := fd.StatusGood, "yes"
		if !mo.Ran {
			st, ran = fd.StatusWarn, "no"
		}
		params := make([]string, 0, len(mo.Params))
		for k, v := range mo.Params {
			params = append(params, fmt.Sprintf("%s=%g", k, v))
		}
		t.Row(st, mo.Name, ran, strings.Join(params, " "), strings.Join(mo.Lines, "; "))
	}
	t.SetWidths(190, 50, 260, 600)
	rows := []fyne.CanvasObject{
		widget.NewLabelWithStyle("Last build", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widgets.FixedHeight(t.Widget(), reportHeight),
	}
	if len(m.Overlaps) > 0 {
		rows = append(rows, widgets.Note("Mods that write the same resource type run in an order emm "+
			"picks at random: "+strings.Join(m.Overlaps, "; "), fd.StatusWarn))
	}
	if len(m.Order) > 0 {
		rows = append(rows, widgets.DimWrapped("emm ran them in this order: "+strings.Join(m.Order, ", ")))
	}
	return container.NewVBox(rows...)
}
