package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// The files that make a directory an Enshrouded install (R1.2). The archive
// pair is what a build patches; the executable is where the fingerprint and
// the type schema live.
const (
	ExeName       = "enshrouded.exe"
	KfcName       = "enshrouded.kfc"
	ResourcesName = "enshrouded.kfc_resources"
)

// ErrNotFound reports that no candidate held an installed copy of the game.
var ErrNotFound = errors.New("no installation of Enshrouded was found")

// datRE matches the content containers beside the archive index.
var datRE = regexp.MustCompile(`^enshrouded_(\d{3})\.dat$`)

// Candidate is one place that was examined, and what was found there (R1.3).
// Every candidate is retained even after a hit so that `detect` can explain why
// it chose what it chose.
type Candidate struct {
	// Root is the Steam root, or "" for a directory given with --game-dir.
	Root string `json:"root,omitempty"`
	// LibraryDir is the Steam library the manifest came from.
	LibraryDir string `json:"library_dir,omitempty"`
	// GameDir is where the game would be if it were installed here.
	GameDir string `json:"game_dir,omitempty"`
	// Reason is empty when the candidate is valid, otherwise why it was rejected.
	Reason string `json:"reason,omitempty"`
}

// Install describes a located game directory.
type Install struct {
	Dir           string `json:"dir"`
	LibraryDir    string `json:"library_dir,omitempty"`
	BuildID       string `json:"build_id,omitempty"`
	Name          string `json:"name,omitempty"`
	LastUpdated   string `json:"last_updated,omitempty"`
	CompatDataDir string `json:"compat_data_dir,omitempty"`
	// Candidates is every place examined, in the order examined.
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Exe, Kfc and Resources are the three files a build reads.
func (in *Install) Exe() string       { return filepath.Join(in.Dir, ExeName) }
func (in *Install) Kfc() string       { return filepath.Join(in.Dir, KfcName) }
func (in *Install) Resources() string { return filepath.Join(in.Dir, ResourcesName) }

// DatFile is one enshrouded_NNN.dat container.
type DatFile struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
	Size  int64  `json:"size"`
}

/*
DatFiles lists the content containers, sorted by index.

`emm` registers nine empty containers past the game's own when it patches, and
the game refuses to start if they are missing (spec 002, found in the first
in-game test). So the listing carries sizes: a zero-byte container is the
fingerprint of a patched install, and a deploy and a rollback both need to know
which ones are theirs.
*/
func (in *Install) DatFiles() ([]DatFile, error) {
	entries, err := os.ReadDir(in.Dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", in.Dir, err)
	}
	var out []DatFile
	for _, e := range entries {
		m := datRE.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", e.Name(), err)
		}
		var idx int
		_, _ = fmt.Sscanf(m[1], "%d", &idx)
		out = append(out, DatFile{Name: e.Name(), Index: idx, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// Roots lists the Steam roots to examine, in order (R1.1).
func Roots() []string {
	var roots []string
	if v := os.Getenv("STEAM_ROOT"); v != "" {
		roots = append(roots, v)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		roots = append(roots,
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".steam", "root"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
			filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
		)
	}
	return roots
}

// Libraries lists the library directories a Steam root knows about.
//
// The root itself is always first: it is a library whether or not it appears in
// libraryfolders.vdf, and on a machine with a single library that file sometimes
// lists it with a path that resolves to the same place through a symlink.
func Libraries(root string) []string {
	libs := []string{root}
	seen := map[string]bool{canonical(root): true}

	b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return libs
	}
	doc := parseVDF(string(b))
	folders := doc.child("libraryfolders")
	if folders == nil {
		folders = doc
	}
	for _, entry := range folders.children {
		path := entry.node.values["path"]
		if path == "" || seen[canonical(path)] {
			continue
		}
		seen[canonical(path)] = true
		libs = append(libs, path)
	}
	return libs
}

// canonical is the key used to de-duplicate library paths: ~/.steam/root is a
// symlink to ~/.local/share/Steam on most installs and must not be examined
// and reported twice.
func canonical(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// Locate finds the game.
//
// An explicit overrideDir skips discovery but is validated identically: a
// --game-dir pointing at a directory without the archive is a mistake worth
// naming, not a reason to silently fall back to whatever Steam has installed.
func Locate(overrideDir string) (*Install, error) {
	if overrideDir != "" {
		lib := libraryOf(overrideDir)
		in, cand := describe("", lib, overrideDir)
		in.Candidates = []Candidate{cand}
		if cand.Reason != "" {
			return nil, fmt.Errorf("%s: %s", overrideDir, cand.Reason)
		}
		applyManifest(in, lib)
		return in, nil
	}

	var candidates []Candidate
	for _, root := range Roots() {
		if _, err := os.Stat(root); err != nil {
			candidates = append(candidates, Candidate{Root: root, Reason: "no such directory"})
			continue
		}
		for _, lib := range Libraries(root) {
			b, err := os.ReadFile(manifestPath(lib))
			if err != nil {
				candidates = append(candidates, Candidate{Root: root, LibraryDir: lib,
					Reason: "no appmanifest_" + AppID + ".acf"})
				continue
			}
			app := parseVDF(string(b)).child("AppState")
			if app == nil || app.values["installdir"] == "" {
				candidates = append(candidates, Candidate{Root: root, LibraryDir: lib,
					Reason: "appmanifest has no installdir"})
				continue
			}
			gameDir := filepath.Join(lib, "steamapps", "common", app.values["installdir"])
			in, cand := describe(root, lib, gameDir)
			candidates = append(candidates, cand)
			if cand.Reason != "" {
				continue
			}
			applyManifest(in, lib)
			in.Candidates = candidates
			return in, nil
		}
	}
	err := fmt.Errorf("%w: looked in %d place(s); set ENSHROUDEDBONKER_GAME_DIR or pass --game-dir",
		ErrNotFound, len(candidates))
	return &Install{Candidates: candidates}, err
}

func manifestPath(lib string) string {
	return filepath.Join(lib, "steamapps", "appmanifest_"+AppID+".acf")
}

// libraryOf recovers the Steam library holding a game directory, on the
// assumption Steam's own layout holds: <library>/steamapps/common/<installdir>.
func libraryOf(gameDir string) string {
	common := filepath.Dir(filepath.Clean(gameDir))
	if filepath.Base(common) != "common" {
		return ""
	}
	steamapps := filepath.Dir(common)
	if filepath.Base(steamapps) != "steamapps" {
		return ""
	}
	return filepath.Dir(steamapps)
}

// applyManifest fills the fields that only the appmanifest knows.
func applyManifest(in *Install, lib string) {
	if lib == "" {
		return
	}
	b, err := os.ReadFile(manifestPath(lib))
	if err != nil {
		return
	}
	app := parseVDF(string(b)).child("AppState")
	if app == nil {
		return
	}
	in.BuildID = app.values["buildid"]
	in.Name = app.values["name"]
	in.LastUpdated = app.values["LastUpdated"]
}

// describe fills in everything derivable from a game directory and reports
// whether it is actually a game directory.
func describe(root, lib, gameDir string) (*Install, Candidate) {
	cand := Candidate{Root: root, LibraryDir: lib, GameDir: gameDir}
	in := &Install{Dir: gameDir, LibraryDir: lib}
	if lib != "" {
		compat := filepath.Join(lib, "steamapps", "compatdata", AppID)
		if _, err := os.Stat(compat); err == nil {
			in.CompatDataDir = compat
		}
	}
	for _, name := range []string{ExeName, KfcName, ResourcesName} {
		fi, err := os.Stat(filepath.Join(gameDir, name))
		switch {
		case err != nil:
			cand.Reason = name + " is missing"
			return in, cand
		case fi.IsDir():
			cand.Reason = name + " is a directory"
			return in, cand
		}
	}
	return in, cand
}
