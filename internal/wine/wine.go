/*
Package wine runs a Windows program under Wine (spec 003 R2, R3).

Upstream publishes emm for Windows only, so on Linux the tool runs upstream's
own emm.exe under Wine rather than asking players to compile it. The Wine is
the Proton that Steam last ran Enshrouded with, read from the game's Proton
prefix, falling back to the system's wine. Every run uses a prefix of the
tool's own: the game's Proton prefix is never used or written.
*/
package wine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotFound reports that neither a Proton for the game nor a system wine
// was found.
var ErrNotFound = errors.New("no Wine was found")

// Wine is one Wine installation.
type Wine struct {
	// Bin is the wine program.
	Bin string `json:"bin"`
	// Source says why this one was chosen.
	Source string `json:"source"`
}

// fontsSuffix ends the line of a Proton prefix's config_info that names the
// Proton which last ran it: <proton>/files/share/fonts/.
const fontsSuffix = "/files/share/fonts/"

/*
Find picks the Wine (R2.1): the Proton recorded in the game's Proton prefix,
when it is still installed, else wine on PATH.

compatDataDir is the game's compatdata/<appid> directory, or "" when it is not
known. Only its config_info is read.
*/
func Find(compatDataDir string) (Wine, error) {
	var tried []string
	if compatDataDir != "" {
		bin, err := protonWine(compatDataDir)
		if err == nil {
			return Wine{Bin: bin, Source: "the Proton Steam last ran Enshrouded with"}, nil
		}
		tried = append(tried, err.Error())
	} else {
		tried = append(tried, "the game's Proton prefix is not known")
	}
	if bin, err := exec.LookPath("wine"); err == nil {
		return Wine{Bin: bin, Source: "the system's wine"}, nil
	}
	tried = append(tried, "wine is not on PATH")
	return Wine{}, fmt.Errorf("%w (%s): run the game once with Proton in Steam, or install wine",
		ErrNotFound, strings.Join(tried, "; "))
}

// protonWine reads which Proton last ran the prefix and returns its wine.
func protonWine(compatDataDir string) (string, error) {
	info := filepath.Join(compatDataDir, "config_info")
	f, err := os.Open(info)
	if err != nil {
		return "", fmt.Errorf("no Proton has run the game yet (%s is missing)", info)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasSuffix(line, fontsSuffix) {
			continue
		}
		bin := filepath.Join(strings.TrimSuffix(line, fontsSuffix), "files", "bin", "wine")
		if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
			return "", fmt.Errorf("the Proton that last ran the game is no longer installed (%s)", bin)
		}
		return bin, nil
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", info, err)
	}
	return "", fmt.Errorf("%s does not name a Proton", info)
}

// Server is the wineserver that belongs to this Wine: the one beside it,
// else the one on PATH.
func (w Wine) Server() string {
	beside := filepath.Join(filepath.Dir(w.Bin), "wineserver")
	if fi, err := os.Stat(beside); err == nil && !fi.IsDir() {
		return beside
	}
	if p, err := exec.LookPath("wineserver"); err == nil {
		return p
	}
	return beside
}

/*
Env is base with the run's Wine settings (R3.2): the prefix, no debug output,
no wine-mono or Gecko (whose install dialogs block a fresh prefix and which emm
does not need), and no display, so nothing opens a window and the run works
over SSH.
*/
func Env(base []string, prefix string) []string {
	drop := map[string]bool{"DISPLAY": true, "WAYLAND_DISPLAY": true,
		"WINEPREFIX": true, "WINEDEBUG": true, "WINEDLLOVERRIDES": true}
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			out = append(out, kv)
		}
	}
	return append(out, "WINEPREFIX="+prefix, "WINEDEBUG=-all", "WINEDLLOVERRIDES=mscoree,mshtml=")
}

// Command runs exe under this Wine in prefix, with an explicit argv.
func (w Wine) Command(ctx context.Context, prefix, exe string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, w.Bin, append([]string{exe}, args...)...) //nolint:gosec // explicit argv, no shell
	cmd.Env = Env(os.Environ(), prefix)
	return cmd
}

// Kill stops every process in prefix (R3.6), so a cancelled run leaves no
// wineserver or Windows program behind.
func (w Wine) Kill(prefix string) error {
	// Its own context: Kill runs after the run's context was cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.Server(), "-k") //nolint:gosec // explicit argv, no shell
	cmd.Env = Env(os.Environ(), prefix)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wineserver -k: %w", err)
	}
	return nil
}

// WinPath is an absolute host path as Wine's Z: drive names it (R3.3).
func WinPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return "Z:" + strings.ReplaceAll(p, "/", `\`)
}
