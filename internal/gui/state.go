package gui

import (
	"context"
	"strconv"

	"fyne.io/fyne/v2"
	fd "github.com/ushineko/fynedesygn"

	"github.com/ushineko/enshroudedbonker/internal/core"
)

func itoa(n int) string { return strconv.Itoa(n) }

// request carries the --config override into every core call, so this window
// and the CLI read one settings document.
func (u *ui) request() core.Request { return core.Request{ConfigPath: u.configPath} }

// load runs a loader off the UI thread behind the shell's busy indicator (or
// inline, headless, so tests see finished state).
func (u *ui) load(what string, run func()) {
	u.sh.Load(what, func(context.Context) error { run(); return nil })
}

func (u *ui) loadStatus() {
	if u.statusOK {
		return
	}
	u.statusOK = true
	u.load("Reading the game install…", func() {
		res, err := core.Status(context.Background(), core.StatusRequest{Request: u.request()})
		fyne.Do(func() {
			if err != nil {
				u.sh.Report("Read the install", err)
				return
			}
			u.status = res
			u.sh.Rebuild()
		})
	})
}

func (u *ui) loadTweaks() {
	if u.tweaksOK {
		return
	}
	u.tweaksOK = true
	u.load("Reading the built-in tweaks…", func() {
		res, err := core.ListTweaks(context.Background(), core.ListTweaksRequest{Request: u.request()})
		fyne.Do(func() {
			if err != nil {
				u.sh.Report("Read the built-in tweaks", err)
				return
			}
			u.tweaks = res
			u.sh.Rebuild()
		})
	})
}

func (u *ui) loadMods() {
	if u.modsOK {
		return
	}
	u.modsOK = true
	u.load("Reading the mod library…", func() {
		res, err := core.ListMods(context.Background(), core.ListModsRequest{Request: u.request()})
		fyne.Do(func() {
			if err != nil {
				u.sh.Report("Read the mod library", err)
				return
			}
			u.mods = res
			u.sh.Rebuild()
			for _, p := range res.Problems {
				u.sh.Flash(p, fd.StatusWarn)
			}
		})
	})
}

func (u *ui) loadReport() {
	if u.reportOK {
		return
	}
	u.reportOK = true
	u.load("Reading the last build…", func() {
		res, err := core.Report(context.Background(), core.ReportRequest{Request: u.request()})
		fyne.Do(func() {
			if err != nil {
				// Never having built is the ordinary state of a new install.
				u.report = core.ReportResult{}
			} else {
				u.report = res
			}
			u.sh.Refresh()
		})
	})
}

func (u *ui) loadSettings() {
	if u.configOK {
		return
	}
	u.configOK = true
	u.load("Reading the settings…", func() {
		res, err := core.ConfigShow(context.Background(), core.ConfigShowRequest{Request: u.request()})
		fyne.Do(func() {
			if err != nil {
				u.sh.Report("Read the settings", err)
				return
			}
			u.settings = res
			u.sh.Rebuild()
		})
	})
}

// perform runs an operation behind the busy indicator and invalidates every
// loaded result when it succeeds, so each section reads what changed.
func (u *ui) perform(what, done string, fn func(ctx context.Context) error) {
	u.sh.Perform(what, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		fyne.Do(func() {
			if done != "" {
				u.sh.OK(done)
			}
			u.sh.Invalidate()
		})
		return nil
	})
}

// setModsEnabled switches mods on or off from any section.
func (u *ui) setModsEnabled(names []string, on bool) {
	verb := "Disabling"
	if on {
		verb = "Enabling"
	}
	u.perform(verb+" "+joinNames(names)+"…", "", func(ctx context.Context) error {
		_, err := core.SetModsEnabled(ctx, core.SetModsEnabledRequest{Request: u.request(), Names: names, Enabled: on})
		return err
	})
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
