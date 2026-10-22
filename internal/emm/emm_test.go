package emm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/wine"
)

// A synthetic emm log in the shape tracing prints it, colour codes included.
// The mod names and numbers are made up.
var synthLog = []string{
	"\x1b[2m2026-01-01T00:00:00.000Z\x1b[0m \x1b[32m INFO\x1b[0m Loading game directory",
	"\x1b[2m2026-01-01T00:00:01.000Z\x1b[0m \x1b[32m INFO\x1b[0m Running mod mod_id=\"alpha\"",
	"\x1b[2m2026-01-01T00:00:01.100Z\x1b[0m \x1b[32m INFO\x1b[0m \x1b[1mlua\x1b[0m\x1b[1m{\x1b[0mmod_id=\"alpha\"\x1b[1m}\x1b[0m: eml::lua: alpha: changed 3",
	"\x1b[2m2026-01-01T00:00:01.200Z\x1b[0m \x1b[32m INFO\x1b[0m lua{mod_id=\"alpha\"}: eml::lua: alpha: second line",
	"2026-01-01T00:00:02.000Z  INFO Running mod mod_id=\"beta\"",
	"2026-01-01T00:00:02.100Z  INFO lua{mod_id=\"beta\"}: eml::lua: beta: x2 on 7 items",
	"2026-01-01T00:00:03.000Z \x1b[33m WARN\x1b[0m something odd but not fatal",
	"2026-01-01T00:00:04.000Z \x1b[31mERROR\x1b[0m failed to frobnicate",
	"Applied 12 patches",
	"   Error: final failure   ",
}

func stripped(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = StripANSI(l)
	}
	return out
}

func TestStripANSI(t *testing.T) {
	assert.Equal(t, " INFO plain", StripANSI("\x1b[32m INFO\x1b[0m plain"))
	assert.Equal(t, "a{b}c", StripANSI("\x1b[1ma\x1b[0m\x1b[1;4m{\x1b[0mb\x1b[1m}\x1b[0mc"))
	assert.Equal(t, "no codes", StripANSI("no codes"))
}

func TestParse_SyntheticLog(t *testing.T) {
	r := Parse(stripped(synthLog))
	assert.Equal(t, []string{"alpha", "beta"}, r.Ran, "Ran keeps the order emm started the mods in")
	assert.Equal(t, map[string][]string{
		"alpha": {"alpha: changed 3", "alpha: second line"},
		"beta":  {"beta: x2 on 7 items"},
	}, r.ModLines)
	assert.Equal(t, 12, r.Applied)
	assert.Equal(t, []string{
		"2026-01-01T00:00:04.000Z ERROR failed to frobnicate",
		"Error: final failure",
	}, r.Errors)
	assert.Len(t, r.Lines, len(synthLog))
}

func TestParse_ModLineIsNotOnlyCheckedForColour(t *testing.T) {
	// Without stripping, the colour codes split `lua{` and the mod line is
	// missed: Run must strip before parsing.
	r := Parse([]string{synthLog[2]})
	assert.Empty(t, r.ModLines)
}

func TestParse_NoSummaryIsMinusOne(t *testing.T) {
	r := Parse([]string{"INFO nothing here"})
	assert.Equal(t, -1, r.Applied, "absent must be told apart from 'Applied 0 patches'")
	assert.Empty(t, r.Errors)
	assert.Equal(t, 0, Parse([]string{"Applied 0 patches"}).Applied)
}

func TestParse_PanicIsAnError(t *testing.T) {
	r := Parse([]string{"thread 'main' panicked at src/main.rs:1:1:"})
	assert.Len(t, r.Errors, 1)
}

func TestParse_ModPrintMentioningErrorIsAModLine(t *testing.T) {
	r := Parse([]string{`lua{mod_id="m"}: eml::lua: m: an ERROR count of 0`})
	assert.Equal(t, []string{"m: an ERROR count of 0"}, r.ModLines["m"])
	assert.Empty(t, r.Errors, "a mod's own print must not fail the build")
}

func noWine() (wine.Wine, error) { return wine.Wine{}, errors.New("no wine in this test") }

func TestLocate(t *testing.T) {
	tools := t.TempDir()
	_, _, err := Locate(tools, "", noWine)
	require.ErrorIs(t, err, ErrNotInstalled)
	assert.Contains(t, err.Error(), "tools get-emm")

	_, _, err = Locate(tools, filepath.Join(tools, "missing"), noWine)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotInstalled), "a bad emm_path is named, not replaced by the pinned download")
	_, _, err = Locate(tools, tools, noWine)
	require.Error(t, err, "a directory is not an emm binary")

	override := filepath.Join(tools, "my-emm")
	require.NoError(t, os.WriteFile(override, nil, 0o600))
	r, src, err := Locate(tools, override, noWine)
	require.NoError(t, err, "a native emm_path never asks for Wine")
	assert.Equal(t, Runner{Path: override}, r)
	assert.Equal(t, "emm_path setting", src)
}

func TestLocate_DownloadedExeRunsUnderWine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("emm.exe runs natively on Windows")
	}
	tools := t.TempDir()
	exe := filepath.Join(InstallDir(tools), ExeName)
	require.NoError(t, os.MkdirAll(filepath.Dir(exe), 0o750))
	require.NoError(t, os.WriteFile(exe, nil, 0o600))

	_, _, err := Locate(tools, "", noWine)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs Wine")

	w := wine.Wine{Bin: "/opt/proton/files/bin/wine", Source: "test"}
	r, src, err := Locate(tools, "", func() (wine.Wine, error) { return w, nil })
	require.NoError(t, err)
	assert.Equal(t, Runner{Path: exe, Wine: &w, Prefix: PrefixDir(tools)}, r)
	assert.Contains(t, src, Tag)

	override := filepath.Join(tools, "other", "EMM.EXE")
	require.NoError(t, os.MkdirAll(filepath.Dir(override), 0o750))
	require.NoError(t, os.WriteFile(override, nil, 0o600))
	r, _, err = Locate(tools, override, func() (wine.Wine, error) { return w, nil })
	require.NoError(t, err)
	assert.Equal(t, &w, r.Wine, "an emm_path ending in .exe also runs under Wine")
}

func TestDirsCarryTheTag(t *testing.T) {
	assert.Equal(t, filepath.Join("/t", "emm-"+Tag), InstallDir("/t"))
	assert.Equal(t, filepath.Join("/t", "wineprefix"), PrefixDir("/t"))
}

// fakeEmm writes a shell script standing in for emm. body runs after the
// argv is recorded to argv.txt beside it.
func fakeEmm(t *testing.T, body string) (path, argvFile string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not on PATH")
	}
	dir := t.TempDir()
	path = filepath.Join(dir, "emm")
	argvFile = filepath.Join(dir, "argv.txt")
	script := "#!" + bash + "\n" +
		"if [ \"$1\" = --version ]; then echo 'emm 0.0.0-fake'; exit 0; fi\n" +
		"printf '%s\\n' \"$@\" > '" + argvFile + "'\n" + body + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755)) //nolint:gosec // test executable
	return path, argvFile
}

func readArgv(t *testing.T, p string) []string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestRun_ArgvAndParsedOutput(t *testing.T) {
	path, argv := fakeEmm(t, `
printf '\033[32m INFO\033[0m Running mod mod_id="alpha"\n'
printf '\033[1mlua\033[0m\033[1m{\033[0mmod_id="alpha"\033[1m}\033[0m: eml::lua: alpha: changed 3\n'
echo 'Applied 4 patches' >&2
`)
	var logged []string
	game := filepath.Join(t.TempDir(), "game dir with spaces")
	res, err := Run(context.Background(), RunOptions{Runner: Runner{Path: path}, GameDir: game, Patch: true,
		Log: func(l string) { logged = append(logged, l) }})
	require.NoError(t, err)
	assert.Equal(t, []string{"run", "-g", game, "--patch"}, readArgv(t, argv))
	assert.Equal(t, []string{"alpha"}, res.Ran)
	assert.Equal(t, []string{"alpha: changed 3"}, res.ModLines["alpha"])
	assert.Equal(t, 4, res.Applied, "stderr is read as well as stdout")
	assert.Len(t, logged, 3)
	for _, l := range logged {
		assert.NotContains(t, l, "\x1b", "the log callback receives stripped lines")
	}
}

func TestRun_ExportAndForceFlags(t *testing.T) {
	path, argv := fakeEmm(t, "exit 0")
	_, err := Run(context.Background(), RunOptions{Runner: Runner{Path: path}, GameDir: "/g", Export: true,
		ExportDir: "/out", Force: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"run", "-g", "/g", "--export", "--export-directory", "/out", "--force"},
		readArgv(t, argv), "export always names its directory, or emm writes into the game")
}

func TestRun_FailureReportsFirstErrorLine(t *testing.T) {
	path, _ := fakeEmm(t, "echo 'Running mod mod_id=\"a\"'\necho 'Error: the archive is not what I expected'\necho 'Error: second'\nexit 3")
	res, err := Run(context.Background(), RunOptions{Runner: Runner{Path: path}, GameDir: "/g", Patch: true})
	require.Error(t, err)
	assert.Equal(t, "emm failed: Error: the archive is not what I expected", err.Error())
	assert.Equal(t, []string{"a"}, res.Ran, "the partial result is returned alongside the error")
}

func TestRun_FailureWithoutErrorLineNamesExit(t *testing.T) {
	path, _ := fakeEmm(t, "exit 7")
	_, err := Run(context.Background(), RunOptions{Runner: Runner{Path: path}, GameDir: "/g"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 7")
}

func TestRun_CancelReturnsContextError(t *testing.T) {
	path, _ := fakeEmm(t, "echo started\nexec sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	go func() {
		<-started
		cancel()
	}()
	begin := time.Now()
	_, err := Run(ctx, RunOptions{Runner: Runner{Path: path}, GameDir: "/g", Log: func(string) {
		select {
		case started <- struct{}{}:
		default:
		}
	}})
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(begin), 20*time.Second)
}

func TestVersion(t *testing.T) {
	path, _ := fakeEmm(t, "exit 0")
	v, err := Version(context.Background(), Runner{Path: path})
	require.NoError(t, err)
	assert.Equal(t, "emm 0.0.0-fake", v)

	_, err = Version(context.Background(), Runner{Path: filepath.Join(t.TempDir(), "nope")})
	require.Error(t, err)
}

// fakeWine writes a wine and a wineserver standing in for Wine. The wine
// records its argv and the Wine settings of its environment, then runs body.
func fakeWine(t *testing.T, body string) (w wine.Wine, dir string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not on PATH")
	}
	dir = t.TempDir()
	script := "#!" + bash + "\n" +
		"printf '%s\\n' \"$@\" > '" + filepath.Join(dir, "argv.txt") + "'\n" +
		"printf '%s\\n' \"WINEPREFIX=$WINEPREFIX\" \"WINEDEBUG=$WINEDEBUG\" \"WINEDLLOVERRIDES=$WINEDLLOVERRIDES\" " +
		"\"DISPLAY=${DISPLAY-unset}\" \"WAYLAND_DISPLAY=${WAYLAND_DISPLAY-unset}\" > '" + filepath.Join(dir, "env.txt") + "'\n" +
		body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wine"), []byte(script), 0o755)) //nolint:gosec // test executable
	server := "#!" + bash + "\nprintf '%s %s\\n' \"$*\" \"$WINEPREFIX\" >> '" + filepath.Join(dir, "server.txt") + "'\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wineserver"), []byte(server), 0o755)) //nolint:gosec // test executable
	return wine.Wine{Bin: filepath.Join(dir, "wine"), Source: "test"}, dir
}

func TestRun_UnderWineTranslatesPathsAndSetsThePrefix(t *testing.T) {
	w, dir := fakeWine(t, "echo 'Applied 2 patches'")
	t.Setenv("DISPLAY", ":9")
	t.Setenv("WAYLAND_DISPLAY", "wayland-9")
	r := Runner{Path: "/tools/emm-v0/emm.exe", Wine: &w, Prefix: "/tools/wineprefix"}
	res, err := Run(context.Background(), RunOptions{Runner: r, GameDir: "/work/shadow", Patch: true,
		Export: true, ExportDir: "/work/export"})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Applied)
	assert.Equal(t, []string{"/tools/emm-v0/emm.exe", "run", "-g", `Z:\work\shadow`, "--patch",
		"--export", "--export-directory", `Z:\work\export`}, readArgv(t, filepath.Join(dir, "argv.txt")),
		"wine gets emm.exe first, then emm's own argv with paths on Wine's Z: drive")
	assert.Equal(t, []string{"WINEPREFIX=/tools/wineprefix", "WINEDEBUG=-all", "WINEDLLOVERRIDES=mscoree,mshtml=",
		"DISPLAY=unset", "WAYLAND_DISPLAY=unset"}, readArgv(t, filepath.Join(dir, "env.txt")),
		"the tool's own prefix, no mono or Gecko dialogs, and no display")
	assert.NoFileExists(t, filepath.Join(dir, "server.txt"), "a run that finished leaves the prefix alone")
}

func TestRun_CancelUnderWineStopsThePrefix(t *testing.T) {
	w, dir := fakeWine(t, "echo started\nexec sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	r := Runner{Path: "/t/emm.exe", Wine: &w, Prefix: "/t/wineprefix"}
	_, err := Run(ctx, RunOptions{Runner: r, GameDir: "/g", Log: func(string) { cancel() }})
	require.ErrorIs(t, err, context.Canceled)
	b, err := os.ReadFile(filepath.Join(dir, "server.txt"))
	require.NoError(t, err, "a cancel runs wineserver -k")
	assert.Equal(t, "-k /t/wineprefix\n", string(b), "for the tool's prefix, not the game's")
}

// serve answers the release and licence URLs from memory.
func serve(t *testing.T, exe []byte, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/LICENSE":
			_, _ = w.Write([]byte("GPL-3.0 text"))
		case "/emm.exe":
			w.WriteHeader(status)
			_, _ = w.Write(exe)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestGet_KeepsADownloadMatchingThePin(t *testing.T) {
	exe := []byte("MZ synthetic emm")
	srv := serve(t, exe, http.StatusOK)
	tools := t.TempDir()
	var last int64
	p, err := Get(context.Background(), GetOptions{ToolsDir: tools, URL: srv.URL + "/emm.exe",
		LicenseURL: srv.URL + "/LICENSE", SHA256: sum(exe), Progress: func(done, _ int64) { last = done }})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(InstallDir(tools), ExeName), p)
	got, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, exe, got)
	assert.FileExists(t, filepath.Join(InstallDir(tools), "LICENSE"), "upstream's licence travels with the binary")
	assert.Equal(t, int64(len(exe)), last)
	entries, err := os.ReadDir(InstallDir(tools))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no temporary files are left behind")
}

func TestGet_RefusesAndDeletesADownloadNotMatchingThePin(t *testing.T) {
	exe := []byte("MZ tampered")
	srv := serve(t, exe, http.StatusOK)
	tools := t.TempDir()
	want := sum([]byte("something else"))
	_, err := Get(context.Background(), GetOptions{ToolsDir: tools, URL: srv.URL + "/emm.exe",
		LicenseURL: srv.URL + "/LICENSE", SHA256: want})
	require.Error(t, err)
	assert.Contains(t, err.Error(), sum(exe), "the error names the hash received")
	assert.Contains(t, err.Error(), want, "and the hash expected")
	assert.NoFileExists(t, filepath.Join(InstallDir(tools), ExeName))
	assert.NoFileExists(t, filepath.Join(InstallDir(tools), ExeName+".download"))
}

func TestGet_HTTPErrorIsReported(t *testing.T) {
	srv := serve(t, nil, http.StatusNotFound)
	_, err := Get(context.Background(), GetOptions{ToolsDir: t.TempDir(), URL: srv.URL + "/emm.exe",
		LicenseURL: srv.URL + "/LICENSE", SHA256: sum(nil)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestVerify(t *testing.T) {
	p := filepath.Join(t.TempDir(), ExeName)
	require.NoError(t, os.WriteFile(p, []byte("not the release"), 0o600))
	err := Verify(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ExeSHA256)
}
