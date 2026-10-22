package core

import (
	"context"
	"fmt"
	"sort"

	"github.com/ushineko/enshroudedbonker/internal/library"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// TweakParam is one parameter as a front end shows it.
type TweakParam struct {
	tweaks.Param
	Overridden bool `json:"overridden"`
}

// TweakInfo is one built-in as a front end shows it.
type TweakInfo struct {
	Name     string       `json:"name"`
	Title    string       `json:"title"`
	Group    string       `json:"group"`
	Desc     string       `json:"desc"`
	Enabled  bool         `json:"enabled"`
	Disabled string       `json:"disabled,omitempty"`
	Params   []TweakParam `json:"params"`
}

// ListTweaksRequest asks for the built-ins.
type ListTweaksRequest struct{ Request }

// ListTweaksResult lists them.
type ListTweaksResult struct {
	Tweaks []TweakInfo `json:"tweaks"`
	Groups []string    `json:"groups"`
}

// ListTweaks describes every built-in with its current values.
func ListTweaks(_ context.Context, req ListTweaksRequest) (ListTweaksResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListTweaksResult{}, err
	}
	gateDisabled := map[string]bool{}
	if s.installErr == nil {
		if d, err := s.gate(); err == nil {
			for _, n := range d.Disabled {
				gateDisabled[n] = true
			}
		}
	}
	res := ListTweaksResult{Groups: append([]string(nil), tweaks.Groups...)}
	for _, t := range tweaks.All() {
		e, _ := s.cfg.Mod(t.Name)
		info := TweakInfo{Name: t.Name, Title: t.Header.Name, Group: t.Header.Group,
			Desc: t.Header.Desc, Enabled: e.Enabled}
		if gateDisabled[t.Name] {
			info.Disabled = "did not apply in the dry run against this game build"
		}
		over := s.cfg.ParamsFor(t.Name)
		for _, p := range t.Params {
			tp := TweakParam{Param: p}
			if v, ok := over[p.Name]; ok {
				tp.Current, tp.Overridden = v, true
			}
			info.Params = append(info.Params, tp)
		}
		res.Tweaks = append(res.Tweaks, info)
	}
	return res, nil
}

// SetTweakParamRequest sets one parameter.
type SetTweakParamRequest struct {
	Request
	Name  string
	Param string
	Value float64
}

// SetTweakParamResult says what was stored.
type SetTweakParamResult struct {
	New     float64 `json:"new"`
	Kind    string  `json:"kind"`
	Clamped bool    `json:"clamped"`
}

// SetTweakParam stores an override, clamped to the declared range.
func SetTweakParam(_ context.Context, req SetTweakParamRequest) (SetTweakParamResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return SetTweakParamResult{}, err
	}
	t, ok := tweaks.Get(req.Name)
	if !ok {
		return SetTweakParamResult{}, fmt.Errorf("no built-in tweak named %q", req.Name)
	}
	for _, p := range t.Params {
		if p.Name != req.Param {
			continue
		}
		v, clamped := p.Clamp(req.Value)
		if v == p.Default {
			s.cfg.ClearParam(req.Name, req.Param)
		} else {
			s.cfg.SetParam(req.Name, req.Param, v)
		}
		if err := s.cfg.Save(); err != nil {
			return SetTweakParamResult{}, err
		}
		return SetTweakParamResult{New: v, Kind: p.Kind, Clamped: clamped}, nil
	}
	return SetTweakParamResult{}, fmt.Errorf("%s has no parameter %q", req.Name, req.Param)
}

// ResetTweakRequest resets one parameter, or all of a tweak's when Param is "".
type ResetTweakRequest struct {
	Request
	Name  string
	Param string
}

// ResetTweakResult is empty; success is the result.
type ResetTweakResult struct{}

// ResetTweak drops overrides.
func ResetTweak(_ context.Context, req ResetTweakRequest) (ResetTweakResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ResetTweakResult{}, err
	}
	if !tweaks.Has(req.Name) {
		return ResetTweakResult{}, fmt.Errorf("no built-in tweak named %q", req.Name)
	}
	s.cfg.ClearParam(req.Name, req.Param)
	return ResetTweakResult{}, s.cfg.Save()
}

// ModInfo is one entry in the combined build list.
type ModInfo struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Builtin     bool     `json:"builtin"`
	Enabled     bool     `json:"enabled"`
	Version     string   `json:"version,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	Unsupported string   `json:"unsupported,omitempty"`
	Types       []string `json:"types,omitempty"`
	Dir         string   `json:"dir,omitempty"`
}

// ListModsRequest asks for every mod.
type ListModsRequest struct{ Request }

// ListModsResult lists the built-ins and the library.
type ListModsResult struct {
	Mods     []ModInfo `json:"mods"`
	Problems []string  `json:"problems,omitempty"`
}

// ListMods lists the built-ins and the library mods together.
func ListMods(_ context.Context, req ListModsRequest) (ListModsResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ListModsResult{}, err
	}
	return listMods(s)
}

func listMods(s *session) (ListModsResult, error) {
	var res ListModsResult
	for _, t := range tweaks.All() {
		e, _ := s.cfg.Mod(t.Name)
		src, _ := tweaks.Source(t.Name)
		res.Mods = append(res.Mods, ModInfo{Name: t.Name, Title: t.Header.Name, Builtin: true,
			Enabled: e.Enabled, Types: referencedTypes(src)})
	}
	libMods, problems, err := library.List(s.paths.Library)
	if err != nil {
		return res, err
	}
	res.Problems = problems
	for _, m := range libMods {
		e, _ := s.cfg.Mod(m.ID)
		res.Mods = append(res.Mods, ModInfo{Name: m.ID, Title: m.Name, Enabled: e.Enabled,
			Version: m.Version, Authors: m.Authors, Unsupported: m.Unsupported, Types: m.Types, Dir: m.Dir})
	}
	sort.SliceStable(res.Mods, func(i, j int) bool {
		if res.Mods[i].Builtin != res.Mods[j].Builtin {
			return res.Mods[i].Builtin
		}
		return res.Mods[i].Name < res.Mods[j].Name
	})
	return res, nil
}

// SetModsEnabledRequest switches mods on or off.
type SetModsEnabledRequest struct {
	Request
	Names   []string
	Enabled bool
}

// SetModsEnabledResult is empty.
type SetModsEnabledResult struct{}

// SetModsEnabled switches built-ins and library mods on or off.
func SetModsEnabled(_ context.Context, req SetModsEnabledRequest) (SetModsEnabledResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return SetModsEnabledResult{}, err
	}
	list, err := listMods(s)
	if err != nil {
		return SetModsEnabledResult{}, err
	}
	known := map[string]ModInfo{}
	for _, m := range list.Mods {
		known[m.Name] = m
	}
	for _, n := range req.Names {
		m, ok := known[n]
		if !ok {
			return SetModsEnabledResult{}, fmt.Errorf("no mod named %q (see `mods list`)", n)
		}
		if req.Enabled && m.Unsupported != "" {
			return SetModsEnabledResult{}, fmt.Errorf("%s cannot be built: %s", n, m.Unsupported)
		}
		s.cfg.SetEnabled(n, req.Enabled)
	}
	return SetModsEnabledResult{}, s.cfg.Save()
}

// ImportModRequest imports a mod directory or zip.
type ImportModRequest struct {
	Request
	Path    string
	Replace bool
}

// ImportModResult names what was imported.
type ImportModResult struct {
	Mod ModInfo `json:"mod"`
}

// ImportMod copies a third-party mod into the library. It starts disabled.
func ImportMod(_ context.Context, req ImportModRequest) (ImportModResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ImportModResult{}, err
	}
	m, err := library.Import(s.paths.Library, req.Path, tweaks.Names(), req.Replace)
	if err != nil {
		return ImportModResult{}, err
	}
	if _, ok := s.cfg.Mod(m.ID); !ok {
		s.cfg.SetEnabled(m.ID, false)
		if err := s.cfg.Save(); err != nil {
			return ImportModResult{}, err
		}
	}
	return ImportModResult{Mod: ModInfo{Name: m.ID, Title: m.Name, Version: m.Version, Authors: m.Authors,
		Unsupported: m.Unsupported, Types: m.Types, Dir: m.Dir}}, nil
}

// RemoveModRequest removes a library mod.
type RemoveModRequest struct {
	Request
	Name string
}

// RemoveModResult is empty.
type RemoveModResult struct{}

// RemoveMod deletes a library mod and forgets it.
func RemoveMod(_ context.Context, req RemoveModRequest) (RemoveModResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return RemoveModResult{}, err
	}
	if tweaks.Has(req.Name) {
		return RemoveModResult{}, fmt.Errorf("%s is a built-in tweak; disable it instead", req.Name)
	}
	if err := library.Remove(s.paths.Library, req.Name); err != nil {
		return RemoveModResult{}, err
	}
	s.cfg.RemoveMod(req.Name)
	return RemoveModResult{}, s.cfg.Save()
}
