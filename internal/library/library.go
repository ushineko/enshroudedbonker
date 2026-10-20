/*
Package library manages third-party EML mods in the user's library directory
(spec 002 R6).

A library mod is an EML mod directory exactly as its author ships it: mod.json
and src/. Import copies it in after checking the manifest; nothing in it is
rewritten, and nothing from it is ever committed to this repository.
*/
package library

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Manifest is the part of mod.json the tool reads.
type Manifest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Description  string   `json:"description,omitempty"`
	Authors      []string `json:"authors,omitempty"`
	License      string   `json:"license,omitempty"`
}

// Mod is one library mod.
type Mod struct {
	Manifest
	Dir string `json:"dir"`
	// Unsupported says why the mod cannot be built here, when it cannot.
	Unsupported string `json:"unsupported,omitempty"`
	// Types lists the resource types its scripts ask for by name, which is
	// what the overlap report compares.
	Types []string `json:"types,omitempty"`
}

//nolint:gochecknoglobals // compiled once
var (
	idRE    = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	typesRE = regexp.MustCompile(`get_resources?_by_type\(\s*"([^"]+)"`)
	getRE   = regexp.MustCompile(`get_resource\(\s*"[^"]*"\s*,\s*"([^"]+)"`)
)

// ErrNotAMod reports a directory without a usable mod.json.
var ErrNotAMod = errors.New("not an EML mod")

// ReadManifest reads and checks a mod directory's manifest.
func ReadManifest(dir string) (Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "mod.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %s has no mod.json", ErrNotAMod, dir)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %s/mod.json does not parse: %v", ErrNotAMod, dir, err)
	}
	switch {
	case !idRE.MatchString(m.ID):
		return m, fmt.Errorf("%w: id %q must be lowercase letters, digits, - and _, starting with a letter",
			ErrNotAMod, m.ID)
	case m.Name == "":
		return m, fmt.Errorf("%w: %s has no name", ErrNotAMod, m.ID)
	case m.Version == "":
		return m, fmt.Errorf("%w: %s has no version", ErrNotAMod, m.ID)
	case len(m.Capabilities) == 0:
		return m, fmt.Errorf("%w: %s lists no capabilities", ErrNotAMod, m.ID)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "mod.lua")); err != nil {
		return m, fmt.Errorf("%w: %s has no src/mod.lua", ErrNotAMod, m.ID)
	}
	return m, nil
}

func describe(dir string, m Manifest) Mod {
	mod := Mod{Manifest: m, Dir: dir}
	for _, c := range m.Capabilities {
		switch c {
		case "patch", "export":
		case "runtime":
			mod.Unsupported = "needs EML's runtime capability, which runs inside the game; " +
				"this tool patches offline only"
		default:
			mod.Unsupported = "asks for an unknown capability " + strings.TrimSpace(c)
		}
	}
	mod.Types = ReferencedTypes(dir)
	return mod
}

// ReferencedTypes lists the resource types a mod's scripts name.
func ReferencedTypes(dir string) []string {
	seen := map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(dir, "src"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".lua") {
			return nil //nolint:nilerr // an unreadable file just names no types
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil //nolint:nilerr // as above
		}
		for _, t := range TypesInSource(b) {
			seen[t] = true
		}
		return nil
	})
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// TypesInSource lists the resource types one script names, sorted.
func TypesInSource(b []byte) []string {
	seen := map[string]bool{}
	for _, re := range []*regexp.Regexp{typesRE, getRE} {
		for _, m := range re.FindAllSubmatch(b, -1) {
			seen[string(m[1])] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// List reads every mod in the library. A directory that is not a mod is
// skipped rather than failing the listing; Problems says which and why.
func List(libDir string) (mods []Mod, problems []string, err error) {
	entries, err := os.ReadDir(libDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", libDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(libDir, e.Name())
		m, err := ReadManifest(dir)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if m.ID != e.Name() {
			problems = append(problems, fmt.Sprintf("%s: directory name differs from its id %q", dir, m.ID))
			continue
		}
		mods = append(mods, describe(dir, m))
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })
	return mods, problems, nil
}

// Import copies a mod directory or a .zip holding one into the library.
// reserved lists ids that may not be used (the built-ins).
func Import(libDir, src string, reserved []string, replace bool) (Mod, error) {
	if strings.EqualFold(filepath.Ext(src), ".zip") {
		tmp, err := os.MkdirTemp("", "enshroudedbonker-import-")
		if err != nil {
			return Mod{}, fmt.Errorf("temp dir: %w", err)
		}
		defer os.RemoveAll(tmp)
		if err := unzip(src, tmp); err != nil {
			return Mod{}, err
		}
		root, err := findModRoot(tmp)
		if err != nil {
			return Mod{}, fmt.Errorf("%s: %w", src, err)
		}
		src = root
	}
	m, err := ReadManifest(src)
	if err != nil {
		return Mod{}, err
	}
	if slices.Contains(reserved, m.ID) {
		return Mod{}, fmt.Errorf("%s uses the id %q, which is a built-in tweak; rename it in its mod.json", src, m.ID)
	}
	dst := filepath.Join(libDir, m.ID)
	if _, err := os.Stat(dst); err == nil {
		if !replace {
			return Mod{}, fmt.Errorf("%s is already in the library; pass --replace to overwrite it", m.ID)
		}
		if err := os.RemoveAll(dst); err != nil {
			return Mod{}, fmt.Errorf("remove old %s: %w", m.ID, err)
		}
	}
	if err := CopyTree(src, dst); err != nil {
		return Mod{}, err
	}
	return describe(dst, m), nil
}

// Remove deletes a mod from the library.
func Remove(libDir, id string) error {
	if !idRE.MatchString(id) {
		return fmt.Errorf("%q is not a mod id", id)
	}
	dir := filepath.Join(libDir, id)
	if _, err := os.Stat(filepath.Join(dir, "mod.json")); err != nil {
		return fmt.Errorf("%s is not in the library", id)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", id, err)
	}
	return nil
}

// findModRoot is the directory in an unpacked archive that holds mod.json:
// the root itself, or the one directory inside it.
func findModRoot(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "mod.json")); err == nil {
		return dir, nil
	}
	var found []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "mod.json" {
			found = append(found, filepath.Dir(p))
		}
		return nil
	})
	switch len(found) {
	case 0:
		return "", fmt.Errorf("%w: the archive holds no mod.json", ErrNotAMod)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("the archive holds %d mods; import them one at a time", len(found))
}

// unzip extracts an archive, refusing entries that would escape the target.
func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer r.Close()
	root, _ := filepath.Abs(dst)
	for _, f := range r.File {
		p := filepath.Join(dst, f.Name) //nolint:gosec // checked against root below
		abs, _ := filepath.Abs(p)
		if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return fmt.Errorf("%s: entry %q escapes the archive", src, f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", p, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(p), err)
		}
		if err := extract(f, p); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, p string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer rc.Close()
	out, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", p, err)
	}
	// Mods are scripts and small assets; a 256 MiB ceiling stops a zip bomb
	// from filling the disk without getting in the way of anything real.
	if _, err := io.Copy(out, io.LimitReader(rc, 256<<20)); err != nil {
		_ = out.Close()
		return fmt.Errorf("extract %s: %w", f.Name, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", p, err)
	}
	return nil
}

// CopyTree copies a directory recursively, regular files only.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open %s: %w", p, err)
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("create %s: %w", target, err)
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return fmt.Errorf("copy %s: %w", p, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close %s: %w", target, err)
		}
		return nil
	})
}
