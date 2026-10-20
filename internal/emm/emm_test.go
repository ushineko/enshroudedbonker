package emm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestLocate(t *testing.T) {
	tools := t.TempDir()
	_, _, err := Locate(tools, "")
	require.ErrorIs(t, err, ErrNotInstalled)

	_, _, err = Locate(tools, filepath.Join(tools, "missing"))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotInstalled), "a bad emm_path is named, not replaced by the pinned build")
	_, _, err = Locate(tools, tools)
	require.Error(t, err, "a directory is not an emm binary")

	pinned := filepath.Join(InstallDir(tools), Binary)
	require.NoError(t, os.MkdirAll(filepath.Dir(pinned), 0o750))
	require.NoError(t, os.WriteFile(pinned, nil, 0o600))
	p, src, err := Locate(tools, "")
	require.NoError(t, err)
	assert.Equal(t, pinned, p)
	assert.Contains(t, src, Tag)

	override := filepath.Join(tools, "my-emm")
	require.NoError(t, os.WriteFile(override, nil, 0o600))
	p, src, err = Locate(tools, override)
	require.NoError(t, err)
	assert.Equal(t, override, p, "the setting outranks the pinned build")
	assert.Equal(t, "emm_path setting", src)
}

func TestInstallAndSourceDirsCarryTheTag(t *testing.T) {
	assert.Equal(t, filepath.Join("/t", "emm-"+Tag), InstallDir("/t"))
	assert.Equal(t, filepath.Join("/t", "src", "kfc-parser-"+Tag), SourceDir("/t"))
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
	res, err := Run(context.Background(), RunOptions{Path: path, GameDir: game, Patch: true,
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
	_, err := Run(context.Background(), RunOptions{Path: path, GameDir: "/g", Export: true,
		ExportDir: "/out", Force: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"run", "-g", "/g", "--export", "--export-directory", "/out", "--force"},
		readArgv(t, argv), "export always names its directory, or emm writes into the game")
}

func TestRun_FailureReportsFirstErrorLine(t *testing.T) {
	path, _ := fakeEmm(t, "echo 'Running mod mod_id=\"a\"'\necho 'Error: the archive is not what I expected'\necho 'Error: second'\nexit 3")
	res, err := Run(context.Background(), RunOptions{Path: path, GameDir: "/g", Patch: true})
	require.Error(t, err)
	assert.Equal(t, "emm failed: Error: the archive is not what I expected", err.Error())
	assert.Equal(t, []string{"a"}, res.Ran, "the partial result is returned alongside the error")
}

func TestRun_FailureWithoutErrorLineNamesExit(t *testing.T) {
	path, _ := fakeEmm(t, "exit 7")
	_, err := Run(context.Background(), RunOptions{Path: path, GameDir: "/g"})
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
	_, err := Run(ctx, RunOptions{Path: path, GameDir: "/g", Log: func(string) {
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
	v, err := Version(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "emm 0.0.0-fake", v)

	_, err = Version(context.Background(), filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
}

func TestBuild_NeedsGitAndCargo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Build(context.Background(), BuildOptions{ToolsDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "on PATH")
}
