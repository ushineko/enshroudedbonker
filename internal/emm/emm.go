/*
Package emm runs upstream's Enshrouded Mod Manager CLI (kfc-parser's
mod-manager-cli, GPL-3.0) as a separate program (spec 002 R3, spec 003).

It is never linked and its source is never vendored. Upstream publishes emm
for Windows only, so this package downloads upstream's own emm.exe from the
pinned release, checks it against the recorded SHA-256, and on Linux runs it
under Wine with an explicit argv. Output is parsed only for the lines upstream
documents or that are plainly stable (a mod's own print, the "Applied N
patches" summary, errors); anything unrecognised is kept and shown, never
fatal.
*/
package emm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	"github.com/ushineko/enshroudedbonker/internal/wine"
)

// The pin (spec 003 R1.1). One place: bumping it means a new dry run against
// every ledger build, which is what the ledger's emm_tag column records, and a
// new ExeSHA256 from the release's asset list.
const (
	Tag  = "v0.2.0"
	Repo = "https://github.com/Brabb3l/kfc-parser"
	// ExeName is the release asset, and its name in the tools directory.
	ExeName = "emm.exe"
	// ExeSHA256 is the digest GitHub publishes for the Tag release's emm.exe.
	ExeSHA256 = "b1133f6b8ea7000596cc7710901bd8307b48165b0e650c0487a611d90ea49305"
)

// ReleaseURL is where the pinned emm.exe is downloaded from.
const ReleaseURL = Repo + "/releases/download/" + Tag + "/" + ExeName

// LicenseURL is upstream's licence at the pinned tag, kept beside the binary.
const LicenseURL = "https://raw.githubusercontent.com/Brabb3l/kfc-parser/" + Tag + "/LICENSE"

// Download ceilings: the release asset is about 21 MB, the licence 35 KB.
const (
	maxExeBytes     = 128 << 20
	maxLicenseBytes = 1 << 20
)

// ErrNotInstalled reports that no emm could be found.
var ErrNotInstalled = errors.New("emm is not installed")

// InstallDir is where the pinned download lives under the tools directory.
func InstallDir(toolsDir string) string { return filepath.Join(toolsDir, "emm-"+Tag) }

// PrefixDir is the Wine prefix emm runs in, the tool's own (spec 003 R3.1).
func PrefixDir(toolsDir string) string { return filepath.Join(toolsDir, "wineprefix") }

// Runner is how to start emm: a native binary, or an emm.exe under Wine.
type Runner struct {
	// Path is the emm binary or emm.exe.
	Path string `json:"path"`
	// Wine is set when Path runs under Wine.
	Wine *wine.Wine `json:"wine,omitempty"`
	// Prefix is the Wine prefix, with Wine.
	Prefix string `json:"prefix,omitempty"`
}

// Command is emm with args, as an explicit argv.
func (r Runner) Command(ctx context.Context, args ...string) *exec.Cmd {
	if r.Wine != nil {
		return r.Wine.Command(ctx, r.Prefix, r.Path, args...)
	}
	return exec.CommandContext(ctx, r.Path, args...) //nolint:gosec // explicit argv, path from settings or our tools dir
}

// HostPath is a path as emm sees it: Wine's Z: drive under Wine.
func (r Runner) HostPath(p string) string {
	if r.Wine != nil {
		return wine.WinPath(p)
	}
	return p
}

// stop ends what a cancelled run left behind in the Wine prefix.
func (r Runner) stop() {
	if r.Wine != nil {
		_ = r.Wine.Kill(r.Prefix)
	}
}

// needsWine reports whether a binary is a Windows program on a host that is
// not Windows.
func needsWine(path string) bool {
	return runtime.GOOS != "windows" && strings.EqualFold(filepath.Ext(path), ".exe")
}

/*
Locate finds the emm to use (spec 003 R4.1): the emm_path setting when set,
else the downloaded emm.exe. A .exe runs under the Wine findWine picks, which
is only asked when one is needed.
*/
func Locate(toolsDir, override string, findWine func() (wine.Wine, error)) (r Runner, source string, err error) {
	switch {
	case override != "":
		if fi, err := os.Stat(override); err != nil || fi.IsDir() {
			return Runner{}, "", fmt.Errorf("emm_path %s is not a file", override)
		}
		r, source = Runner{Path: override}, "emm_path setting"
	default:
		p := filepath.Join(InstallDir(toolsDir), ExeName)
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			return Runner{}, "", fmt.Errorf("%w: run `enshroudedbonker tools get-emm`, "+
				"or point the emm_path setting at an emm binary", ErrNotInstalled)
		}
		r, source = Runner{Path: p}, "upstream release "+Tag
	}
	if needsWine(r.Path) {
		w, err := findWine()
		if err != nil {
			return Runner{}, "", fmt.Errorf("emm is a Windows program and needs Wine: %w", err)
		}
		r.Wine, r.Prefix = &w, PrefixDir(toolsDir)
	}
	return r, source, nil
}

// Version asks emm for its version line.
func Version(ctx context.Context, r Runner) (string, error) {
	cmd := r.Command(ctx, "--version")
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", r.Path, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Verify checks a file against the pinned release digest.
func Verify(path string) error {
	got, err := hashFile(path)
	if err != nil {
		return err
	}
	if got != ExeSHA256 {
		return fmt.Errorf("%s has SHA-256 %s, but the %s release's %s is %s", path, got, Tag, ExeName, ExeSHA256)
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GetOptions configures downloading the pinned emm.exe.
type GetOptions struct {
	ToolsDir string
	// URL and LicenseURL default to ReleaseURL and LicenseURL.
	URL, LicenseURL string
	// SHA256 defaults to ExeSHA256.
	SHA256 string
	// Client defaults to http.DefaultClient.
	Client *http.Client
	// Progress receives bytes received and the total (-1 when unknown).
	Progress func(done, total int64)
}

/*
Get downloads the pinned emm.exe and upstream's licence (spec 003 R1).

The binary is written under a temporary name and moved into place only when
its SHA-256 matches the pin; a mismatch deletes it. The licence comes first, so
an emm.exe in the tools directory always has its licence beside it.
*/
func Get(ctx context.Context, o GetOptions) (string, error) {
	if o.URL == "" {
		o.URL = ReleaseURL
	}
	if o.LicenseURL == "" {
		o.LicenseURL = LicenseURL
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	if o.SHA256 == "" {
		o.SHA256 = ExeSHA256
	}
	dst := InstallDir(o.ToolsDir)
	if err := config.MkdirAll(dst); err != nil {
		return "", err
	}
	if _, err := download(ctx, o.Client, o.LicenseURL, filepath.Join(dst, "LICENSE"), maxLicenseBytes, nil); err != nil {
		return "", err
	}
	out := filepath.Join(dst, ExeName)
	tmp := out + ".download"
	got, err := download(ctx, o.Client, o.URL, tmp, maxExeBytes, o.Progress)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if got != o.SHA256 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("the download from %s has SHA-256 %s, but the %s release's %s is %s; it was deleted",
			o.URL, got, Tag, ExeName, o.SHA256)
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("replace %s: %w", out, err)
	}
	return out, nil
}

// download writes url to path through path's own temporary name and returns
// the SHA-256 of what it wrote.
func download(ctx context.Context, c *http.Client, url, path string, limit int64,
	progress func(done, total int64),
) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", url, err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	if resp.ContentLength > limit {
		return "", fmt.Errorf("download %s: %d bytes is more than the %d expected", url, resp.ContentLength, limit)
	}
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", tmp, err)
	}
	h := sha256.New()
	w := io.MultiWriter(f, h, &counter{total: resp.ContentLength, progress: progress})
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("more than %d bytes", limit)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("replace %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// counter reports download progress as bytes pass through it.
type counter struct {
	done, total int64
	progress    func(done, total int64)
}

func (c *counter) Write(p []byte) (int, error) {
	c.done += int64(len(p))
	if c.progress != nil {
		c.progress(c.done, c.total)
	}
	return len(p), nil
}

// run executes one command, streaming its combined output line by line.
func run(cmd *exec.Cmd, log func(string)) error {
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
		return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}

// RunOptions is one `emm run`.
type RunOptions struct {
	Runner  Runner
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
	args := []string{"run", "-g", o.Runner.HostPath(o.GameDir)}
	if o.Patch {
		args = append(args, "--patch")
	}
	if o.Export {
		args = append(args, "--export", "--export-directory", o.Runner.HostPath(o.ExportDir))
	}
	if o.Force {
		args = append(args, "--force")
	}
	var lines []string
	var mu sync.Mutex
	err := run(o.Runner.Command(ctx, args...), func(l string) {
		mu.Lock()
		lines = append(lines, l)
		mu.Unlock()
		if o.Log != nil {
			o.Log(l)
		}
	})
	res := Parse(lines)
	if err != nil {
		if ctx.Err() != nil {
			o.Runner.stop()
			return res, fmt.Errorf("emm run: %w", ctx.Err())
		}
		msg := err.Error()
		if len(res.Errors) > 0 {
			msg = res.Errors[0]
		}
		return res, fmt.Errorf("emm failed: %s", msg)
	}
	return res, nil
}
