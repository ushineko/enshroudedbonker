package gui

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/enshroudedbonker/internal/core"
)

/*
buildMods is every mod the build can run: the built-ins (whose settings live in
Tweaks) and third-party EML mods imported into the library. A library mod starts
disabled; turning it on is a deliberate act.
*/
func (u *ui) buildMods() fyne.CanvasObject {
	u.loadMods()
	head := widgets.Heading("Mods", "Built-in tweaks and EML mods you have imported. Imported mods "+
		"stay in your library directory and are never shared or uploaded.")
	if !u.modsOK || u.mods.Mods == nil {
		return container.NewBorder(head, nil, nil, nil, widget.NewLabel("Reading the mod library…"))
	}
	body := container.NewVBox()
	var builtin, library []core.ModInfo
	for _, m := range u.mods.Mods {
		if m.Builtin {
			builtin = append(builtin, m)
		} else {
			library = append(library, m)
		}
	}
	body.Add(widget.NewLabelWithStyle("Library", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	if len(library) == 0 {
		body.Add(widgets.DimWrapped("No mods imported. Import a mod folder (mod.json and src/mod.lua) " +
			"or a .zip of one."))
	}
	for _, m := range library {
		body.Add(u.modRow(m))
	}
	body.Add(widget.NewSeparator())
	body.Add(widget.NewLabelWithStyle("Built-in", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	for _, m := range builtin {
		body.Add(u.modRow(m))
	}

	importDir := widget.NewButtonWithIcon("Import folder…", theme.FolderOpenIcon(), func() {
		dialogs.ChooseFolder(u.sh.Window, "", func(p string) { u.importMod(p) })
	})
	importZip := widget.NewButtonWithIcon("Import .zip…", theme.FileIcon(), func() {
		dialogs.ChooseFile(u.sh.Window, "", storage.NewExtensionFileFilter([]string{".zip"}), func(p string) { u.importMod(p) })
	})
	u.sh.Gate(importDir, importZip)
	actions := container.NewVBox(widget.NewSeparator(), container.NewHBox(importDir, importZip))
	return container.NewBorder(head, actions, nil, nil, u.sh.VScroll("mods", body))
}

func (u *ui) modRow(m core.ModInfo) fyne.CanvasObject {
	on := widget.NewCheck(widgets.OrNone(m.Title, m.Name), nil)
	on.Checked = m.Enabled
	on.OnChanged = func(b bool) { u.setModsEnabled([]string{m.Name}, b) }
	if u.sh.Working() {
		on.Disable()
	}
	meta := m.Name
	if m.Version != "" {
		meta += " " + m.Version
	}
	if len(m.Authors) > 0 {
		meta += " · " + strings.Join(m.Authors, ", ")
	}
	var right fyne.CanvasObject = widget.NewLabel("")
	if !m.Builtin {
		remove := widget.NewButtonWithIcon("Remove…", theme.DeleteIcon(), func() { u.removeMod(m.Name) })
		remove.Importance = widget.LowImportance
		u.sh.Gate(remove)
		right = remove
	}
	rows := []fyne.CanvasObject{container.NewBorder(nil, nil, container.NewHBox(on, widgets.Dim(meta)), right)}
	if m.Unsupported != "" {
		on.Disable()
		rows = append(rows, widgets.Note(m.Unsupported, fd.StatusWarn))
	}
	if !m.Builtin && len(m.Types) > 0 {
		rows = append(rows, widgets.DimWrapped("Changes: "+strings.Join(m.Types, ", ")))
	}
	return container.NewVBox(rows...)
}

func (u *ui) importMod(path string) {
	u.perform("Importing "+path+"…", "", func(ctx context.Context) error {
		res, err := core.ImportMod(ctx, core.ImportModRequest{Request: u.request(), Path: path})
		if err != nil {
			return err
		}
		fyne.Do(func() { u.sh.OK("Imported " + res.Mod.Name + ". It starts disabled; tick it to build it.") })
		return nil
	})
}

func (u *ui) removeMod(name string) {
	dialogs.ConfirmDestructive(u.sh.Window, "Remove "+name+"?",
		"The mod's folder is deleted from your library. The game is not touched until the next deploy.",
		"Remove", func() {
			u.perform("Removing "+name+"…", "Removed "+name+".", func(ctx context.Context) error {
				_, err := core.RemoveMod(ctx, core.RemoveModRequest{Request: u.request(), Name: name})
				return err
			})
		})
}
