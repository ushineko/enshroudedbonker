package gui

import (
	"context"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/forms"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/enshroudedbonker/internal/core"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// tweaksSection is one tab per group.
func tweaksSection(u *ui, icon func() fyne.Resource) shell.Section {
	groups := groupsInOrder()
	parts := make([]shell.Section, 0, len(groups))
	for _, page := range groups {
		parts = append(parts, shell.NewSection(page, nil, func(*shell.Shell) fyne.CanvasObject {
			return u.buildTweakPage(page)
		}))
	}
	return shell.NewTabs("Tweaks", icon, parts...)
}

//nolint:gochecknoglobals // a fixed table, read-only after initialisation
var tweakPageBlurbs = map[string]string{
	"Resources": "What gathering, chopping, mining and digging yield.",
	"Loot":      "What enemies and chests drop, and how rare it is.",
	"Inventory": "How much a stack and a chest hold.",
	"Crafting":  "Crafting time, salvage returns, upgrade costs and station storage.",
	"World":     "Where you can fast travel from.",
}

/*
buildTweakPage is one group's tweaks as cards. A tweak that is switched off keeps
its slot, and the "changed since the last build" line is reserved whether or
not it has text, so nothing moves under the pointer.
*/
func (u *ui) buildTweakPage(page string) fyne.CanvasObject {
	u.loadTweaks()
	if !u.tweaksOK || u.tweaks.Tweaks == nil {
		return container.NewVScroll(container.NewVBox(widgets.Heading(page, "Reading the built-in tweaks…")))
	}
	body := container.NewVBox(widgets.Heading(page, tweakPageBlurbs[page]+
		" Turn one on, set what it does, and build."))
	n := 0
	for _, tw := range u.tweaks.Tweaks {
		if tw.Group != page {
			continue
		}
		if n > 0 {
			body.Add(widget.NewSeparator())
		}
		body.Add(u.tweakCard(tw))
		n++
	}
	if n == 0 {
		body.Add(widgets.DimWrapped("No built-in tweaks in this group."))
	}
	return container.NewBorder(nil, u.tweaksActions(), nil, nil, u.sh.VScroll("tweaks/"+page, body))
}

func (u *ui) tweakCard(tw core.TweakInfo) fyne.CanvasObject {
	on := widget.NewCheck(tw.Title, nil)
	on.Checked = tw.Enabled
	on.OnChanged = func(checked bool) { u.setModsEnabled([]string{tw.Name}, checked) }
	if u.sh.Working() {
		on.Disable()
	}

	unbuilt := widget.NewLabel("")
	unbuilt.Importance = widget.WarningImportance
	head := container.NewBorder(nil, nil, container.NewHBox(on, widgets.Dim(tw.Name)), nil, unbuilt)

	desc := widget.NewLabel(tw.Desc)
	desc.Wrapping = fyne.TextWrapWord
	desc.Importance = widget.LowImportance

	rows := []fyne.CanvasObject{head, desc}
	if tw.Disabled != "" {
		on.Disable()
		rows = append(rows, widgets.Note("Disabled for this game build: "+tw.Disabled, fd.StatusWarn))
	}
	for _, p := range tw.Params {
		rows = append(rows, u.paramRow(tw.Name, p, unbuilt))
	}
	return container.NewVBox(rows...)
}

/*
paramRow is one parameter. Numbers get the library's slider-and-field pair,
which commits once on release or Enter; a bool gets a check.
*/
func (u *ui) paramRow(mod string, p core.TweakParam, unbuilt *widget.Label) fyne.CanvasObject {
	label := widget.NewLabel(p.Label)
	label.Importance = widget.LowImportance
	format := func(v float64) string { return tweaks.FormatValue(v, p.Kind) }
	def := widgets.Dim("default " + format(p.Default))
	reset := widget.NewButtonWithIcon("Reset", theme.ContentUndoIcon(), nil)
	reset.Importance = widget.LowImportance
	u.sh.Gate(reset)

	var (
		control fyne.CanvasObject
		show    func(float64)
	)
	if p.Kind == tweaks.KindBool {
		chk := widget.NewCheck("", nil)
		chk.Checked = p.Current != 0
		chk.OnChanged = func(b bool) {
			v := 0.0
			if b {
				v = 1
			}
			u.applyParam(mod, p, v, unbuilt, show)
		}
		show = func(v float64) {
			if want := v != 0; chk.Checked != want {
				chk.Checked = want
				chk.Refresh()
			}
		}
		control = chk
	} else {
		var pair *forms.SliderEntry
		pair = forms.NewSliderEntry(forms.SliderOptions{
			Min: p.Min, Max: p.Max, Step: p.Step, Value: p.Current,
			Format: format,
			Commit: func(v float64) { u.applyParam(mod, p, v, unbuilt, pair.Set) },
			OnInvalid: func(text string) {
				u.sh.Flash(p.Label+": "+strconv.Quote(text)+" is not a number.", fd.StatusWarn)
			},
		})
		control, show = pair.Widget(), pair.Set
	}
	reset.OnTapped = func() { u.resetParam(mod, p, unbuilt, show) }

	return container.NewBorder(nil, nil,
		widgets.FixedWidth(label, 240),
		container.NewHBox(widgets.FixedWidth(def, 130), reset),
		control)
}

// applyParam writes one parameter and updates the card in place, so the
// control being used keeps focus and the page keeps its scroll.
func (u *ui) applyParam(mod string, p core.TweakParam, v float64, unbuilt *widget.Label, show func(float64)) {
	u.sh.Perform(fmt.Sprintf("Setting %s %s…", mod, p.Name), func(ctx context.Context) error {
		res, err := core.SetTweakParam(ctx, core.SetTweakParamRequest{
			Request: u.request(), Name: mod, Param: p.Name, Value: v,
		})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			show(res.New)
			u.noteParam(mod, p.Name, res.New, res.New != p.Default)
			unbuilt.SetText("changed — build to apply")
			if res.Clamped {
				u.sh.Flash(fmt.Sprintf("%s only takes %s to %s, so %s was used.", p.Label,
					format(p, p.Min), format(p, p.Max), format(p, res.New)), fd.StatusWarn)
			}
		})
		return nil
	})
}

func format(p core.TweakParam, v float64) string { return tweaks.FormatValue(v, p.Kind) }

func (u *ui) resetParam(mod string, p core.TweakParam, unbuilt *widget.Label, show func(float64)) {
	u.sh.Perform("Resetting "+mod+" "+p.Name+"…", func(ctx context.Context) error {
		if _, err := core.ResetTweak(ctx, core.ResetTweakRequest{Request: u.request(), Name: mod, Param: p.Name}); err != nil {
			return err
		}
		fyne.Do(func() {
			show(p.Default)
			u.noteParam(mod, p.Name, p.Default, false)
			unbuilt.SetText("changed — build to apply")
		})
		return nil
	})
}

// noteParam keeps the loaded model in step with the screen. UI thread.
func (u *ui) noteParam(mod, param string, value float64, overridden bool) {
	for i := range u.tweaks.Tweaks {
		if u.tweaks.Tweaks[i].Name != mod {
			continue
		}
		for j := range u.tweaks.Tweaks[i].Params {
			if u.tweaks.Tweaks[i].Params[j].Name == param {
				u.tweaks.Tweaks[i].Params[j].Current = value
				u.tweaks.Tweaks[i].Params[j].Overridden = overridden
			}
		}
	}
}

func (u *ui) tweaksActions() fyne.CanvasObject {
	build := widget.NewButtonWithIcon("Build", theme.MediaPlayIcon(), func() {
		u.sh.Select("Build")
		u.startBuild(false)
	})
	build.Importance = widget.HighImportance
	u.sh.Gate(build)
	return container.NewVBox(widget.NewSeparator(), container.NewHBox(build,
		widgets.Dim("Values are kept in the settings file; the command line reads the same ones.")))
}
