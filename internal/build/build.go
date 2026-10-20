package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
	"github.com/ushineko/enshroudedbonker/internal/library"
	"github.com/ushineko/enshroudedbonker/internal/steam"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// ModSpec is one mod to stage.
type ModSpec struct {
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
	// Overrides are a built-in's parameter values, as set by the user.
	Overrides map[string]float64 `json:"overrides,omitempty"`
	// Dir is a library mod's directory.
	Dir string `json:"-"`
}

// Options is one build.
type Options struct {
	Install  *steam.Install
	Pristine *Pristine
	Emm      string
	Mods     []ModSpec
	// Shadow is the scratch game directory; it is emptied first.
	Shadow string
	// Out receives the output and manifest. Empty means a dry run: emm's
	// verdict is the product, and nothing is kept.
	Out string
	Log func(line string)
}

// File is one output file.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ModOutcome is what one mod did.
type ModOutcome struct {
	Name    string             `json:"name"`
	Builtin bool               `json:"builtin"`
	Params  map[string]float64 `json:"params,omitempty"`
	Ran     bool               `json:"ran"`
	Lines   []string           `json:"lines,omitempty"`
}

// Manifest describes a build (R7.1, R9).
type Manifest struct {
	Fingerprint compat.Fingerprint `json:"fingerprint"`
	Created     time.Time          `json:"created"`
	Mods        []ModOutcome       `json:"mods"`
	// Order is the order emm happened to run the mods in this time.
	Order    []string `json:"order"`
	Applied  int      `json:"applied"`
	Files    []File   `json:"files"`
	Overlaps []string `json:"overlaps,omitempty"`
	Dir      string   `json:"-"`
}

// ManifestName is the manifest's file name in a build directory.
const ManifestName = "manifest.json"

// datRE matches a content container.
var datRE = regexp.MustCompile(`^enshrouded_\d{3}\.dat$`)

// PrepareShadow builds a scratch game directory: copies of the pristine pair,
// and symlinks to the executable and the game's own content containers.
func PrepareShadow(shadow string, in *steam.Install, p *Pristine) error {
	if err := os.RemoveAll(shadow); err != nil {
		return fmt.Errorf("clear %s: %w", shadow, err)
	}
	if err := config.MkdirAll(filepath.Join(shadow, "mods")); err != nil {
		return err
	}
	if err := CopyFile(p.Kfc(), filepath.Join(shadow, steam.KfcName)); err != nil {
		return err
	}
	if err := CopyFile(p.Resources(), filepath.Join(shadow, steam.ResourcesName)); err != nil {
		return err
	}
	if err := os.Symlink(in.Exe(), filepath.Join(shadow, steam.ExeName)); err != nil {
		return fmt.Errorf("link the executable: %w", err)
	}
	for _, d := range p.Dats {
		if err := os.Symlink(filepath.Join(in.Dir, d.Name), filepath.Join(shadow, d.Name)); err != nil {
			return fmt.Errorf("link %s: %w", d.Name, err)
		}
	}
	return nil
}

// Stage writes every mod into the shadow's mods/ folder.
func Stage(shadow string, mods []ModSpec) error {
	dir := filepath.Join(shadow, "mods")
	for _, m := range mods {
		if m.Builtin {
			if err := tweaks.Stage(dir, m.Name, m.Overrides); err != nil {
				return err
			}
			continue
		}
		if err := library.CopyTree(m.Dir, filepath.Join(dir, m.Name)); err != nil {
			return err
		}
	}
	return nil
}

// Run builds: prepare the shadow, stage, run emm, check and collect.
func Run(ctx context.Context, o Options) (*Manifest, emm.Result, error) {
	man := &Manifest{Fingerprint: o.Pristine.Fingerprint, Created: time.Now().UTC(), Applied: -1}
	logf := func(format string, args ...any) {
		if o.Log != nil {
			o.Log(fmt.Sprintf(format, args...))
		}
	}
	logf("preparing the shadow game directory")
	if err := PrepareShadow(o.Shadow, o.Install, o.Pristine); err != nil {
		return man, emm.Result{}, err
	}
	if err := Stage(o.Shadow, o.Mods); err != nil {
		return man, emm.Result{}, err
	}
	logf("running emm with %d mod(s)", len(o.Mods))
	res, err := emm.Run(ctx, emm.RunOptions{Path: o.Emm, GameDir: o.Shadow, Patch: true, Log: o.Log})
	man.Order, man.Applied = res.Ran, res.Applied
	for _, m := range o.Mods {
		man.Mods = append(man.Mods, ModOutcome{
			Name: m.Name, Builtin: m.Builtin, Params: m.Overrides,
			Ran: contains(res.Ran, m.Name), Lines: res.ModLines[m.Name],
		})
	}
	if err != nil {
		return man, res, err
	}
	if res.Applied < 0 {
		return man, res, errors.New("emm finished without saying how many patches it applied")
	}

	// emm appends to the resource file and never rewrites what was there.
	// A build whose output does not start with the pristine bytes is one this
	// tool does not understand, and is not shipped.
	logf("checking the output against the pristine archive")
	shadowRes := filepath.Join(o.Shadow, steam.ResourcesName)
	prefix, err := gamefiles.HashPrefix(shadowRes, o.Pristine.ResSize)
	if err != nil {
		return man, res, err
	}
	if prefix != o.Pristine.ResSHA256 {
		return man, res, errors.New("emm rewrote the original part of the resource file " +
			"instead of appending to it; the output is not shipped")
	}
	if o.Out == "" {
		return man, res, nil
	}
	files, err := collect(o.Shadow)
	if err != nil {
		return man, res, err
	}
	man.Files = files
	if err := publish(o.Shadow, o.Out, man, res); err != nil {
		return man, res, err
	}
	man.Dir = o.Out
	return man, res, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// collect lists what emm wrote: the archive pair and every content container
// that is a real file rather than one of the shadow's links.
func collect(shadow string) ([]File, error) {
	entries, err := os.ReadDir(shadow)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", shadow, err)
	}
	var files []File
	for _, e := range entries {
		name := e.Name()
		if name != steam.KfcName && name != steam.ResourcesName && !datRE.MatchString(name) {
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		p := filepath.Join(shadow, name)
		sum, err := gamefiles.HashFile(p)
		if err != nil {
			return nil, err
		}
		files = append(files, File{Name: name, Size: fileSize(p), SHA256: sum})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

/*
publish moves the output into the build directory.

The new output is assembled beside the old one and swapped in by rename, so a
failure or a cancel part-way leaves the previous build whole.
*/
func publish(shadow, out string, man *Manifest, res emm.Result) error {
	next := out + ".new"
	_ = os.RemoveAll(next)
	if err := config.MkdirAll(next); err != nil {
		return err
	}
	for _, f := range man.Files {
		if err := os.Rename(filepath.Join(shadow, f.Name), filepath.Join(next, f.Name)); err != nil {
			return fmt.Errorf("move %s: %w", f.Name, err)
		}
	}
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(next, ManifestName), b, 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(next, "emm.log"), []byte(strings.Join(res.Lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("write emm.log: %w", err)
	}
	old := out + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(out); err == nil {
		if err := os.Rename(out, old); err != nil {
			return fmt.Errorf("set aside the previous build: %w", err)
		}
	}
	if err := os.Rename(next, out); err != nil {
		_ = os.Rename(old, out)
		return fmt.Errorf("publish the build: %w", err)
	}
	_ = os.RemoveAll(old)
	return nil
}

// LoadManifest reads a build directory's manifest.
func LoadManifest(dir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("read build manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse build manifest: %w", err)
	}
	m.Dir = dir
	return &m, nil
}
