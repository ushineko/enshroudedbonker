package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/steps"

	"github.com/ushineko/enshroudedbonker/internal/core"
)

// maxLogLines is how much of a build's output is kept. emm prints a line per
// tweak summary plus its own progress, so one build is a few hundred lines.
const maxLogLines = 5000

// The steps, in the order core reports them.
const (
	stepGate = iota
	stepShadow
	stepEmm
	stepCollect
)

func buildSteps() []steps.Step {
	return []steps.Step{
		{Name: "Check", Note: "the game build against the compatibility ledger"},
		{Name: "Prepare", Note: "a shadow game directory from the vanilla copy"},
		{Name: "Patch", Note: "run emm over every enabled mod"},
		{Name: "Collect", Note: "verify and keep the output for Deploy"},
	}
}

// stepFor maps core's progress messages onto steps.
func stepFor(what string) (int, bool) {
	switch what {
	case "checking the game build":
		return stepGate, true
	case "preparing the shadow game directory":
		return stepShadow, true
	case "running emm":
		return stepEmm, true
	case "done":
		return stepCollect, true
	}
	return 0, false
}

// buildRun outlives the Build section: the log is still there when the user
// comes back from another section.
type buildRun struct {
	running   bool
	cancelled bool
	cancel    context.CancelFunc

	steps *steps.List
	pane  *logpane.Pane

	finished  bool
	summary   string
	summarySt fd.Status

	totals *widget.Label
}

func (r *buildRun) init() {
	r.pane = logpane.New(logpane.NewModel(maxLogLines))
	r.steps = steps.NewSteps(buildSteps()...)
}

func (r *buildRun) detach() {
	r.steps.Detach()
	r.pane.Detach()
	r.totals = nil
}

func (r *buildRun) reset() {
	r.running, r.cancelled, r.finished = true, false, false
	r.steps.Reset()
	r.steps.Set(stepGate, steps.Running, buildSteps()[stepGate].Note)
	r.summary, r.summarySt = "", fd.StatusInfo
	r.pane.Model().Reset()
	r.pane.SetFollowing(true)
}

// advance never walks a step backwards.
func (r *buildRun) advance(step int, note string) {
	all := r.steps.Steps()
	if step < 0 || step >= len(all) {
		return
	}
	if st := all[step].State; st != steps.Pending && st != steps.Running {
		return
	}
	if note == "" {
		note = all[step].Note
	}
	r.steps.Advance(step, note)
}

func (r *buildRun) stop(state steps.State, note string) {
	r.steps.Stop(state, note)
	r.running = false
	r.finished = true
}

func logLevel(l core.Level) logpane.Level {
	switch l {
	case core.LevelDebug:
		return logpane.Debug
	case core.LevelWarn:
		return logpane.Warn
	case core.LevelError:
		return logpane.Error
	}
	return logpane.Info
}

func (u *ui) buildEvents() core.Events {
	return core.Events{
		Log: func(level core.Level, msg string) { u.run.pane.Model().Append(logLevel(level), msg) },
		Progress: func(p core.Progress) {
			step, ok := stepFor(p.What)
			if !ok {
				return
			}
			fyne.Do(func() {
				u.run.advance(step, "")
				u.drawTotals()
			})
		},
	}
}

// startBuild runs a build off the UI thread behind a cancellable busy popup.
func (u *ui) startBuild(force bool) {
	if u.sh.Working() {
		u.sh.Flash("Something is already running. Wait for it, or cancel it.", fd.StatusWarn)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.run.reset()
	u.run.cancel = cancel
	u.drawTotals()
	u.sh.RedrawStatus()

	req := core.BuildRequest{Request: core.Request{ConfigPath: u.configPath, Events: u.buildEvents()}, Force: force}
	go func() {
		done := u.sh.BusyCancellable("Building…", u.cancelBuild)
		defer done()
		defer cancel()
		stop := u.run.pane.Pump()
		res, err := core.Build(ctx, req)
		stop()
		fyne.Do(func() { u.finishBuild(res, err) })
	}()
}

func (u *ui) cancelBuild() {
	if !u.run.running || u.run.cancel == nil {
		return
	}
	u.run.cancelled = true
	u.run.cancel()
	u.sh.Flash("Cancelling. emm is being stopped; the last good build is kept.", fd.StatusInfo)
}

// finishBuild records the outcome in one banner. UI thread.
func (u *ui) finishBuild(res core.BuildResult, err error) {
	u.run.cancel = nil
	switch {
	case u.run.cancelled || errors.Is(err, context.Canceled):
		u.run.stop(steps.Cancelled, "cancelled")
		u.run.summary, u.run.summarySt = "The build was cancelled. The previous build, if any, is unchanged.", fd.StatusWarn
	case err != nil:
		u.run.stop(steps.Failed, err.Error())
		u.run.summary, u.run.summarySt = "The build failed: "+err.Error(), fd.StatusBad
	default:
		u.run.steps.Advance(stepCollect, "kept")
		u.run.steps.Finish(stepCollect, "kept")
		u.run.running, u.run.finished = false, true
		u.run.summary, u.run.summarySt = buildSummary(res)
	}
	u.run.running = false
	u.sh.Flash(u.run.summary, u.run.summarySt)
	u.drawTotals()
	u.run.pane.Draw()
	u.sh.Invalidate()
}

func buildSummary(res core.BuildResult) (string, fd.Status) {
	m := res.Manifest
	if m == nil {
		return "The build finished, but wrote no manifest.", fd.StatusWarn
	}
	msg := fmt.Sprintf("Built %d mod(s): %d patches, %d file(s). Deploy to put it in the game.",
		len(m.Mods), m.Applied, len(m.Files))
	notRan := []string{}
	for _, mo := range m.Mods {
		if !mo.Ran {
			notRan = append(notRan, mo.Name)
		}
	}
	switch {
	case len(notRan) > 0:
		return msg + " emm did not run: " + strings.Join(notRan, ", ") + ".", fd.StatusWarn
	case len(m.Overlaps) > 0 || len(res.Warnings) > 0:
		return msg + " See the warnings in the output.", fd.StatusWarn
	}
	return msg, fd.StatusGood
}
