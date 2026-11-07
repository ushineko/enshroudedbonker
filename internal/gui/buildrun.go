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

// The steps, in the order core reports them. A manual build starts at Check and
// leaves Deploy pending.
const (
	stepReady = iota
	stepGate
	stepPatch
	stepDeploy
)

func buildSteps() []steps.Step {
	return []steps.Step{
		{Name: "Get ready", Note: "emm and a vanilla copy"},
		{Name: "Check", Note: "the mods against this build"},
		{Name: "Patch", Note: "a copy, with emm"},
		{Name: "Deploy", Note: "write it into the game"},
	}
}

// stepFor maps core's progress messages onto steps.
func stepFor(what string) (int, bool) {
	switch {
	case what == "getting ready":
		return stepReady, true
	case what == "checking the game build", strings.HasPrefix(what, "dry run: "):
		return stepGate, true
	case what == "preparing the shadow game directory", what == "running emm", what == "done":
		return stepPatch, true
	case what == "writing into the game":
		return stepDeploy, true
	}
	return 0, false
}

// buildRun outlives Overview: the log is still there when the user comes back
// from another section.
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
	r.steps.Set(stepReady, steps.Running, buildSteps()[stepReady].Note)
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

// startBuild runs a manual build, for Advanced.
func (u *ui) startBuild(force bool) {
	u.startRun("Building…", func(ctx context.Context, req core.Request) func() {
		res, err := core.Build(ctx, core.BuildRequest{Request: req, Force: force})
		return func() {
			u.finishRun(err, stepPatch, "kept", "The build was cancelled. The previous build, if any, is unchanged.",
				"The build failed: ", func() (string, fd.Status) { return buildSummary(res) })
		}
	})
}

// startApply is the main button: whatever is missing, then the deploy.
func (u *ui) startApply() {
	u.startRun("Applying your mods…", func(ctx context.Context, req core.Request) func() {
		res, err := core.Apply(ctx, core.ApplyRequest{Request: req})
		return func() {
			u.finishRun(err, stepDeploy, "written", "Applying was cancelled. The game was not changed.",
				"The mods were not applied: ", func() (string, fd.Status) { return applySummary(res) })
		}
	})
}

// startRun runs an operation off the UI thread behind a cancellable busy popup,
// on Overview, where its steps and log are. work returns what to do on the UI
// thread once it ends.
func (u *ui) startRun(what string, work func(ctx context.Context, req core.Request) func()) {
	if u.sh.Working() {
		u.sh.Flash("Something is already running. Wait for it, or cancel it.", fd.StatusWarn)
		return
	}
	if cur := u.sh.Current(); cur == nil || cur.Title() != "Overview" {
		u.sh.Select("Overview")
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.run.reset()
	u.run.cancel = cancel
	u.drawTotals()
	u.sh.RedrawStatus()

	req := core.Request{ConfigPath: u.configPath, Events: u.buildEvents()}
	go func() {
		done := u.sh.BusyCancellable(what, u.cancelRun)
		defer done()
		defer cancel()
		stop := u.run.pane.Pump()
		finish := work(ctx, req)
		stop()
		fyne.Do(finish)
	}()
}

func (u *ui) cancelRun() {
	if !u.run.running || u.run.cancel == nil {
		return
	}
	u.run.cancelled = true
	u.run.cancel()
	u.sh.Flash("Cancelling. Once the game is being written, that finishes first.", fd.StatusInfo)
}

// finishRun records the outcome in one banner. UI thread. An apply that was
// already writing when Cancel was pressed still succeeds, and is reported so.
func (u *ui) finishRun(err error, last int, lastNote, cancelled, failed string, summary func() (string, fd.Status)) {
	u.run.cancel = nil
	switch {
	case err != nil && (u.run.cancelled || errors.Is(err, context.Canceled)):
		u.run.stop(steps.Cancelled, "cancelled")
		u.run.summary, u.run.summarySt = cancelled, fd.StatusWarn
	case err != nil:
		u.run.stop(steps.Failed, err.Error())
		u.run.summary, u.run.summarySt = failed+err.Error(), fd.StatusBad
	default:
		u.run.steps.Advance(last, lastNote)
		u.run.steps.Finish(last, lastNote)
		u.run.running, u.run.finished = false, true
		u.run.summary, u.run.summarySt = summary()
	}
	u.run.running = false
	u.sh.Flash(u.run.summary, u.run.summarySt)
	u.drawTotals()
	u.run.pane.Draw()
	u.sh.Invalidate()
}

func applySummary(res core.ApplyResult) (string, fd.Status) {
	d := res.Deployed
	if d == nil {
		return "Applying finished, but left no deploy record.", fd.StatusWarn
	}
	msg := fmt.Sprintf("%d mod(s) are in the game. Launch it to play.", len(d.Mods))
	switch {
	case len(res.Skipped) > 0:
		return msg + " Left out, because they do not work on this game build: " +
			strings.Join(res.Skipped, ", ") + ".", fd.StatusWarn
	case len(res.Warnings) > 0:
		return msg + " See the warnings in the output.", fd.StatusWarn
	}
	return msg, fd.StatusGood
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
