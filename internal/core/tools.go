package core

import (
	"context"
	"errors"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/wine"
)

// GetEmmRequest downloads the pinned emm.exe.
type GetEmmRequest struct{ Request }

// GetEmmResult says where it went and what runs it.
type GetEmmResult struct {
	Path    string `json:"path"`
	Tag     string `json:"tag"`
	Version string `json:"version"`
	// Wine is the Wine it runs under, or nil where it runs natively.
	Wine *wine.Wine `json:"wine,omitempty"`
}

/*
GetEmm downloads upstream's emm.exe for the pinned tag, checks its SHA-256 and
asks it for its version (spec 003 R1). On Linux that first run also creates
emm's Wine prefix, so a later build doesn't pay for it.
*/
func GetEmm(ctx context.Context, req GetEmmRequest) (GetEmmResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return GetEmmResult{}, err
	}
	if err := config.MkdirAll(s.paths.Tools); err != nil {
		return GetEmmResult{}, err
	}
	ev := req.Events
	ev.logf(LevelInfo, "downloading %s %s from %s", emm.ExeName, emm.Tag, emm.ReleaseURL)
	lastMB := int64(-1)
	path, err := emm.Get(ctx, emm.GetOptions{ToolsDir: s.paths.Tools, Progress: func(done, total int64) {
		if mb := done >> 20; mb != lastMB && total > 0 {
			lastMB = mb
			ev.progress(int(mb), int(total>>20), "downloading "+emm.ExeName)
		}
	}})
	if err != nil {
		return GetEmmResult{}, err
	}
	ev.logf(LevelInfo, "SHA-256 matches the %s release", emm.Tag)
	r, _, err := emm.Locate(s.paths.Tools, "", s.wine)
	if err != nil {
		return GetEmmResult{Path: path, Tag: emm.Tag}, err
	}
	if r.Wine != nil {
		ev.logf(LevelInfo, "running it under %s (%s); the first run sets up its Wine prefix", r.Wine.Bin, r.Wine.Source)
	}
	v, err := emm.Version(ctx, r)
	if err != nil {
		return GetEmmResult{Path: path, Tag: emm.Tag, Wine: r.Wine}, err
	}
	return GetEmmResult{Path: path, Tag: emm.Tag, Version: v, Wine: r.Wine}, nil
}

// PristineCaptureRequest captures the installed archive as the vanilla copy.
type PristineCaptureRequest struct{ Request }

// PristineImportRequest imports a vanilla copy from a backup directory.
type PristineImportRequest struct {
	Request
	Dir string
}

// PristineResult describes what is held.
type PristineResult struct {
	Pristine *build.Pristine `json:"pristine"`
}

// PristineCapture copies the installed archive pair, if it looks vanilla.
func PristineCapture(_ context.Context, req PristineCaptureRequest) (PristineResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return PristineResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return PristineResult{}, err
	}
	fp, err := s.fingerprint()
	if err != nil {
		return PristineResult{}, err
	}
	if d := compat.Decide(fp, nil); d.Verdict == compat.Inconsistent {
		return PristineResult{}, errors.New(d.Reason)
	}
	var deployedKfc string
	if d, err := build.LoadDeployed(s.paths.Deployed); err == nil && d != nil {
		deployedKfc = d.KfcSHA256()
	}
	p, err := build.CapturePristine(s.paths.Pristine, s.install, fp, deployedKfc)
	if err != nil {
		return PristineResult{}, err
	}
	return PristineResult{Pristine: p}, nil
}

// PristineImport takes the vanilla pair from a backup directory.
func PristineImport(_ context.Context, req PristineImportRequest) (PristineResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return PristineResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return PristineResult{}, err
	}
	fp, err := s.fingerprint()
	if err != nil {
		return PristineResult{}, err
	}
	p, err := build.ImportPristine(s.paths.Pristine, s.install, fp, config.ExpandPath(req.Dir))
	if err != nil {
		return PristineResult{}, err
	}
	return PristineResult{Pristine: p}, nil
}

// ConfigShowRequest reads the settings.
type ConfigShowRequest struct{ Request }

// ConfigShowResult lists them.
type ConfigShowResult struct {
	Path     string            `json:"path"`
	Settings map[string]string `json:"settings"`
	Keys     []string          `json:"keys"`
}

// ConfigShow reads every setting `config set` accepts.
func ConfigShow(_ context.Context, req ConfigShowRequest) (ConfigShowResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ConfigShowResult{}, err
	}
	res := ConfigShowResult{Path: s.cfg.Path(), Settings: map[string]string{}, Keys: config.Keys()}
	for _, k := range config.Keys() {
		v, _ := s.cfg.Get(k)
		res.Settings[k] = v
	}
	return res, nil
}

// ConfigSetRequest writes one setting.
type ConfigSetRequest struct {
	Request
	Key   string
	Value string
}

// ConfigSetResult is the value stored.
type ConfigSetResult struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ConfigSet writes one setting.
func ConfigSet(_ context.Context, req ConfigSetRequest) (ConfigSetResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ConfigSetResult{}, err
	}
	if err := s.cfg.Set(req.Key, req.Value); err != nil {
		return ConfigSetResult{}, err
	}
	if err := s.cfg.Save(); err != nil {
		return ConfigSetResult{}, err
	}
	v, _ := s.cfg.Get(req.Key)
	return ConfigSetResult{Key: req.Key, Value: v}, nil
}
