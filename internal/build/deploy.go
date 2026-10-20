package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
	"github.com/ushineko/enshroudedbonker/internal/steam"
)

// ErrGameRunning reports a deploy or rollback attempted while the game runs.
var ErrGameRunning = errors.New("Enshrouded is running; quit it first") //nolint:staticcheck // a proper noun leads

// Deployed records what the last deploy wrote (R7.2).
type Deployed struct {
	Fingerprint compat.Fingerprint `json:"fingerprint"`
	When        time.Time          `json:"when"`
	Files       []File             `json:"files"`
	// Added are files the deploy created that the vanilla game does not have
	// (emm's empty containers). Rollback removes exactly these.
	Added []string `json:"added"`
	// Archive is where the files it replaced were kept, when they were not
	// already the pristine pair.
	Archive string   `json:"archive,omitempty"`
	Mods    []string `json:"mods"`
}

// KfcSHA256 is the deployed archive index's hash, which is what status
// compares the install against.
func (d *Deployed) KfcSHA256() string {
	for _, f := range d.Files {
		if f.Name == steam.KfcName {
			return f.SHA256
		}
	}
	return ""
}

// LoadDeployed reads the deploy record; nil when there is none.
func LoadDeployed(path string) (*Deployed, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deploy record: %w", err)
	}
	var d Deployed
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse deploy record: %w", err)
	}
	return &d, nil
}

func saveDeployed(path string, d *Deployed) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deploy record: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write deploy record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace deploy record: %w", err)
	}
	return nil
}

// DeployOptions is one deploy.
type DeployOptions struct {
	Install     *steam.Install
	Fingerprint compat.Fingerprint
	Pristine    *Pristine
	Build       *Manifest
	ArchiveRoot string
	RecordPath  string
	// Running is steam.GameRunning, injectable for tests.
	Running func() (bool, bool)
	Log     func(string)
}

func gameRunning(f func() (bool, bool)) error {
	if f == nil {
		f = steam.GameRunning
	}
	if running, _ := f(); running {
		return ErrGameRunning
	}
	return nil
}

/*
Deploy writes a build into the game (R7.2).

Refuses while the game runs and when the build was made for a different
fingerprint. If the install holds something other than the pristine pair --
another tool's patch, say -- that is copied to the archive first. Each file is
written through a temporary name and renamed into place. Containers a previous
deploy added that this build does not have are removed.
*/
func Deploy(o DeployOptions) (*Deployed, error) {
	logf := func(format string, args ...any) {
		if o.Log != nil {
			o.Log(fmt.Sprintf(format, args...))
		}
	}
	if err := gameRunning(o.Running); err != nil {
		return nil, err
	}
	if o.Build.Fingerprint.ID() != o.Fingerprint.ID() {
		return nil, fmt.Errorf("the build was made for game build %s, but %s is installed; build again",
			o.Build.Fingerprint.Short(), o.Fingerprint.Short())
	}
	prev, err := LoadDeployed(o.RecordPath)
	if err != nil {
		return nil, err
	}

	rec := &Deployed{Fingerprint: o.Fingerprint, When: time.Now().UTC()}
	for _, m := range o.Build.Mods {
		rec.Mods = append(rec.Mods, m.Name)
	}

	// Keep what is there unless it is the pristine pair or our own last deploy.
	cur, err := gamefiles.HashFile(o.Install.Kfc())
	if err != nil {
		return nil, err
	}
	ours := prev != nil && cur == prev.KfcSHA256()
	if cur != o.Pristine.KfcSHA256 && !ours {
		dir := filepath.Join(o.ArchiveRoot, time.Now().UTC().Format("20060102T150405Z"))
		logf("the install holds an archive this tool did not write; keeping a copy in %s", dir)
		if err := config.MkdirAll(dir); err != nil {
			return nil, err
		}
		for _, name := range []string{steam.KfcName, steam.ResourcesName} {
			if err := CopyFile(filepath.Join(o.Install.Dir, name), filepath.Join(dir, name)); err != nil {
				return nil, err
			}
		}
		rec.Archive = dir
		pruneArchive(o.ArchiveRoot, 2)
	}

	vanilla := map[string]bool{}
	for _, d := range o.Pristine.Dats {
		vanilla[d.Name] = true
	}
	// Containers first, the index last: the index is what refers to the rest,
	// so a deploy interrupted part-way leaves the old index pointing at files
	// that still hold what it expects.
	files := append([]File(nil), o.Build.Files...)
	rank := func(name string) int {
		switch name {
		case steam.KfcName:
			return 2
		case steam.ResourcesName:
			return 1
		}
		return 0
	}
	sort.SliceStable(files, func(i, j int) bool { return rank(files[i].Name) < rank(files[j].Name) })
	want := map[string]bool{}
	for _, f := range files {
		logf("writing %s (%d bytes)", f.Name, f.Size)
		if err := CopyFile(filepath.Join(o.Build.Dir, f.Name), filepath.Join(o.Install.Dir, f.Name)); err != nil {
			return nil, err
		}
		rec.Files = append(rec.Files, f)
		want[f.Name] = true
		if f.Name != steam.KfcName && f.Name != steam.ResourcesName && !vanilla[f.Name] {
			rec.Added = append(rec.Added, f.Name)
		}
	}
	if prev != nil {
		for _, name := range prev.Added {
			if !want[name] {
				logf("removing %s, which the previous deploy added", name)
				_ = os.Remove(filepath.Join(o.Install.Dir, name))
			}
		}
	}
	sort.Strings(rec.Added)
	if err := saveDeployed(o.RecordPath, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// RollbackOptions is one rollback.
type RollbackOptions struct {
	Install    *steam.Install
	Pristine   *Pristine
	RecordPath string
	Running    func() (bool, bool)
	Log        func(string)
}

// RollbackResult says what a rollback did.
type RollbackResult struct {
	Restored []string `json:"restored"`
	Removed  []string `json:"removed"`
}

/*
Rollback puts the pristine pair back and removes what a deploy added (R7.3).

Containers are removed when the deploy record lists them, and also when they
are empty and not among the game's own: those are emm's, whichever run put them
there, and the vanilla index does not refer to them.
*/
func Rollback(o RollbackOptions) (*RollbackResult, error) {
	if err := gameRunning(o.Running); err != nil {
		return nil, err
	}
	res := &RollbackResult{}
	for _, name := range []string{steam.KfcName, steam.ResourcesName} {
		src := filepath.Join(o.Pristine.Dir, name)
		if err := CopyFile(src, filepath.Join(o.Install.Dir, name)); err != nil {
			return res, err
		}
		res.Restored = append(res.Restored, name)
	}
	vanilla := map[string]bool{}
	for _, d := range o.Pristine.Dats {
		vanilla[d.Name] = true
	}
	remove := map[string]bool{}
	if rec, err := LoadDeployed(o.RecordPath); err == nil && rec != nil {
		for _, name := range rec.Added {
			remove[name] = true
		}
	}
	if dats, err := o.Install.DatFiles(); err == nil {
		for _, d := range dats {
			if d.Size == 0 && !vanilla[d.Name] {
				remove[d.Name] = true
			}
		}
	}
	for name := range remove {
		if vanilla[name] {
			continue
		}
		if err := os.Remove(filepath.Join(o.Install.Dir, name)); err == nil {
			res.Removed = append(res.Removed, name)
		}
	}
	sort.Strings(res.Removed)
	_ = os.Remove(filepath.Join(o.Install.Dir, steam.KfcName+".bak"))
	if err := os.Remove(o.RecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return res, fmt.Errorf("remove deploy record: %w", err)
	}
	return res, nil
}

// pruneArchive keeps the newest n archive entries.
func pruneArchive(root string, n int) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	for len(dirs) > n {
		_ = os.RemoveAll(filepath.Join(root, dirs[0]))
		dirs = dirs[1:]
	}
}

// InstallState is what the install's archive is, as far as the tool knows.
const (
	StateVanilla  = "vanilla"
	StateDeployed = "deployed"
	StateOther    = "modified"
	StateUnknown  = "unknown"
)

// State compares the install's archive index with the pristine and the
// deployed ones. The index alone is hashed: it changes on every patch, and it
// is 9 MB rather than 215.
func State(in *steam.Install, p *Pristine, d *Deployed) (string, error) {
	h, err := gamefiles.HashFile(in.Kfc())
	if err != nil {
		return StateUnknown, err
	}
	switch {
	case d != nil && h == d.KfcSHA256():
		return StateDeployed, nil
	case p != nil && h == p.KfcSHA256:
		return StateVanilla, nil
	case p == nil:
		return StateUnknown, nil
	}
	return StateOther, nil
}
