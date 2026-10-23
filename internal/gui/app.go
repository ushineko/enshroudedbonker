/*
Package gui is the desktop front end.

The window renders internal/core and does nothing else (project rule: CLI/GUI
parity). The window itself -- navigation, content pane, status bar, busy
indicator, banners -- is fynedesygn's shell; this package supplies the sections
and the build run. Every core call runs off the UI thread and hops back with
fyne.Do, and nothing transient reflows the interface.
*/
package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/core"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// appID names the preference store and, on Wayland, the window's app_id.
const appID = "io.ushineko.enshroudedbonker"

// ui holds the program's state and the shell that draws it. The loaded results
// each carry a companion flag: "loaded and empty" and "not loaded" are
// different states.
type ui struct {
	sh         *shell.Shell
	version    string
	commit     string
	configPath string

	status   core.StatusResult
	statusOK bool
	tweaks   core.ListTweaksResult
	tweaksOK bool
	mods     core.ListModsResult
	modsOK   bool
	report   core.ReportResult
	reportOK bool
	settings core.ConfigShowResult
	configOK bool

	run buildRun
}

var sectionTitles = []string{"Overview", "Tweaks", "Mods", "Build", "Settings", "Appearance", "About"}

type sectionEntry struct {
	icon    func() fyne.Resource
	build   func(*ui) fyne.CanvasObject
	detach  func(*ui)
	section func(u *ui, icon func() fyne.Resource) shell.Section
}

func sectionBuilders() map[string]sectionEntry {
	return map[string]sectionEntry{
		"Overview":   {icon: theme.HomeIcon, build: (*ui).buildOverview},
		"Tweaks":     {icon: theme.SettingsIcon, section: tweaksSection},
		"Mods":       {icon: theme.ListIcon, build: (*ui).buildMods},
		"Build":      {icon: theme.MediaPlayIcon, build: (*ui).buildBuild, detach: func(u *ui) { u.run.detach() }},
		"Settings":   {icon: theme.DocumentCreateIcon, build: (*ui).buildSettings},
		"Appearance": {icon: theme.ColorPaletteIcon, section: appearanceSection},
		"About":      {icon: theme.HelpIcon, section: aboutSection},
	}
}

func appearanceSection(u *ui, _ func() fyne.Resource) shell.Section {
	return shell.AppearanceSection("enshroudedbonker " + u.version + " · emm " + emm.Tag)
}

func aboutSection(u *ui, icon func() fyne.Resource) shell.Section {
	return shell.NewSection("About", icon, func(*shell.Shell) fyne.CanvasObject { return u.buildAbout() })
}

func sections(u *ui) []shell.Section {
	builders := sectionBuilders()
	out := make([]shell.Section, 0, len(sectionTitles))
	for _, title := range sectionTitles {
		b, ok := builders[title]
		if !ok {
			continue
		}
		if b.section != nil {
			out = append(out, b.section(u, b.icon))
			continue
		}
		sec := shell.NewSection(title, b.icon, func(*shell.Shell) fyne.CanvasObject { return b.build(u) })
		if b.detach != nil {
			sec.OnDetach(func() { b.detach(u) })
		}
		out = append(out, sec)
	}
	return out
}

// SectionNames lists the navigation entries, for --section.
func SectionNames() []string { return append([]string(nil), sectionTitles...) }

// SchemeNames lists the colour schemes, for --scheme.
func SchemeNames() []string { return fdtheme.SchemeNames() }

// Options configure a run.
type Options struct {
	Version    string
	Commit     string
	ConfigPath string
	Section    string
	Scheme     string
}

// Run opens the window and blocks until it is closed.
func Run(o Options) {
	u := &ui{version: o.Version, commit: o.Commit, configPath: o.ConfigPath}
	u.run.init()
	shell.Run(u.shellOptions(o))
}

func (u *ui) shellOptions(o Options) shell.Options {
	return shell.Options{
		AppID:     appID,
		Name:      "enshroudedbonker",
		Version:   u.version,
		Icon:      appIcon(),
		Sections:  sections(u),
		Section:   o.Section,
		Scheme:    o.Scheme,
		StatusBar: func(*shell.Shell) []fyne.CanvasObject { return u.statusSegments() },
		OnCreate:  func(s *shell.Shell) { u.sh = s },
		OnStart: func(*shell.Shell) {
			u.loadStatus()
		},
		OnInvalidate:  u.onInvalidate,
		AlsoWorking:   func() bool { return u.run.running },
		NavModes:      []shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden},
		NavPlacements: []shell.NavPlacement{shell.NavLeft, shell.NavTop},
	}
}

func (u *ui) onInvalidate(s *shell.Shell) {
	u.statusOK, u.tweaksOK, u.modsOK, u.reportOK, u.configOK = false, false, false, false, false
	if !s.OnScreen() {
		return
	}
	u.loadStatus()
}

// statusSegments: the game build, the gate, emm and what is deployed, from
// every section.
func (u *ui) statusSegments() []fyne.CanvasObject {
	if !u.statusOK {
		return []fyne.CanvasObject{widgets.StatusText("reading…", fd.StatusInfo)}
	}
	st := u.status
	game := widgets.StatusText("not found", fd.StatusBad)
	if st.Install.Found && st.Fingerprint != nil {
		game = widgets.StatusText(st.Fingerprint.Short(), gateStatus(st.Gate.Verdict))
	}
	emmText := widgets.StatusText("not installed", fd.StatusBad)
	if st.Emm.Installed {
		emmText = widgets.StatusText(widgets.OrNone(st.Emm.Version, "installed"), fd.StatusGood)
	}
	return []fyne.CanvasObject{
		widgets.Dim("game"), game, widgets.Sep(),
		widgets.Dim("emm"), emmText, widgets.Sep(),
		widgets.Dim("install"), widgets.StatusText(st.State, stateStatus(st.State)), widgets.Sep(),
		widgets.Dim("enabled"), widget.NewLabel(enabledText(st)),
	}
}

func enabledText(st core.StatusResult) string {
	return itoa(len(st.Enabled)) + " of " + itoa(st.Tweaks+st.LibraryMod)
}

func gateStatus(v string) fd.Status {
	switch v {
	case compat.Verified:
		return fd.StatusGood
	case compat.Derived, compat.Unchecked:
		return fd.StatusWarn
	}
	return fd.StatusBad
}

func stateStatus(s string) fd.Status {
	switch s {
	case "deployed":
		return fd.StatusGood
	case "vanilla":
		return fd.StatusInfo
	case "modified":
		return fd.StatusWarn
	}
	return fd.StatusInfo
}

/*
Actions is every operation this window can reach, named by the CLI command it
corresponds to; the parity test compares it with the cobra tree. A name here is
a claim that the operation is wired, not that a button exists.
*/
func Actions() []string {
	return []string{
		// Overview
		"status", "detect", "tools get-emm", "compat check", "pristine capture", "pristine import",
		"deploy", "rollback",
		// Tweaks
		"tweaks list", "tweaks set", "tweaks reset",
		// Mods
		"mods list", "mods enable", "mods disable", "mods import", "mods remove",
		// Build
		"build", "report",
		// Settings
		"config show", "config set",
	}
}

// groupsInOrder is the Tweaks section's pages.
func groupsInOrder() []string { return append([]string(nil), tweaks.Groups...) }
