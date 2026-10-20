package core

import (
	"context"
	"errors"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/emm"
)

// BuildEmmRequest builds the pinned emm from source.
type BuildEmmRequest struct{ Request }

// BuildEmmResult says where it went.
type BuildEmmResult struct {
	Path    string `json:"path"`
	Tag     string `json:"tag"`
	Version string `json:"version"`
}

// BuildEmm checks out the pinned kfc-parser tag and builds emm with cargo
// (R3.3).
func BuildEmm(ctx context.Context, req BuildEmmRequest) (BuildEmmResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return BuildEmmResult{}, err
	}
	if err := config.MkdirAll(s.paths.Tools); err != nil {
		return BuildEmmResult{}, err
	}
	req.Events.logf(LevelInfo, "building emm %s from %s (this takes a few minutes)", emm.Tag, emm.Repo)
	path, err := emm.Build(ctx, emm.BuildOptions{ToolsDir: s.paths.Tools, Log: s.emmLog()})
	if err != nil {
		return BuildEmmResult{}, err
	}
	v, _ := emm.Version(ctx, path)
	return BuildEmmResult{Path: path, Tag: emm.Tag, Version: v}, nil
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
