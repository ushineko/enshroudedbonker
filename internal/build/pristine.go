/*
Package build is the pipeline between the settings and the game (spec 002 R4,
R7): the pristine copy of the archive, the shadow game directory emm runs in,
the build output, and deploy and rollback.

Three properties are load-bearing:

  - The game directory is written only by Deploy and Rollback, both atomic per
    file, both refusing while the game runs.
  - Every build starts from the pristine pair, never from the install, so the
    resource file never accumulates appended records across builds.
  - The pristine pair is captured once per fingerprint from an install that is
    provably unpatched, or imported from a backup that carries the same build
    string.
*/
package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
	"github.com/ushineko/enshroudedbonker/internal/steam"
)

// ErrNoPristine reports that no vanilla copy exists for the installed build.
var ErrNoPristine = errors.New("no pristine copy of the game archive for this build")

// ErrLooksPatched reports an install that cannot be captured as pristine.
var ErrLooksPatched = errors.New("the installed archive looks patched")

// Pristine is the record kept beside a vanilla archive pair.
type Pristine struct {
	Fingerprint compat.Fingerprint `json:"fingerprint"`
	Dir         string             `json:"dir"`
	KfcSHA256   string             `json:"kfc_sha256"`
	KfcSize     int64              `json:"kfc_size"`
	ResSHA256   string             `json:"resources_sha256"`
	ResSize     int64              `json:"resources_size"`
	// Dats are the game's own content containers, which the shadow dir links.
	Dats     []steam.DatFile `json:"dats"`
	Captured time.Time       `json:"captured"`
	Source   string          `json:"source"`
}

// Kfc and Resources are the pristine files.
func (p *Pristine) Kfc() string       { return filepath.Join(p.Dir, steam.KfcName) }
func (p *Pristine) Resources() string { return filepath.Join(p.Dir, steam.ResourcesName) }

func pristineDir(root string, fp compat.Fingerprint) string { return filepath.Join(root, fp.ID()) }

// LoadPristine reads the pristine record for a fingerprint.
func LoadPristine(root string, fp compat.Fingerprint) (*Pristine, error) {
	dir := pristineDir(root, fp)
	b, err := os.ReadFile(filepath.Join(dir, "pristine.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w (%s)", ErrNoPristine, fp.Short())
	}
	if err != nil {
		return nil, fmt.Errorf("read pristine record: %w", err)
	}
	var p Pristine
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse pristine record: %w", err)
	}
	p.Dir = dir
	return &p, nil
}

/*
PatchedSigns lists the reasons an install looks patched, empty when it looks
vanilla.

emm registers nine empty content containers past the game's own, so a
zero-byte enshrouded_NNN.dat is the fingerprint of a patched archive, and an
enshrouded.kfc.bak is what emm leaves when it patches in place. Neither is
conclusive proof of the opposite, which is why a clean result is "looks
vanilla", and why the Steam route (verify integrity) is offered alongside.
*/
func PatchedSigns(in *steam.Install) ([]string, error) {
	var signs []string
	dats, err := in.DatFiles()
	if err != nil {
		return nil, err
	}
	for _, d := range dats {
		if d.Size == 0 {
			signs = append(signs, "empty container "+d.Name+" (emm adds these when it patches)")
		}
	}
	if _, err := os.Stat(in.Kfc() + ".bak"); err == nil {
		signs = append(signs, steam.KfcName+".bak exists (emm's in-place backup)")
	}
	return signs, nil
}

// CapturePristine copies the install's archive pair as the pristine copy for
// its fingerprint, refusing an install that looks patched.
func CapturePristine(root string, in *steam.Install, fp compat.Fingerprint, deployedKfc string) (*Pristine, error) {
	signs, err := PatchedSigns(in)
	if err != nil {
		return nil, err
	}
	if deployedKfc != "" {
		if h, err := gamefiles.HashFile(in.Kfc()); err == nil && h == deployedKfc {
			signs = append(signs, "it is the archive the last deploy wrote")
		}
	}
	if len(signs) > 0 {
		return nil, fmt.Errorf("%w: %v. Restore it first (Steam: Verify integrity of game files), "+
			"or import a vanilla backup with `pristine import`", ErrLooksPatched, signs)
	}
	return storePristine(root, in, fp, in.Kfc(), in.Resources(), "captured from the install")
}

// ImportPristine takes the pristine pair from a backup directory holding
// enshrouded.kfc and enshrouded.kfc_resources. The backup's archive must carry
// the installed build string.
func ImportPristine(root string, in *steam.Install, fp compat.Fingerprint, dir string) (*Pristine, error) {
	kfc := filepath.Join(dir, steam.KfcName)
	res := filepath.Join(dir, steam.ResourcesName)
	v, err := gamefiles.ReadVersion(kfc, 1<<20)
	if err != nil {
		return nil, err
	}
	if v.Raw != fp.Kfc.Raw {
		return nil, fmt.Errorf("%s is from build %s, but the installed game is %s",
			kfc, v.Short(), fp.Short())
	}
	if _, err := os.Stat(res); err != nil {
		return nil, fmt.Errorf("%s: %w", res, err)
	}
	return storePristine(root, in, fp, kfc, res, "imported from "+dir)
}

func storePristine(root string, in *steam.Install, fp compat.Fingerprint, kfc, res, source string) (*Pristine, error) {
	dir := pristineDir(root, fp)
	if err := config.MkdirAll(dir); err != nil {
		return nil, err
	}
	p := &Pristine{Fingerprint: fp, Dir: dir, Captured: time.Now().UTC(), Source: source}
	if err := CopyFile(kfc, p.Kfc()); err != nil {
		return nil, err
	}
	if err := CopyFile(res, p.Resources()); err != nil {
		return nil, err
	}
	var err error
	if p.KfcSHA256, err = gamefiles.HashFile(p.Kfc()); err != nil {
		return nil, err
	}
	if p.ResSHA256, err = gamefiles.HashFile(p.Resources()); err != nil {
		return nil, err
	}
	p.KfcSize, p.ResSize = fileSize(p.Kfc()), fileSize(p.Resources())
	dats, err := in.DatFiles()
	if err != nil {
		return nil, err
	}
	for _, d := range dats {
		if d.Size > 0 {
			p.Dats = append(p.Dats, d)
		}
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode pristine record: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pristine.json"), b, 0o600); err != nil {
		return nil, fmt.Errorf("write pristine record: %w", err)
	}
	return p, nil
}

func fileSize(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return fi.Size()
}

// CopyFile copies a file through a temporary name in the destination
// directory and renames it into place, so a reader never sees half a file.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	tmp := filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".enshroudedbonker-tmp")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) //nolint:gosec // game files are world-readable as Steam installs them
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", dst, err)
	}
	return nil
}
