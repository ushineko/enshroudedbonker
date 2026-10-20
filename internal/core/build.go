package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/library"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

func latestDir(s *session) string { return filepath.Join(s.paths.Builds, "latest") }

func referencedTypes(src []byte) []string { return library.TypesInSource(src) }

// specs turns the enabled list into staged-mod specs.
func (s *session) specs(names []string) ([]build.ModSpec, error) {
	lib, _, err := library.List(s.paths.Library)
	if err != nil {
		return nil, err
	}
	libByID := map[string]library.Mod{}
	for _, m := range lib {
		libByID[m.ID] = m
	}
	var out []build.ModSpec
	for _, n := range names {
		if tweaks.Has(n) {
			out = append(out, build.ModSpec{Name: n, Builtin: true, Overrides: s.cfg.ParamsFor(n)})
			continue
		}
		m, ok := libByID[n]
		if !ok {
			return nil, fmt.Errorf("%s is enabled but is neither a built-in nor in the library", n)
		}
		if m.Unsupported != "" {
			return nil, fmt.Errorf("%s cannot be built: %s", n, m.Unsupported)
		}
		out = append(out, build.ModSpec{Name: n, Dir: m.Dir})
	}
	return out, nil
}

/*
overlaps lists resource types more than one enabled mod names, where at least
one of them is a library mod (spec 002 R6.3).

emm runs mods in an arbitrary order, so two mods writing the same field give a
result that changes from build to build. The built-ins are known to write
disjoint fields of the types they share, so only pairs involving a library mod
are reported. Naming a type is not proof of writing the same field; the report
says "check", not "conflict".
*/
func overlaps(mods []build.ModSpec) []string {
	byType := map[string][]string{}
	builtin := map[string]bool{}
	for _, m := range mods {
		var types []string
		if m.Builtin {
			builtin[m.Name] = true
			src, _ := tweaks.Source(m.Name)
			types = referencedTypes(src)
		} else {
			types = library.ReferencedTypes(m.Dir)
		}
		for _, t := range types {
			byType[t] = append(byType[t], m.Name)
		}
	}
	var out []string
	for t, names := range byType {
		if len(names) < 2 {
			continue
		}
		third := false
		for _, n := range names {
			if !builtin[n] {
				third = true
			}
		}
		if !third {
			continue
		}
		sort.Strings(names)
		out = append(out, fmt.Sprintf("%s is used by %s; emm runs mods in no fixed order, so check "+
			"they change different fields", t, strings.Join(names, ", ")))
	}
	sort.Strings(out)
	return out
}

// BuildRequest builds the enabled mods.
type BuildRequest struct {
	Request
	// Force builds an unchecked install without a dry run first. The gate
	// still refuses unsupported and inconsistent installs.
	Force bool
}

// BuildResult is what a build produced.
type BuildResult struct {
	Manifest *build.Manifest `json:"manifest"`
	Warnings []string        `json:"warnings,omitempty"`
}

// Build stages every enabled mod into a shadow game directory, runs emm, and
// keeps the output for Deploy (R7.1).
func Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return BuildResult{}, err
	}
	ev := req.Events
	if err := s.requireInstall(); err != nil {
		return BuildResult{}, err
	}
	ev.progress(1, 5, "checking the game build")
	d, err := s.gate()
	if err != nil {
		return BuildResult{}, err
	}
	switch {
	case d.Verdict == compat.Unchecked && !req.Force:
		return BuildResult{}, errors.New(d.Reason + " Run the compatibility check first.")
	case d.Verdict == compat.Unsupported || d.Verdict == compat.Inconsistent:
		return BuildResult{}, errors.New("refusing to build: " + d.Reason)
	}
	p, err := s.pristine()
	if err != nil {
		return BuildResult{}, fmt.Errorf("%w; capture it with `pristine capture` (from a vanilla install) "+
			"or `pristine import <dir>`", err)
	}
	emmPath, _, err := s.emm()
	if err != nil {
		return BuildResult{}, err
	}
	names := s.cfg.EnabledNames()
	if len(names) == 0 {
		return BuildResult{}, errors.New("no mods are enabled")
	}
	for _, n := range d.Disabled {
		for _, e := range names {
			if e == n {
				return BuildResult{}, fmt.Errorf("%s did not apply in the dry run against this game build; "+
					"disable it", n)
			}
		}
	}
	specs, err := s.specs(names)
	if err != nil {
		return BuildResult{}, err
	}
	res := BuildResult{Warnings: overlaps(specs)}
	for _, w := range res.Warnings {
		ev.logf(LevelWarn, "%s", w)
	}
	ev.progress(2, 5, "preparing the shadow game directory")
	ev.logf(LevelInfo, "building %d mod(s): %s", len(names), strings.Join(names, ", "))
	man, out, err := build.Run(ctx, build.Options{
		Install: s.install, Pristine: p, Emm: emmPath, Mods: specs,
		Shadow: s.paths.Shadow, Out: latestDir(s),
		Log: func(line string) {
			if strings.Contains(line, "Running mod") {
				ev.progress(3, 5, "running emm")
			}
			ev.logf(LevelDebug, "%s", line)
		},
	})
	for name, lines := range out.ModLines {
		for _, l := range lines {
			ev.logf(LevelInfo, "%s: %s", name, strings.TrimPrefix(l, name+": "))
		}
	}
	if err != nil {
		return BuildResult{Manifest: man}, err
	}
	ev.progress(5, 5, "done")
	man.Overlaps = res.Warnings
	res.Manifest = man
	ev.logf(LevelInfo, "applied %d patches; %d file(s) ready to deploy", man.Applied, len(man.Files))
	return res, nil
}

// CompatCheckRequest runs the dry run.
type CompatCheckRequest struct{ Request }

// CompatCheckResult is the recorded dry run and the decision it leads to.
type CompatCheckResult struct {
	DryRun   compat.DryRun   `json:"dry_run"`
	Decision compat.Decision `json:"decision"`
}

/*
CompatCheck dry-runs every built-in with its defaults against a scratch copy of
the pristine archive and records the result for this fingerprint (R2.4).

All of them run together first, which is one emm run. If that fails, each runs
alone to find which ones cannot apply, so one broken tweak disables itself
rather than the whole build.
*/
func CompatCheck(ctx context.Context, req CompatCheckRequest) (CompatCheckResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return CompatCheckResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return CompatCheckResult{}, err
	}
	fp, err := s.fingerprint()
	if err != nil {
		return CompatCheckResult{}, err
	}
	if d := compat.Decide(fp, nil); d.Verdict == compat.Inconsistent {
		return CompatCheckResult{Decision: d}, errors.New(d.Reason)
	}
	p, err := s.pristine()
	if err != nil {
		return CompatCheckResult{}, fmt.Errorf("%w; capture or import it first", err)
	}
	emmPath, _, err := s.emm()
	if err != nil {
		return CompatCheckResult{}, err
	}
	version, _ := emm.Version(ctx, emmPath)
	dry := compat.DryRun{Fingerprint: fp, EmmVersion: version, When: time.Now().UTC(), OK: true}

	names := tweaks.Names()
	all := make([]build.ModSpec, 0, len(names))
	for _, n := range names {
		all = append(all, build.ModSpec{Name: n, Builtin: true})
	}
	ev := req.Events
	ev.progress(1, len(names)+1, "dry run: every built-in together")
	_, out, runErr := build.Run(ctx, build.Options{Install: s.install, Pristine: p, Emm: emmPath,
		Mods: all, Shadow: s.paths.Shadow, Log: s.emmLog()})
	if ctx.Err() != nil {
		return CompatCheckResult{}, fmt.Errorf("compat check: %w", ctx.Err())
	}
	if runErr == nil {
		for _, n := range names {
			dry.Tweaks = append(dry.Tweaks, tweakVerdict(n, out))
		}
	} else {
		ev.logf(LevelWarn, "together they failed (%v); trying each alone", runErr)
		for i, n := range names {
			ev.progress(i+2, len(names)+1, "dry run: "+n)
			_, one, err := build.Run(ctx, build.Options{Install: s.install, Pristine: p, Emm: emmPath,
				Mods: []build.ModSpec{{Name: n, Builtin: true}}, Shadow: s.paths.Shadow, Log: s.emmLog()})
			if ctx.Err() != nil {
				return CompatCheckResult{}, fmt.Errorf("compat check: %w", ctx.Err())
			}
			tv := tweakVerdict(n, one)
			if err != nil {
				tv.OK, tv.Problem = false, err.Error()
			}
			dry.Tweaks = append(dry.Tweaks, tv)
		}
	}
	applied := 0
	for _, t := range dry.Tweaks {
		if t.OK {
			applied++
		}
	}
	if applied == 0 {
		// Nothing applied, whether emm failed or merely reported nothing (a
		// changed log format looks like the latter): this build is unsupported.
		msg := "no built-in tweak reported applying"
		if runErr != nil {
			msg = runErr.Error()
		}
		dry.OK, dry.Error = false, msg
	}
	for _, t := range dry.Tweaks {
		lvl := LevelInfo
		if !t.OK {
			lvl = LevelWarn
		}
		ev.logf(lvl, "%s: %s", t.Name, firstNonEmpty(t.Problem, t.Summary))
	}
	if err := compat.SaveDryRun(s.paths.Compat, dry); err != nil {
		return CompatCheckResult{}, err
	}
	return CompatCheckResult{DryRun: dry, Decision: compat.Decide(fp, &dry)}, nil
}

// tweakVerdict reads one built-in's own report from a run. A built-in that
// ran but printed nothing, or reported changing nothing, did not apply.
func tweakVerdict(name string, r emm.Result) compat.TweakResult {
	tv := compat.TweakResult{Name: name}
	lines := append([]string(nil), r.ModLines[name]...)
	if len(lines) == 0 {
		tv.Problem = "ran but reported nothing"
		return tv
	}
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, name+": ")
	}
	tv.Summary = strings.Join(lines, "; ")
	if strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), " 0") ||
		strings.Contains(tv.Summary, " on 0 ") {
		tv.Problem = "found nothing to change: " + tv.Summary
		return tv
	}
	tv.OK = true
	return tv
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// DeployRequest installs the last build.
type DeployRequest struct{ Request }

// DeployResult is the deploy record.
type DeployResult struct {
	Deployed *build.Deployed `json:"deployed"`
}

// Deploy writes the last build into the game (R7.2).
func Deploy(_ context.Context, req DeployRequest) (DeployResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return DeployResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return DeployResult{}, err
	}
	fp, err := s.fingerprint()
	if err != nil {
		return DeployResult{}, err
	}
	p, err := s.pristine()
	if err != nil {
		return DeployResult{}, err
	}
	man, err := build.LoadManifest(latestDir(s))
	if err != nil {
		return DeployResult{}, fmt.Errorf("nothing has been built yet: %w", err)
	}
	rec, err := build.Deploy(build.DeployOptions{Install: s.install, Fingerprint: fp, Pristine: p, Build: man,
		ArchiveRoot: s.paths.Archive, RecordPath: s.paths.Deployed, Running: s.running(), Log: s.infoLog()})
	if err != nil {
		return DeployResult{}, err
	}
	return DeployResult{Deployed: rec}, nil
}

// RollbackRequest restores the vanilla archive.
type RollbackRequest struct{ Request }

// RollbackResult says what was restored and removed.
type RollbackResult struct {
	build.RollbackResult
}

// Rollback puts the pristine pair back (R7.3).
func Rollback(_ context.Context, req RollbackRequest) (RollbackResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return RollbackResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return RollbackResult{}, err
	}
	p, err := s.pristine()
	if err != nil {
		return RollbackResult{}, fmt.Errorf("%w; use Steam's Verify integrity of game files instead", err)
	}
	r, err := build.Rollback(build.RollbackOptions{Install: s.install, Pristine: p,
		RecordPath: s.paths.Deployed, Running: s.running(), Log: s.infoLog()})
	if err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{RollbackResult: *r}, nil
}

// ReportRequest reads the last build's manifest.
type ReportRequest struct{ Request }

// ReportResult is that manifest.
type ReportResult struct {
	Manifest *build.Manifest `json:"manifest"`
	Dir      string          `json:"dir"`
}

// Report returns the last build's manifest.
func Report(_ context.Context, req ReportRequest) (ReportResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ReportResult{}, err
	}
	m, err := build.LoadManifest(latestDir(s))
	if err != nil {
		return ReportResult{}, err
	}
	return ReportResult{Manifest: m, Dir: m.Dir}, nil
}
