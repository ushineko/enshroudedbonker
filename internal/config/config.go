package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// FileEnv names the environment variable that overrides the config file path.
const FileEnv = "ENSHROUDEDBONKER_CONFIG"

// GameDirEnv outranks the game_dir setting. It is also what the integration
// tests key off: one variable, one meaning.
const GameDirEnv = "ENSHROUDEDBONKER_GAME_DIR"

// ModEntry is one entry in the build list: a built-in tweak or a library mod,
// by id. There is no order to keep -- emm runs mods in whatever order its hash
// map yields (spec 002 R6.3) -- so the list is just which mods are in it and
// whether each is on.
type ModEntry struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

/*
Config is the settings document.

One JSON file rather than Fyne's preference store so that the CLI and the GUI
read the same document. Unknown keys are kept and written back, so a newer
binary's settings survive an older binary's Save.
*/
type Config struct {
	GameDir      string `json:"game_dir"`
	LibraryDir   string `json:"library_dir"`
	ToolsDir     string `json:"tools_dir"`
	WorkspaceDir string `json:"workspace_dir"`
	// EmmPath points at an emm to use instead of the one tools get-emm
	// downloads: a native binary, or an emm.exe, which runs under Wine.
	// Empty uses the pinned download.
	EmmPath string     `json:"emm_path"`
	Mods    []ModEntry `json:"mods"`
	// Params holds per-mod parameter overrides: {"<mod>": {"<NAME>": value}}.
	// They are applied by rewriting the `local NAME = value` line in a staged
	// copy of the mod; the source is never touched.
	Params map[string]map[string]float64 `json:"params"`

	path  string
	extra map[string]json.RawMessage
}

// FilePath is where the settings live: $ENSHROUDEDBONKER_CONFIG, else
// $XDG_CONFIG_HOME/enshroudedbonker/config.json.
func FilePath() string {
	if p := os.Getenv(FileEnv); p != "" {
		return ExpandPath(p)
	}
	return filepath.Join(configHome(), "enshroudedbonker", "config.json")
}

// Defaults returns the settings a machine with no config file has.
func Defaults() *Config {
	data := filepath.Join(dataHome(), "enshroudedbonker")
	return &Config{
		LibraryDir:   filepath.Join(data, "library"),
		ToolsDir:     filepath.Join(data, "tools"),
		WorkspaceDir: filepath.Join(data, "workspace"),
		Params:       map[string]map[string]float64{},
	}
}

// Load reads the default settings file.
func Load() (*Config, error) { return LoadFrom(FilePath()) }

// LoadFrom reads a settings file. A missing file is the defaults, not an
// error: a fresh machine has none until something is saved.
func LoadFrom(path string) (*Config, error) {
	path = ExpandPath(path)
	c := Defaults()
	c.path = path
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err == nil {
		for _, k := range knownKeys() {
			delete(raw, k)
		}
		if len(raw) > 0 {
			c.extra = raw
		}
	}
	c.fillDefaults()
	return c, nil
}

func knownKeys() []string {
	return []string{"game_dir", "library_dir", "tools_dir", "workspace_dir", "emm_path", "mods", "params"}
}

func (c *Config) fillDefaults() {
	d := Defaults()
	if c.LibraryDir == "" {
		c.LibraryDir = d.LibraryDir
	}
	if c.ToolsDir == "" {
		c.ToolsDir = d.ToolsDir
	}
	if c.WorkspaceDir == "" {
		c.WorkspaceDir = d.WorkspaceDir
	}
	if c.Params == nil {
		c.Params = map[string]map[string]float64{}
	}
	c.GameDir = ExpandPath(c.GameDir)
	c.LibraryDir = ExpandPath(c.LibraryDir)
	c.ToolsDir = ExpandPath(c.ToolsDir)
	c.WorkspaceDir = ExpandPath(c.WorkspaceDir)
	c.EmmPath = ExpandPath(c.EmmPath)
}

// Path is the file this was loaded from and will be saved to.
func (c *Config) Path() string { return c.path }

// Save writes the settings atomically: a crash mid-write must not leave a
// half-written document that the next run cannot parse.
func (c *Config) Save() error {
	if c.path == "" {
		c.path = FilePath()
	}
	if err := MkdirAll(filepath.Dir(c.path)); err != nil {
		return err
	}
	doc := map[string]any{}
	for k, v := range c.extra {
		doc[k] = v
	}
	doc["game_dir"] = c.GameDir
	doc["library_dir"] = c.LibraryDir
	doc["tools_dir"] = c.ToolsDir
	doc["workspace_dir"] = c.WorkspaceDir
	doc["emm_path"] = c.EmmPath
	doc["mods"] = c.Mods
	doc["params"] = c.Params
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("replace %s: %w", c.path, err)
	}
	return nil
}

// Paths are the directories derived from the settings.
type Paths struct {
	Config    string `json:"config"`
	Library   string `json:"library"`
	Tools     string `json:"tools"`
	Workspace string `json:"workspace"`
	// Pristine holds the vanilla archive pair, one directory per fingerprint.
	Pristine string `json:"pristine"`
	// Builds holds the latest build's output and report.
	Builds string `json:"builds"`
	// Shadow is the scratch game dir a build runs emm in.
	Shadow string `json:"shadow"`
	// Archive holds what deploys replaced.
	Archive string `json:"archive"`
	// Deployed is the record of what the last deploy wrote.
	Deployed string `json:"deployed"`
	// Compat holds the dry-run verdict per fingerprint.
	Compat string `json:"compat"`
}

// Paths derives every directory the program writes under.
func (c *Config) Paths() Paths {
	ws := c.WorkspaceDir
	return Paths{
		Config:    c.path,
		Library:   c.LibraryDir,
		Tools:     c.ToolsDir,
		Workspace: ws,
		Pristine:  filepath.Join(ws, "pristine"),
		Builds:    filepath.Join(ws, "builds"),
		Shadow:    filepath.Join(ws, "shadow"),
		Archive:   filepath.Join(ws, "archive"),
		Deployed:  filepath.Join(ws, "deployed.json"),
		Compat:    filepath.Join(ws, "compat"),
	}
}

// ResolvedGameDir is where the game is, and who said so: the environment
// outranks the setting, and empty means "discover it from Steam".
func (c *Config) ResolvedGameDir() (dir, source string) {
	if v := os.Getenv(GameDirEnv); v != "" {
		return ExpandPath(v), "$" + GameDirEnv
	}
	if c.GameDir != "" {
		return c.GameDir, "config game_dir"
	}
	return "", "Steam manifests"
}

// Keys lists the settings `config set` accepts.
func Keys() []string {
	return []string{"game_dir", "library_dir", "tools_dir", "workspace_dir", "emm_path"}
}

func (c *Config) field(key string) (*string, error) {
	switch key {
	case "game_dir":
		return &c.GameDir, nil
	case "library_dir":
		return &c.LibraryDir, nil
	case "tools_dir":
		return &c.ToolsDir, nil
	case "workspace_dir":
		return &c.WorkspaceDir, nil
	case "emm_path":
		return &c.EmmPath, nil
	}
	return nil, fmt.Errorf("unknown setting %q (known: %s)", key, strings.Join(Keys(), ", "))
}

// Get reads one setting.
func (c *Config) Get(key string) (string, error) {
	f, err := c.field(key)
	if err != nil {
		return "", err
	}
	return *f, nil
}

// Set writes one setting. An empty value resets a directory to its default.
func (c *Config) Set(key, value string) error {
	f, err := c.field(key)
	if err != nil {
		return err
	}
	*f = ExpandPath(strings.TrimSpace(value))
	c.fillDefaults()
	return nil
}

// Mod looks up an entry in the build list.
func (c *Config) Mod(name string) (ModEntry, bool) {
	for _, m := range c.Mods {
		if m.Name == name {
			return m, true
		}
	}
	return ModEntry{}, false
}

// SetEnabled switches a mod on or off, adding it to the list if needed.
func (c *Config) SetEnabled(name string, on bool) {
	for i := range c.Mods {
		if c.Mods[i].Name == name {
			c.Mods[i].Enabled = on
			return
		}
	}
	c.Mods = append(c.Mods, ModEntry{Name: name, Enabled: on})
}

// RemoveMod drops a mod from the list and forgets its parameters.
func (c *Config) RemoveMod(name string) {
	c.Mods = slices.DeleteFunc(c.Mods, func(m ModEntry) bool { return m.Name == name })
	delete(c.Params, name)
}

// SetParam records an override; ClearParam drops one (or every one for a mod
// when name is empty).
func (c *Config) SetParam(mod, name string, v float64) {
	if c.Params[mod] == nil {
		c.Params[mod] = map[string]float64{}
	}
	c.Params[mod][name] = v
}

// ClearParam drops an override.
func (c *Config) ClearParam(mod, name string) {
	if name == "" {
		delete(c.Params, mod)
		return
	}
	delete(c.Params[mod], name)
	if len(c.Params[mod]) == 0 {
		delete(c.Params, mod)
	}
}

// ParamsFor is a copy of a mod's overrides.
func (c *Config) ParamsFor(mod string) map[string]float64 {
	return maps.Clone(c.Params[mod])
}

// EnabledNames lists the enabled mods, sorted, which is the order every report
// uses (there is no build order to reflect).
func (c *Config) EnabledNames() []string {
	var out []string
	for _, m := range c.Mods {
		if m.Enabled {
			out = append(out, m.Name)
		}
	}
	sort.Strings(out)
	return out
}
