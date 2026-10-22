package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/cli"
)

// sandbox aims HOME, XDG, Steam and the settings file at throwaway paths and
// returns the settings file every run is given with --config.
func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("STEAM_ROOT", filepath.Join(root, "no-steam"))
	t.Setenv("ENSHROUDEDBONKER_CONFIG", "")
	t.Setenv("ENSHROUDEDBONKER_GAME_DIR", "")
	return filepath.Join(root, "settings.json")
}

func run(t *testing.T, cfg string, args ...string) (string, error) {
	t.Helper()
	root := cli.Root()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"--config", cfg}, args...))
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestStatusJSON_NoGameIsStillValidJSON(t *testing.T) {
	cfg := sandbox(t)
	out, err := run(t, cfg, "status", "--json")
	require.NoError(t, err, out)
	var st struct {
		ConfigPath string `json:"config_path"`
		Install    struct {
			Found bool   `json:"found"`
			Error string `json:"error"`
		} `json:"install"`
		Emm struct {
			Installed bool   `json:"installed"`
			PinnedTag string `json:"pinned_tag"`
		} `json:"emm"`
		Tweaks int `json:"tweaks"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &st), "status --json must print only JSON:\n%s", out)
	assert.Equal(t, cfg, st.ConfigPath)
	assert.False(t, st.Install.Found)
	assert.NotEmpty(t, st.Install.Error)
	assert.False(t, st.Emm.Installed)
	assert.NotEmpty(t, st.Emm.PinnedTag)
	assert.Greater(t, st.Tweaks, 0)
}

func TestTweaksList_Text(t *testing.T) {
	cfg := sandbox(t)
	out, err := run(t, cfg, "tweaks", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "big_stacks  [off]  Big stacks (Inventory)")
	assert.Contains(t, out, "STACK_MULTIPLIER")
	assert.Contains(t, out, "DUNGEONS               true  (default true, false..true)",
		"switches print as true/false, not 1/0")
}

type listed struct {
	Tweaks []struct {
		Name   string `json:"name"`
		Params []struct {
			Name       string  `json:"name"`
			Current    float64 `json:"current"`
			Default    float64 `json:"default"`
			Overridden bool    `json:"overridden"`
		} `json:"params"`
	} `json:"tweaks"`
}

func param(t *testing.T, l listed, tweak, name string) (current float64, overridden bool) {
	t.Helper()
	for _, tw := range l.Tweaks {
		if tw.Name != tweak {
			continue
		}
		for _, p := range tw.Params {
			if p.Name == name {
				return p.Current, p.Overridden
			}
		}
	}
	t.Fatalf("%s.%s not listed", tweak, name)
	return 0, false
}

func TestTweaksSet_ShowsInListJSON(t *testing.T) {
	cfg := sandbox(t)
	out, err := run(t, cfg, "tweaks", "set", "big_stacks", "STACK_MULTIPLIER", "20")
	require.NoError(t, err, out)
	assert.Contains(t, out, "20")
	assert.NotContains(t, out, "clamped")

	out, err = run(t, cfg, "tweaks", "list", "--json")
	require.NoError(t, err)
	var l listed
	require.NoError(t, json.Unmarshal([]byte(out), &l), out)
	cur, over := param(t, l, "big_stacks", "STACK_MULTIPLIER")
	assert.Equal(t, 20.0, cur)
	assert.True(t, over)
	cur, over = param(t, l, "big_stacks", "STACK_CAP")
	assert.False(t, over, "setting one parameter must not mark its siblings")
	assert.Equal(t, 65535.0, cur)

	_, err = run(t, cfg, "tweaks", "reset", "big_stacks")
	require.NoError(t, err)
	out, err = run(t, cfg, "tweaks", "list", "--json")
	require.NoError(t, err)
	l = listed{}
	require.NoError(t, json.Unmarshal([]byte(out), &l))
	cur, over = param(t, l, "big_stacks", "STACK_MULTIPLIER")
	assert.False(t, over)
	assert.Equal(t, 10.0, cur)
}

func TestTweaksSet_BoolPrintsFalse(t *testing.T) {
	cfg := sandbox(t)
	out, err := run(t, cfg, "tweaks", "set", "fast_travel_markers", "DUNGEONS", "false")
	require.NoError(t, err, out)
	assert.Contains(t, out, "false")
	assert.NotContains(t, out, ": 0", "a switch must print as false, not 0")
}

func TestTweaksSet_ClampIsReported(t *testing.T) {
	cfg := sandbox(t)
	out, err := run(t, cfg, "tweaks", "set", "big_stacks", "STACK_MULTIPLIER", "5000")
	require.NoError(t, err)
	assert.Contains(t, out, "100 (clamped")
}

func TestUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"too few args":      {"tweaks", "set", "big_stacks", "STACK_MULTIPLIER"},
		"too many args":     {"tweaks", "set", "big_stacks", "STACK_MULTIPLIER", "1", "2"},
		"not a number":      {"tweaks", "set", "big_stacks", "STACK_MULTIPLIER", "lots"},
		"unknown flag":      {"status", "--frobnicate"},
		"unknown command":   {"frobnicate"},
		"unknown sub":       {"tweaks", "frobnicate"},
		"extra arg to noop": {"status", "extra"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := sandbox(t)
			_, err := run(t, cfg, args...)
			require.Error(t, err)
			var ue *cli.UsageError
			assert.True(t, errors.As(err, &ue), "%v must be a UsageError so main exits 2", err)
		})
	}
}

func TestOperationalErrorIsNotUsageError(t *testing.T) {
	cfg := sandbox(t)
	_, err := run(t, cfg, "tweaks", "set", "no_such_tweak", "X", "1")
	require.Error(t, err)
	var ue *cli.UsageError
	assert.False(t, errors.As(err, &ue), "a well-formed command that fails is exit 1, not 2")
}

func TestConfigSetShowRoundTrip(t *testing.T) {
	cfg := sandbox(t)
	dir := filepath.Join(filepath.Dir(cfg), "elsewhere")
	_, err := run(t, cfg, "config", "set", "library_dir", dir)
	require.NoError(t, err)
	out, err := run(t, cfg, "config", "show")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, dir), out)
}
