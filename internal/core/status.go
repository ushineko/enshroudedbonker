package core

import (
	"context"
	"errors"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/buildinfo"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/library"
	"github.com/ushineko/enshroudedbonker/internal/steam"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// InstallInfo is the game side of status.
type InstallInfo struct {
	Found      bool   `json:"found"`
	Error      string `json:"error,omitempty"`
	Source     string `json:"source"`
	Dir        string `json:"dir,omitempty"`
	LibraryDir string `json:"library_dir,omitempty"`
	BuildID    string `json:"build_id,omitempty"`
	Name       string `json:"name,omitempty"`
	ProtonDir  string `json:"proton_dir,omitempty"`
	Running    bool   `json:"running"`
	// RunningKnown is false where the platform cannot tell.
	RunningKnown bool `json:"running_known"`
}

// EmmInfo is the tool side of status.
type EmmInfo struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Source    string `json:"source,omitempty"`
	Version   string `json:"version,omitempty"`
	PinnedTag string `json:"pinned_tag"`
	Error     string `json:"error,omitempty"`
}

// PristineInfo says whether the vanilla pair is held for this build.
type PristineInfo struct {
	Present  bool   `json:"present"`
	Source   string `json:"source,omitempty"`
	Captured string `json:"captured,omitempty"`
	// Patched lists why the install cannot be captured from right now.
	Patched []string `json:"patched,omitempty"`
}

// BuildInfo summarises the last build.
type BuildInfo struct {
	Present bool     `json:"present"`
	Created string   `json:"created,omitempty"`
	Mods    []string `json:"mods,omitempty"`
	Applied int      `json:"applied"`
	// Current is whether it was made for the installed build.
	Current bool `json:"current"`
}

// StatusRequest asks for the whole picture.
type StatusRequest struct{ Request }

// StatusResult is the whole picture.
type StatusResult struct {
	Version     string              `json:"version"`
	Commit      string              `json:"commit"`
	ConfigPath  string              `json:"config_path"`
	Paths       config.Paths        `json:"paths"`
	Install     InstallInfo         `json:"install"`
	Fingerprint *compat.Fingerprint `json:"fingerprint,omitempty"`
	Gate        compat.Decision     `json:"gate"`
	Emm         EmmInfo             `json:"emm"`
	Pristine    PristineInfo        `json:"pristine"`
	// State is vanilla, deployed, modified or unknown.
	State      string          `json:"state"`
	Deployed   *build.Deployed `json:"deployed,omitempty"`
	LastBuild  BuildInfo       `json:"last_build"`
	Tweaks     int             `json:"tweaks"`
	LibraryMod int             `json:"library_mods"`
	Enabled    []string        `json:"enabled"`
}

// Status reports everything a front end shows on its first screen. A missing
// game is a fact in the result, not an error.
func Status(ctx context.Context, req StatusRequest) (StatusResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return StatusResult{}, err
	}
	res := StatusResult{
		Version: buildinfo.Version, Commit: buildinfo.Commit,
		ConfigPath: s.cfg.Path(), Paths: s.paths,
		Tweaks: len(tweaks.Names()), Enabled: s.cfg.EnabledNames(),
		State: build.StateUnknown,
	}
	if mods, _, err := library.List(s.paths.Library); err == nil {
		res.LibraryMod = len(mods)
	}
	res.Emm = emmInfo(ctx, s)

	res.Install.Source = s.gameSource
	if s.installErr != nil {
		res.Install.Error = s.installErr.Error()
		return res, nil //nolint:nilerr // status reports a missing install, it doesn't fail on one
	}
	in := s.install
	res.Install = InstallInfo{Found: true, Source: s.gameSource, Dir: in.Dir, LibraryDir: in.LibraryDir,
		BuildID: in.BuildID, Name: in.Name, ProtonDir: in.CompatDataDir}
	res.Install.Running, res.Install.RunningKnown = s.running()()

	fp, err := s.fingerprint()
	if err != nil {
		res.Gate = compat.Decision{Verdict: compat.Unsupported, Reason: err.Error()}
		return res, nil //nolint:nilerr // an unreadable fingerprint is a verdict, not a failure
	}
	res.Fingerprint = &fp
	if res.Gate, err = s.gate(); err != nil {
		return res, err
	}

	p, perr := s.pristine()
	switch {
	case perr == nil:
		res.Pristine = PristineInfo{Present: true, Source: p.Source, Captured: p.Captured.Format("2006-01-02 15:04")}
	case errors.Is(perr, build.ErrNoPristine):
		res.Pristine.Patched, _ = build.PatchedSigns(in)
	default:
		return res, perr
	}
	if res.Deployed, err = build.LoadDeployed(s.paths.Deployed); err != nil {
		return res, err
	}
	if res.State, err = build.State(in, p, res.Deployed); err != nil {
		return res, err
	}
	if m, err := build.LoadManifest(latestDir(s)); err == nil {
		res.LastBuild = BuildInfo{Present: true, Created: m.Created.Format("2006-01-02 15:04"),
			Applied: m.Applied, Current: m.Fingerprint.ID() == fp.ID()}
		for _, mo := range m.Mods {
			res.LastBuild.Mods = append(res.LastBuild.Mods, mo.Name)
		}
	}
	return res, nil
}

func emmInfo(ctx context.Context, s *session) EmmInfo {
	info := EmmInfo{PinnedTag: emm.Tag}
	path, source, err := s.emm()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Installed, info.Path, info.Source = true, path, source
	if v, err := emm.Version(ctx, path); err == nil {
		info.Version = v
	} else {
		info.Error = err.Error()
	}
	return info
}

// DetectRequest asks where the game was looked for.
type DetectRequest struct{ Request }

// DetectResult lists every place examined.
type DetectResult struct {
	Found      bool              `json:"found"`
	Dir        string            `json:"dir,omitempty"`
	Error      string            `json:"error,omitempty"`
	Candidates []steam.Candidate `json:"candidates"`
}

// Detect reports where the game was looked for and why each place was
// rejected.
func Detect(_ context.Context, req DetectRequest) (DetectResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return DetectResult{}, err
	}
	if s.installErr != nil {
		var cands []steam.Candidate
		if s.install != nil {
			cands = s.install.Candidates
		}
		return DetectResult{Error: s.installErr.Error(), Candidates: cands}, nil
	}
	return DetectResult{Found: true, Dir: s.install.Dir, Candidates: s.install.Candidates}, nil
}
