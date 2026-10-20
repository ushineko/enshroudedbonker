/*
Package emm runs upstream's Enshrouded Mod Manager CLI (kfc-parser's
mod-manager-cli, GPL-3.0) as a separate program (spec 002 R3).

It is never linked and its source is never vendored: this package builds it
from the pinned upstream tag with the user's own cargo, finds the binary, and
runs it with an explicit argv. Output is parsed only for the lines upstream
documents or that are plainly stable (a mod's own print, the "Applied N
patches" summary, errors); anything unrecognised is kept and shown, never
fatal.
*/
package emm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/enshroudedbonker/internal/config"
)

// The pin (R3.1). One place: bumping it means a new dry run against every
// ledger build, which is what the ledger's emm_tag column records.
const (
	Tag    = "v0.2.0"
	Repo   = "https://github.com/Brabb3l/kfc-parser"
	Crate  = "mod-manager-cli"
	Binary = "emm"
)

// ErrNotInstalled reports that no emm binary could be found.
var ErrNotInstalled = errors.New("emm is not installed")

func exeName() string {
	if runtime.GOOS == "windows" {
		return Binary + ".exe"
	}
	return Binary
}

// InstallDir is where the pinned build lives under the tools directory.
func InstallDir(toolsDir string) string { return filepath.Join(toolsDir, "emm-"+Tag) }

// SourceDir is where the pinned source is checked out.
func SourceDir(toolsDir string) string { return filepath.Join(toolsDir, "src", "kfc-parser-"+Tag) }

// Locate finds the emm to use: the emm_path setting when set, else the pinned
// build under the tools directory.
func Locate(toolsDir, override string) (path, source string, err error) {
	if override != "" {
		if fi, err := os.Stat(override); err != nil || fi.IsDir() {
			return "", "", fmt.Errorf("emm_path %s is not a file", override)
		}
		return override, "emm_path setting", nil
	}
	p := filepath.Join(InstallDir(toolsDir), exeName())
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p, "pinned build " + Tag, nil
	}
	return "", "", fmt.Errorf("%w: run `enshroudedbonker tools build-emm` (needs git and cargo), "+
		"or point the emm_path setting at a binary", ErrNotInstalled)
}

// Version asks a binary for its version line.
func Version(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, path, "--version").Output() //nolint:gosec // explicit argv, path from settings or our tools dir
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// BuildOptions configures building the pinned emm from source.
type BuildOptions struct {
	ToolsDir string
	// Log receives every line git and cargo print.
	Log func(line string)
}

/*
Build checks out the pinned tag and builds the CLI with cargo (R3.3).

The checkout is reused when it is already there at the right tag, so a second
build after a failed one does not clone again. cargo is called with --locked so
the dependency set is the one upstream committed, not whatever resolves today.
*/
func Build(ctx context.Context, o BuildOptions) (string, error) {
	for _, tool := range []string{"git", "cargo"} {
		if _, err := exec.LookPath(tool); err != nil {
			return "", fmt.Errorf("building emm needs %s on PATH: %w", tool, err)
		}
	}
	src := SourceDir(o.ToolsDir)
	if _, err := os.Stat(filepath.Join(src, "Cargo.toml")); err != nil {
		if err := config.MkdirAll(filepath.Dir(src)); err != nil {
			return "", err
		}
		_ = os.RemoveAll(src)
		if err := run(ctx, "", o.Log, "git", "clone", "--depth", "1", "--branch", Tag, Repo, src); err != nil {
			return "", err
		}
	}
	if err := run(ctx, src, o.Log, "cargo", "build", "--release", "--locked", "-p", Crate); err != nil {
		return "", err
	}
	built := filepath.Join(src, "target", "release", exeName())
	dst := InstallDir(o.ToolsDir)
	if err := config.MkdirAll(dst); err != nil {
		return "", err
	}
	out := filepath.Join(dst, exeName())
	if err := copyFile(built, out, 0o755); err != nil {
		return "", err
	}
	// Upstream's license travels with the binary (GPL-3.0).
	_ = copyFile(filepath.Join(src, "LICENSE"), filepath.Join(dst, "LICENSE"), 0o644)
	return out, nil
}

// run executes one command, streaming its combined output line by line.
func run(ctx context.Context, dir string, log func(string), name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// A child that keeps the pipe open must not hold a cancel up forever.
	cmd.WaitDelay = 5 * time.Second
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			if log != nil {
				log(StripANSI(sc.Text()))
			}
		}
		// A line past the buffer stops the scanner; keep draining so the
		// writer never blocks on a pipe nobody reads.
		_, _ = io.Copy(io.Discard, pr)
	}()
	err := cmd.Run()
	_ = pw.Close()
	wg.Wait()
	if err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("replace %s: %w", dst, err)
	}
	return nil
}

// RunOptions is one `emm run`.
type RunOptions struct {
	Path    string
	GameDir string
	Patch   bool
	Export  bool
	// ExportDir is required with Export; emm falls back to <game>/export
	// otherwise, with a warning, which would write into the game.
	ExportDir string
	Force     bool
	Log       func(line string)
}

// Result is what a run reported.
type Result struct {
	// ModLines holds each mod's own printed lines, by mod id.
	ModLines map[string][]string `json:"mod_lines"`
	// Ran lists the mods emm started, in the order it started them.
	Ran []string `json:"ran"`
	// Applied is the "Applied N patches" figure; -1 when absent.
	Applied int `json:"applied"`
	// Errors are the error lines emm printed.
	Errors []string `json:"errors,omitempty"`
	// Lines is everything, ANSI stripped.
	Lines []string `json:"-"`
}

//nolint:gochecknoglobals // compiled once
var (
	ansiRE    = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	modLineRE = regexp.MustCompile(`lua\{mod_id="([^"]+)"\}: eml::lua: (.*)$`)
	runningRE = regexp.MustCompile(`Running mod mod_id="([^"]+)"`)
	appliedRE = regexp.MustCompile(`Applied ([0-9]+) patches`)
	errorRE   = regexp.MustCompile(`^(Error: .*|thread '.*' .*panicked at .*|.* ERROR .*)$`)
)

// StripANSI removes colour codes from a log line.
func StripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// Parse reads a run's output.
func Parse(lines []string) Result {
	r := Result{ModLines: map[string][]string{}, Applied: -1, Lines: lines}
	for _, l := range lines {
		switch {
		case modLineRE.MatchString(l):
			m := modLineRE.FindStringSubmatch(l)
			r.ModLines[m[1]] = append(r.ModLines[m[1]], m[2])
		case runningRE.MatchString(l):
			r.Ran = append(r.Ran, runningRE.FindStringSubmatch(l)[1])
		case appliedRE.MatchString(l):
			n, _ := strconv.Atoi(appliedRE.FindStringSubmatch(l)[1])
			r.Applied = n
		case errorRE.MatchString(strings.TrimSpace(l)):
			r.Errors = append(r.Errors, strings.TrimSpace(l))
		}
	}
	return r
}

// Run executes `emm run` and parses what it printed.
func Run(ctx context.Context, o RunOptions) (Result, error) {
	args := []string{"run", "-g", o.GameDir}
	if o.Patch {
		args = append(args, "--patch")
	}
	if o.Export {
		args = append(args, "--export", "--export-directory", o.ExportDir)
	}
	if o.Force {
		args = append(args, "--force")
	}
	var lines []string
	var mu sync.Mutex
	err := run(ctx, "", func(l string) {
		mu.Lock()
		lines = append(lines, l)
		mu.Unlock()
		if o.Log != nil {
			o.Log(l)
		}
	}, o.Path, args...)
	res := Parse(lines)
	if err != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		msg := err.Error()
		if len(res.Errors) > 0 {
			msg = res.Errors[0]
		}
		return res, fmt.Errorf("emm failed: %s", msg)
	}
	return res, nil
}
