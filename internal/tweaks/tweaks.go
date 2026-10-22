/*
Package tweaks holds the built-in EML mods this project ships (spec 002 R5).

Each is an ordinary EML mod directory -- mod.json and src/mod.lua -- written for
this project and licensed with it, embedded in the binary. A built-in works
unchanged when dropped into upstream EML's mods/ folder; the headers are
comments and the parameters are plain `local` assignments.

Nothing here is third-party. Mods from Nexus belong to their authors and stay
in the user's library directory.
*/
package tweaks

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed mods
var mods embed.FS

// Groups are the Tweaks section's pages, in display order. A tweak whose header
// names something else lands on GroupOther, and a test keeps every built-in off
// it.
//
//nolint:gochecknoglobals // a fixed list
var Groups = []string{"Resources", "Loot", "Inventory", "Crafting", "World"}

// GroupOther is where a tweak with an unrecognised group lands.
const GroupOther = "Other"

// Tweak is one built-in.
type Tweak struct {
	// Name is the mod id, which is also its directory and its config key.
	Name   string  `json:"name"`
	Header Header  `json:"header"`
	Params []Param `json:"params"`
}

// Names lists the built-ins, sorted.
func Names() []string {
	entries, err := fs.ReadDir(mods, "mods")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Has reports whether a name is a built-in.
func Has(name string) bool {
	for _, n := range Names() {
		if n == name {
			return true
		}
	}
	return false
}

// Source returns a copy of a built-in's mod.lua.
func Source(name string) ([]byte, bool) {
	if !Has(name) {
		return nil, false
	}
	b, err := mods.ReadFile(path.Join("mods", name, "src", "mod.lua"))
	if err != nil {
		return nil, false
	}
	return append([]byte(nil), b...), true
}

// Manifest returns a built-in's mod.json.
func Manifest(name string) ([]byte, bool) {
	if !Has(name) {
		return nil, false
	}
	b, err := mods.ReadFile(path.Join("mods", name, "mod.json"))
	if err != nil {
		return nil, false
	}
	return append([]byte(nil), b...), true
}

// Get describes one built-in.
func Get(name string) (Tweak, bool) {
	src, ok := Source(name)
	if !ok {
		return Tweak{}, false
	}
	t := Tweak{Name: name, Header: ParseHeader(src), Params: Parameters(src)}
	if t.Header.Name == "" {
		t.Header.Name = name
	}
	t.Header.Group = normaliseGroup(t.Header.Group)
	return t, true
}

// All describes every built-in.
func All() []Tweak {
	var out []Tweak
	for _, n := range Names() {
		if t, ok := Get(n); ok {
			out = append(out, t)
		}
	}
	return out
}

func normaliseGroup(g string) string {
	for _, known := range Groups {
		if strings.EqualFold(known, g) {
			return known
		}
	}
	return GroupOther
}

// Render is a built-in's source with overrides applied. Unknown override names
// are an error rather than ignored: a value the user set and the build quietly
// dropped is worse than a build that stops and says so.
func Render(name string, overrides map[string]float64) ([]byte, error) {
	t, ok := Get(name)
	if !ok {
		return nil, fmt.Errorf("no built-in tweak named %q", name)
	}
	src, _ := Source(name)
	if len(overrides) == 0 {
		return src, nil
	}
	kinds := map[string]string{}
	for _, p := range t.Params {
		kinds[p.Name] = p.Kind
	}
	for k := range overrides {
		if _, ok := kinds[k]; !ok {
			return nil, fmt.Errorf("%s has no parameter %q", name, k)
		}
	}
	return OverrideAll(src, overrides, kinds)
}

// Stage writes a built-in, with overrides applied, as an EML mod directory
// under dir (dir/<name>/mod.json and dir/<name>/src/mod.lua).
func Stage(dir, name string, overrides map[string]float64) error {
	src, err := Render(name, overrides)
	if err != nil {
		return err
	}
	manifest, _ := Manifest(name)
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", root, err)
	}
	if err := os.WriteFile(filepath.Join(root, "mod.json"), manifest, 0o600); err != nil {
		return fmt.Errorf("write %s mod.json: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "mod.lua"), src, 0o600); err != nil {
		return fmt.Errorf("write %s mod.lua: %w", name, err)
	}
	return nil
}
