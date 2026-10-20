package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/fynedesygn/shell"
)

//nolint:gochecknoglobals // a fixed table
var settingHelp = map[string]string{
	"game_dir":      "The directory holding enshrouded.exe. Empty: found from Steam's library folders.",
	"library_dir":   "Where imported mods are kept.",
	"tools_dir":     "Where emm is built and kept.",
	"workspace_dir": "Vanilla copies, builds and deploy archives (about 450 MB per game build).",
	"emm_path":      "An emm binary to use instead of the one built into the tools directory.",
}

// buildSettings is every key `config set` accepts, each with its own Save.
func (u *ui) buildSettings() fyne.CanvasObject {
	u.loadSettings()
	head := widgets.Heading("Settings", "Shared with the command line: `enshroudedbonker config` "+
		"reads and writes the same file.")
	if !u.configOK || u.settings.Keys == nil {
		return container.NewBorder(head, nil, nil, nil, widget.NewLabel("Reading the settings…"))
	}
	body := container.NewVBox(widgets.PlainRow("Settings file", u.settings.Path), widget.NewSeparator())
	for _, key := range u.settings.Keys {
		entry := widget.NewEntry()
		entry.SetText(u.settings.Settings[key])
		entry.SetPlaceHolder("default")
		k := key
		save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() { u.setConfig(k, entry.Text) })
		u.sh.Gate(save)
		field := dialogs.WithBrowse(u.sh.Window, entry, key != "emm_path")
		body.Add(widget.NewLabelWithStyle(key, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		body.Add(widgets.DimWrapped(settingHelp[key]))
		body.Add(container.NewBorder(nil, nil, nil, save, field))
	}
	if u.statusOK {
		p := u.status.Paths
		body.Add(widget.NewSeparator())
		body.Add(widget.NewLabelWithStyle("Where things are", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		for _, r := range [][2]string{
			{"Library", p.Library}, {"Tools", p.Tools}, {"Vanilla copies", p.Pristine},
			{"Builds", p.Builds}, {"Deploy archive", p.Archive},
		} {
			body.Add(widgets.PlainRow(r[0], r[1]))
		}
	}
	return container.NewBorder(head, nil, nil, nil, u.sh.VScroll("settings", body))
}

// --- About -------------------------------------------------------------------

const projectURL = "https://github.com/ushineko/enshroudedbonker"

func (u *ui) buildAbout() fyne.CanvasObject {
	return shell.AboutSection(u.about()).Build(u.sh)
}

func (u *ui) about() shell.About {
	return shell.About{
		Icon:    appIcon(),
		Name:    "enshroudedbonker",
		Version: u.version + " (" + u.commit + ")",
		Blurb: "Builds EML Lua mods -- its own tweaks and ones you import -- into Enshrouded's archive " +
			"with upstream's emm, and deploys and rolls back the result. Native on Linux with Proton.",
		URL:     projectURL,
		URLText: "Project documentation",
		Notes: []shell.Note{
			{Title: "Upstream does the patching", Detail: "emm, from kfc-parser, applies the mods. " +
				"enshroudedbonker builds it from one pinned tag and runs it as a separate program."},
			{Title: "Checked against your build", Detail: "A game update changes the archive. Every " +
				"build is checked against a compatibility ledger, and an unknown build is dry-run " +
				"before anything is patched."},
			{Title: "Touch little", Detail: "Builds run in a scratch copy. Only Deploy and Rollback " +
				"write into the game: enshrouded.kfc, enshrouded.kfc_resources and the empty " +
				"containers emm adds, each atomically, never while the game runs."},
			{Title: "Saves remember", Detail: "Stacks and chests bigger than vanilla stay in a save " +
				"after a rollback. Do not join other people's servers with a patched game."},
		},
		Facts: []shell.Fact{
			{Label: "emm", Value: emmFact(u)},
			{Label: "Game build", Value: gameFact(u)},
			{Label: "Settings file", Value: widgets.OrNone(u.status.ConfigPath, "not read yet")},
			{Label: "Licence", Value: "MIT (emm: GPL-3.0, separate program)"},
		},
	}
}

func emmFact(u *ui) string {
	if !u.statusOK || !u.status.Emm.Installed {
		return "not installed"
	}
	return widgets.OrNone(u.status.Emm.Version, "installed")
}

func gameFact(u *ui) string {
	if !u.statusOK || u.status.Fingerprint == nil {
		return "no game found"
	}
	return u.status.Fingerprint.Short()
}
