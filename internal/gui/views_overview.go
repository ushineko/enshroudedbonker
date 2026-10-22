package gui

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/core"
)

/*
buildOverview answers "can this machine build, and what is in the game".

Four cards, in the order the pipeline needs them: the game and its build, the
tool, the vanilla copy, and what is deployed. A missing piece explains itself
with the action that fixes it, rather than turning into a failed build later.
*/
func (u *ui) buildOverview() fyne.CanvasObject {
	u.loadStatus()
	body := container.NewVBox(
		widgets.Heading("Overview", "What this machine has, and what is in the game."),
		u.gameCard(), widget.NewSeparator(),
		u.emmCard(), widget.NewSeparator(),
		u.pristineCard(), widget.NewSeparator(),
		u.deployCard(),
	)
	return container.NewBorder(nil, u.overviewActions(), nil, nil, u.sh.VScroll("overview", body))
}

func (u *ui) gameCard() fyne.CanvasObject {
	st := u.status
	if !u.statusOK {
		return widgets.Card("Game", widget.NewLabel("Reading the install…"))
	}
	if !st.Install.Found {
		return widgets.Card("Game", u.noGameBlock(st.Install.Error))
	}
	rows := []fyne.CanvasObject{
		widgets.PlainRow("Directory", st.Install.Dir),
		widgets.PlainRow("Found by", st.Install.Source),
		widgets.PlainRow("Steam build", widgets.OrNone(st.Install.BuildID, "no appmanifest")),
	}
	if st.Fingerprint != nil {
		rows = append(rows, widgets.PlainRow("Game build", st.Fingerprint.Short()))
	}
	rows = append(rows, widgets.FactRow("Compatibility", st.Gate.Verdict, gateStatus(st.Gate.Verdict)),
		widgets.Note(st.Gate.Reason, gateStatus(st.Gate.Verdict)))
	if st.Gate.Verdict != compat.Verified && st.Gate.Verdict != compat.Inconsistent {
		check := widget.NewButtonWithIcon("Run the compatibility check", theme.SearchIcon(), func() { u.compatCheck() })
		u.sh.Gate(check)
		if !st.Emm.Installed || !st.Pristine.Present {
			check.Disable()
		}
		rows = append(rows, container.NewHBox(check))
	}
	if st.Install.RunningKnown && st.Install.Running {
		rows = append(rows, widgets.Note("Enshrouded is running. Deploy and rollback wait until it is closed.",
			fd.StatusWarn))
	}
	return widgets.Card("Game", rows...)
}

func (u *ui) noGameBlock(reason string) fyne.CanvasObject {
	body := container.NewVBox(
		widgets.FactRow("Game", "not found", fd.StatusBad),
		widgets.Note(reason+". Set the directory that holds enshrouded.exe below; it is remembered "+
			"in the settings, where the command line reads it too.", fd.StatusInfo),
	)
	dir := widget.NewEntry()
	dir.SetPlaceHolder("the Enshrouded directory, the one holding enshrouded.exe")
	save := widget.NewButtonWithIcon("Use this directory", theme.ConfirmIcon(), func() {
		u.setConfig("game_dir", dir.Text)
	})
	body.Add(container.NewBorder(nil, nil, nil, save, dialogs.WithBrowse(u.sh.Window, dir, true)))

	res, err := core.Detect(context.Background(), core.DetectRequest{Request: u.request()})
	if err == nil && len(res.Candidates) > 0 {
		t := table.New()
		t.Header("Steam library", "Game directory", "Verdict")
		for _, c := range res.Candidates {
			lib := widgets.OrNone(c.LibraryDir, widgets.OrNone(c.Root, "(explicit game directory)"))
			verdict, st := "ok", fd.StatusGood
			if c.Reason != "" {
				verdict, st = "rejected: "+c.Reason, fd.StatusWarn
			}
			t.Row(st, lib, c.GameDir, verdict)
		}
		body.Add(widget.NewLabelWithStyle("Where enshroudedbonker looked", fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true}))
		body.Add(widgets.FixedHeight(t.Widget(), 160))
	}
	return body
}

func (u *ui) emmCard() fyne.CanvasObject {
	e := u.status.Emm
	if !u.statusOK {
		return widgets.Card("emm", widget.NewLabel("Looking for emm…"))
	}
	if !e.Installed {
		block, get := widgets.Action("Get emm "+e.PinnedTag,
			"emm is upstream's Enshrouded Mod Manager, which applies the mods to the archive. This "+
				"downloads upstream's own release (about 21 MB) into the tools directory and checks "+
				"it before keeping it. It runs under the same Proton as the game; nothing is "+
				"installed system-wide.",
			"Get emm", false, func() { u.getEmm() })
		get.Importance = widget.HighImportance
		u.sh.Gate(get)
		return widgets.Card("emm", widgets.FactRow("emm", "not installed", fd.StatusBad), block)
	}
	status := fd.StatusGood
	if e.Error != "" {
		status = fd.StatusBad
	}
	rows := []fyne.CanvasObject{
		widgets.FactRow("emm", widgets.OrNone(e.Version, "installed"), status),
		widgets.PlainRow("From", e.Source),
		widgets.PlainRow("Path", e.Path),
	}
	if e.Wine != nil {
		rows = append(rows, widgets.PlainRow("Runs under", e.Wine.Bin+" ("+e.Wine.Source+")"))
	}
	if e.Error != "" {
		rows = append(rows, widgets.PlainRow("Problem", e.Error))
	}
	return widgets.Card("emm", rows...)
}

func (u *ui) pristineCard() fyne.CanvasObject {
	p := u.status.Pristine
	if !u.statusOK || !u.status.Install.Found {
		return widgets.Card("Vanilla copy", widget.NewLabel("Needs the game."))
	}
	if p.Present {
		return widgets.Card("Vanilla copy",
			widgets.FactRow("Held", p.Source, fd.StatusGood),
			widgets.PlainRow("Captured", p.Captured),
			widgets.DimWrapped("Every build starts from this copy, never from the install, so patches "+
				"never stack on top of each other."))
	}
	rows := []fyne.CanvasObject{widgets.FactRow("Held", "none for this game build", fd.StatusWarn)}
	capture := widget.NewButtonWithIcon("Capture from the install", theme.DownloadIcon(), func() { u.capturePristine() })
	importBtn := widget.NewButtonWithIcon("Import a backup…", theme.FolderOpenIcon(), func() { u.importPristineDialog() })
	u.sh.Gate(capture, importBtn)
	if len(p.Patched) > 0 {
		capture.Disable()
		rows = append(rows, widgets.Note("The install looks patched ("+strings.Join(p.Patched, "; ")+"), so it "+
			"cannot be captured as vanilla. Restore it with Steam's Verify integrity of game files, "+
			"or import a backup of the vanilla enshrouded.kfc and enshrouded.kfc_resources.", fd.StatusWarn))
	} else {
		rows = append(rows, widgets.DimWrapped("A copy of enshrouded.kfc and enshrouded.kfc_resources "+
			"(about 225 MB) is kept in the workspace. The game is not touched."))
	}
	rows = append(rows, container.NewHBox(capture, importBtn))
	return widgets.Card("Vanilla copy", rows...)
}

func (u *ui) deployCard() fyne.CanvasObject {
	st := u.status
	if !u.statusOK || !st.Install.Found {
		return widgets.Card("Deployed", widget.NewLabel("Needs the game."))
	}
	rows := []fyne.CanvasObject{widgets.FactRow("Install", st.State, stateStatus(st.State))}
	if st.Deployed != nil {
		rows = append(rows, widgets.PlainRow("Deployed", st.Deployed.When.Format("2006-01-02 15:04")),
			widgets.DimWrapped(strings.Join(st.Deployed.Mods, ", ")))
	}
	if st.State == "modified" {
		rows = append(rows, widgets.Note("The archive is neither vanilla nor what this tool last deployed. "+
			"Another tool may have patched it; a deploy keeps a copy first.", fd.StatusWarn))
	}
	if st.LastBuild.Present {
		lb := "built " + st.LastBuild.Created + ", " + itoa(st.LastBuild.Applied) + " patches"
		s := fd.StatusGood
		if !st.LastBuild.Current {
			lb += ", for a different game build"
			s = fd.StatusWarn
		}
		rows = append(rows, widgets.FactRow("Last build", lb, s))
	} else {
		rows = append(rows, widgets.FactRow("Last build", "none yet", fd.StatusInfo))
	}
	rows = append(rows, widgets.DimWrapped("Single player and worlds you host. Stack and chest sizes stay "+
		"in a save after a rollback."))
	return widgets.Card("Deployed", rows...)
}

func (u *ui) overviewActions() fyne.CanvasObject {
	build := widget.NewButtonWithIcon("Build", theme.MediaPlayIcon(), func() {
		u.sh.Select("Build")
		u.startBuild(false)
	})
	build.Importance = widget.HighImportance
	deploy := widget.NewButtonWithIcon("Deploy…", theme.DownloadIcon(), func() { u.deployLast() })
	deploy.Importance = widget.DangerImportance
	rollback := widget.NewButtonWithIcon("Rollback…", theme.ContentUndoIcon(), func() { u.rollback() })
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { u.sh.Invalidate() })
	u.sh.Gate(build, deploy, rollback, refresh)
	st := u.status
	if !st.Install.Found || !st.Emm.Installed || !st.Pristine.Present || !st.Gate.Buildable() {
		build.Disable()
	}
	if !u.canDeploy() {
		deploy.Disable()
	}
	if !st.Pristine.Present || st.State == "vanilla" {
		rollback.Disable()
	}
	return container.NewVBox(widget.NewSeparator(), container.NewHBox(build, deploy, rollback, refresh))
}

func (u *ui) canDeploy() bool {
	st := u.status
	return st.Install.Found && st.LastBuild.Present && st.LastBuild.Current && st.Pristine.Present
}

// --- operations --------------------------------------------------------------

func (u *ui) compatCheck() {
	u.perform("Dry-running every tweak against a copy of the archive…", "", func(ctx context.Context) error {
		res, err := core.CompatCheck(ctx, core.CompatCheckRequest{Request: u.request()})
		if err != nil {
			return err
		}
		failed := 0
		for _, t := range res.DryRun.Tweaks {
			if !t.OK {
				failed++
			}
		}
		fyne.Do(func() {
			if failed > 0 {
				u.sh.Flash(itoa(failed)+" tweak(s) did not apply to this game build and are disabled for it. "+
					res.Decision.Reason, fd.StatusWarn)
				return
			}
			u.sh.Flash("Every tweak applied in the dry run. "+res.Decision.Reason, gateStatus(res.Decision.Verdict))
		})
		return nil
	})
}

func (u *ui) getEmm() {
	u.perform("Downloading emm and setting up its Wine prefix…", "", func(ctx context.Context) error {
		res, err := core.GetEmm(ctx, core.GetEmmRequest{Request: u.request()})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.sh.OK("Got " + res.Version + ". It is ready to build.") })
		return nil
	})
}

func (u *ui) capturePristine() {
	u.perform("Copying the vanilla archive…", "The vanilla copy is held. Builds start from it.",
		func(ctx context.Context) error {
			_, err := core.PristineCapture(ctx, core.PristineCaptureRequest{Request: u.request()})
			return err
		})
}

func (u *ui) importPristineDialog() {
	dialogs.ChooseFolder(u.sh.Window, "", func(dir string) {
		u.perform("Importing the vanilla archive…", "The vanilla copy is held. Builds start from it.",
			func(ctx context.Context) error {
				_, err := core.PristineImport(ctx, core.PristineImportRequest{Request: u.request(), Dir: dir})
				return err
			})
	})
}

func (u *ui) deployLast() {
	st := u.status
	body := container.NewVBox(
		widgets.Wrapped("The last build is copied into the game: enshrouded.kfc, enshrouded.kfc_resources, "+
			"and the empty containers emm adds. Each file is written under a temporary name and renamed "+
			"into place."),
		widgets.PlainRow("Into", st.Install.Dir),
		widgets.Wrapped("Quit the game first. Not touched: your saves and the game's other files."),
		widgets.Note("Stack and chest sizes the mods allow stay in a save after a rollback. Do not join "+
			"other people's servers with a patched game.", fd.StatusWarn),
	)
	dialogs.ConfirmWithBody(u.sh.Window, "Deploy the last build?", body, "Deploy", func() {
		u.perform("Writing into the game…", "Deployed. Launch the game to play with it.",
			func(ctx context.Context) error {
				req := u.request()
				req.Events = core.Events{Log: func(level core.Level, msg string) {
					u.run.pane.Model().Append(logLevel(level), msg)
				}}
				_, err := core.Deploy(ctx, core.DeployRequest{Request: req})
				fyne.Do(u.run.pane.Draw)
				return err
			})
	}).Show()
}

func (u *ui) rollback() {
	dialogs.ConfirmDestructive(u.sh.Window, "Put the vanilla archive back?",
		"The vanilla copy of enshrouded.kfc and enshrouded.kfc_resources is written back and the "+
			"containers emm added are removed. Quit the game first. Bigger stacks and chests already "+
			"in your save stay as they are.",
		"Rollback", func() {
			u.perform("Restoring the vanilla archive…", "The game is vanilla again.",
				func(ctx context.Context) error {
					_, err := core.Rollback(ctx, core.RollbackRequest{Request: u.request()})
					return err
				})
		})
}

func (u *ui) setConfig(key, value string) {
	u.perform("Saving "+key+"…", "Saved "+key+".", func(ctx context.Context) error {
		_, err := core.ConfigSet(ctx, core.ConfigSetRequest{Request: u.request(), Key: key, Value: value})
		return err
	})
}
