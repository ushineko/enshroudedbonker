package tweaks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNames_NonEmptySortedAndHas(t *testing.T) {
	names := Names()
	require.NotEmpty(t, names)
	assert.True(t, slices.IsSorted(names))
	for _, n := range names {
		assert.True(t, Has(n), n)
	}
	assert.False(t, Has("no_such_tweak"))
	_, ok := Get("no_such_tweak")
	assert.False(t, ok)
	_, ok = Source("../tweaks")
	assert.False(t, ok, "Source must only serve names that are built-ins")
}

func TestBuiltins_HeaderNameAndKnownGroup(t *testing.T) {
	for _, n := range Names() {
		src, ok := Source(n)
		require.True(t, ok, n)
		h := ParseHeader(src)
		assert.NotEmpty(t, h.Name, "%s: no @tweak name", n)
		assert.NotEmpty(t, h.Desc, "%s: no @desc", n)
		assert.Contains(t, Groups, h.Group, "%s: group %q would land on %s", n, h.Group, GroupOther)

		tw, ok := Get(n)
		require.True(t, ok)
		assert.NotEqual(t, GroupOther, tw.Header.Group, n)
	}
}

func TestBuiltins_ParamsAreDeclaredAssignedAndInRange(t *testing.T) {
	for _, n := range Names() {
		src, _ := Source(n)
		assigned := assignments(src)
		seen := map[string]bool{}
		for _, p := range Parameters(src) {
			assert.False(t, seen[p.Name], "%s: %s declared twice", n, p.Name)
			seen[p.Name] = true
			_, ok := assigned[p.Name]
			assert.True(t, ok, "%s: @param %s has no `local %s = value` line", n, p.Name, p.Name)
			assert.Less(t, p.Min, p.Max, "%s.%s: empty range", n, p.Name)
			assert.GreaterOrEqual(t, p.Default, p.Min, "%s.%s default below min", n, p.Name)
			assert.LessOrEqual(t, p.Default, p.Max, "%s.%s default above max", n, p.Name)
			assert.Greater(t, p.Step, 0.0, "%s.%s", n, p.Name)
			assert.Contains(t, []string{KindInt, KindFloat, KindBool}, p.Kind)
			assert.NotEqual(t, p.Name, p.Label, "%s.%s has no label", n, p.Name)
			assert.Equal(t, p.Default, p.Current)
			if c, changed := p.Clamp(p.Default); changed {
				t.Errorf("%s.%s: default %v is not a legal value of its kind (clamps to %v)", n, p.Name, p.Default, c)
			}
		}
	}
}

func TestBuiltins_HeaderDefaultAgreesWithAssignment(t *testing.T) {
	// Parameters takes the assignment as the default; a header that drifted
	// from it would show the user one number and build with another.
	attr := regexp.MustCompile(`default=([^ ]+)`)
	for _, n := range Names() {
		src, _ := Source(n)
		params := Parameters(src)
		for _, line := range strings.Split(string(src), "\n") {
			m := paramLineRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			d := attr.FindStringSubmatch(m[2])
			if d == nil {
				continue
			}
			want, _ := parseValue(d[1])
			for _, p := range params {
				if p.Name == m[1] {
					assert.InDelta(t, want, p.Default, 1e-9, "%s.%s: header default %s, assignment %v", n, p.Name, d[1], p.Default)
				}
			}
		}
	}
}

func TestBuiltins_RenderWithoutOverridesIsSource(t *testing.T) {
	for _, n := range Names() {
		src, _ := Source(n)
		got, err := Render(n, nil)
		require.NoError(t, err)
		assert.Equal(t, string(src), string(got), n)
		got, err = Render(n, map[string]float64{})
		require.NoError(t, err)
		assert.Equal(t, string(src), string(got), n)
	}
}

func TestBuiltins_OverrideChangesOnlyThatLine(t *testing.T) {
	for _, n := range Names() {
		tw, _ := Get(n)
		src, _ := Source(n)
		before := strings.Split(string(src), "\n")
		for _, p := range tw.Params {
			v := p.Max
			if v == p.Default {
				v = p.Min
			}
			out, err := Render(n, map[string]float64{p.Name: v})
			require.NoError(t, err, "%s.%s", n, p.Name)
			after := strings.Split(string(out), "\n")
			require.Len(t, after, len(before), "%s.%s: line count changed", n, p.Name)
			var changed []int
			for i := range before {
				if before[i] != after[i] {
					changed = append(changed, i)
				}
			}
			require.Len(t, changed, 1, "%s.%s: exactly one line must change", n, p.Name)
			line := after[changed[0]]
			assert.True(t, strings.HasPrefix(line, "local "+p.Name+" = "+FormatValue(v, p.Kind)),
				"%s.%s: got %q", n, p.Name, line)
			for _, q := range Parameters(out) {
				if q.Name == p.Name {
					assert.Equal(t, v, q.Current, "re-reading the render must yield the override")
				}
			}
		}
	}
}

func TestRender_UnknownOverrideIsAnError(t *testing.T) {
	_, err := Render("big_stacks", map[string]float64{"NOT_A_PARAM": 1})
	require.Error(t, err, "a value the user set must never be silently dropped")
	_, err = Render("no_such_tweak", nil)
	require.Error(t, err)
}

func TestOverride_MissingAssignmentIsErrNoAssignment(t *testing.T) {
	_, err := Override([]byte("local A = 1\n"), "B", 2, KindInt)
	require.ErrorIs(t, err, ErrNoAssignment)
}

func TestOverride_KeepsIndentCommentAndLaterAssignments(t *testing.T) {
	src := []byte("-- head\n  local X = 1 -- why\nprint(X)\nlocal X = 7\n")
	out, err := Override(src, "X", 5, KindInt)
	require.NoError(t, err)
	assert.Equal(t, "-- head\n  local X = 5   -- why\nprint(X)\nlocal X = 7\n", string(out))
	assert.Equal(t, "-- head\n  local X = 1 -- why\nprint(X)\nlocal X = 7\n", string(src), "the input must not be mutated")
}

func TestOverride_PrefixNameIsNotMatched(t *testing.T) {
	src := []byte("local XY = 1\nlocal X = 2\n")
	out, err := Override(src, "X", 3, KindInt)
	require.NoError(t, err)
	assert.Equal(t, "local XY = 1\nlocal X = 3\n", string(out))
}

func TestBoolParamsFormatTrueFalse(t *testing.T) {
	out, err := Render("fast_travel_markers", map[string]float64{"DUNGEONS": 0})
	require.NoError(t, err)
	assert.Contains(t, string(out), "\nlocal DUNGEONS = false\n")
	assert.NotContains(t, string(out), "DUNGEONS = 0", "Lua treats 0 as true; a bool must render as false")

	tw, _ := Get("fast_travel_markers")
	for _, p := range tw.Params {
		assert.Equal(t, KindBool, p.Kind)
		assert.Equal(t, 1.0, p.Default, "`local X = true` reads as 1")
	}
	assert.Equal(t, "true", FormatValue(1, KindBool))
	assert.Equal(t, "true", FormatValue(0.3, KindBool))
	assert.Equal(t, "false", FormatValue(0, KindBool))
}

func TestFormatValue_IntAndFloat(t *testing.T) {
	assert.Equal(t, "3", FormatValue(2.6, KindInt))
	assert.Equal(t, "-2", FormatValue(-2.4, KindInt))
	assert.Equal(t, "2.0", FormatValue(2, KindFloat), "a float must stay a float literal")
	assert.Equal(t, "0.05", FormatValue(0.05, KindFloat))
	assert.Equal(t, "1000000000000000000000.0", FormatValue(1e21, KindFloat), "never an exponent without a point")
	assert.Equal(t, "1.5", FormatValue(1.5, ""))
}

// A slider sums its steps, and the sum carries float error: 1.0 plus two steps
// of 0.1 is 1.2000000000000002. The value reads as the step it was set to, in
// the window, the CLI and the Lua line alike (issue #18).
func TestFormatValue_DropsFloatErrorFromSteps(t *testing.T) {
	// Variables, so the sums happen at run time: Go folds constants exactly.
	v, step := 1.0, 0.1
	v += step
	v += step
	require.NotEqual(t, 1.2, v, "the sum must carry the error this test is about")
	assert.Equal(t, "1.2", FormatValue(v, KindFloat))
	assert.Equal(t, "1.5", FormatValue(1.5000000000000002, KindFloat))
	a, b := 0.1, 0.2
	assert.Equal(t, "0.3", FormatValue(a+b, KindFloat))
	assert.Equal(t, "0.0", FormatValue(-1e-12, KindFloat), "no negative zero")
	assert.Equal(t, "0.000001", FormatValue(1e-6, KindFloat), "a small value that is meant survives")
}

func TestClamp(t *testing.T) {
	i := Param{Kind: KindInt, Min: 1, Max: 10}
	for _, tc := range []struct {
		p       Param
		in, out float64
		changed bool
	}{
		{i, 5, 5, false},
		{i, 4.6, 5, true},
		{i, 0, 1, true},
		{i, 99, 10, true},
		{Param{Kind: KindFloat, Min: 0.05, Max: 1}, 0.333, 0.333, false},
		{Param{Kind: KindFloat, Min: 0.05, Max: 1}, 0, 0.05, true},
		{Param{Kind: KindFloat, Min: 0.05, Max: 1}, 2, 1, true},
		{Param{Kind: KindBool, Min: 0, Max: 1}, 0, 0, false},
		{Param{Kind: KindBool, Min: 0, Max: 1}, 1, 1, false},
		{Param{Kind: KindBool, Min: 0, Max: 1}, 7, 1, true},
		{Param{Kind: KindBool, Min: 0, Max: 1}, -1, 1, true},
		{Param{Kind: KindInt}, 1e6, 1e6, false}, // no range declared: no bound
	} {
		got, changed := tc.p.Clamp(tc.in)
		assert.Equal(t, tc.out, got, "%+v Clamp(%v)", tc.p, tc.in)
		assert.Equal(t, tc.changed, changed, "%+v Clamp(%v) changed", tc.p, tc.in)
	}
}

func TestParameters_BoolForcesRangeAndMissingKindIsInt(t *testing.T) {
	src := []byte(`-- @param B label="b" min=5 max=9 step=3 kind=bool
-- @param N label="n" min=1 max=3
local B = false
local N = 2
`)
	ps := Parameters(src)
	require.Len(t, ps, 2)
	assert.Equal(t, Param{Name: "B", Label: "b", Min: 0, Max: 1, Step: 1, Default: 0, Current: 0, Kind: KindBool}, ps[0])
	assert.Equal(t, KindInt, ps[1].Kind)
	assert.Equal(t, 1.0, ps[1].Step)
	assert.Equal(t, 2.0, ps[1].Default)
}

func TestParseHeader_UnquotedAndMultilineDesc(t *testing.T) {
	h := ParseHeader([]byte("--@tweak name=Solo group=loot\n-- @desc one\n--   @desc two  \nlocal x = 1\n"))
	assert.Equal(t, "Solo", h.Name)
	assert.Equal(t, "loot", h.Group)
	assert.Equal(t, "one two", h.Desc)
	assert.Equal(t, "Loot", normaliseGroup("LOOT"))
	assert.Equal(t, GroupOther, normaliseGroup("Weather"))
}

func TestOverrideAll_StopsAtFirstMissingInNameOrder(t *testing.T) {
	src := []byte("local B = 1\n")
	_, err := OverrideAll(src, map[string]float64{"C": 1, "A": 1, "B": 2}, map[string]string{})
	require.ErrorIs(t, err, ErrNoAssignment)
	assert.Contains(t, err.Error(), ": A", "failures must name the same parameter on every run")
}

func TestBuiltins_ManifestValid(t *testing.T) {
	for _, n := range Names() {
		b, ok := Manifest(n)
		require.True(t, ok, n)
		var m struct {
			ID           string   `json:"id"`
			Name         string   `json:"name"`
			Version      string   `json:"version"`
			Capabilities []string `json:"capabilities"`
			Description  string   `json:"description"`
			License      string   `json:"license"`
		}
		require.NoError(t, json.Unmarshal(b, &m), n)
		assert.Equal(t, n, m.ID, "%s: id must equal the directory name (it is the config key)", n)
		assert.NotEmpty(t, m.Name, n)
		assert.NotEmpty(t, m.Version, n)
		assert.NotEmpty(t, m.Description, n)
		assert.Equal(t, []string{"patch"}, m.Capabilities, "%s: built-ins patch offline only", n)
		assert.Equal(t, "MIT", m.License, n)
	}
}

func TestStage_WritesEMLLayoutWithOverride(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Stage(dir, "big_stacks", map[string]float64{"STACK_MULTIPLIER": 20}))
	lua, err := os.ReadFile(filepath.Join(dir, "big_stacks", "src", "mod.lua"))
	require.NoError(t, err)
	assert.Contains(t, string(lua), "\nlocal STACK_MULTIPLIER = 20\n")
	man, err := os.ReadFile(filepath.Join(dir, "big_stacks", "mod.json"))
	require.NoError(t, err)
	want, _ := Manifest("big_stacks")
	assert.Equal(t, want, man)

	src, _ := Source("big_stacks")
	assert.Contains(t, string(src), "\nlocal STACK_MULTIPLIER = 10\n", "the embedded source is never touched")

	require.Error(t, Stage(dir, "big_stacks", map[string]float64{"NOPE": 1}))
}

func TestSource_ReturnsACopy(t *testing.T) {
	a, _ := Source("big_stacks")
	a[0] = '!'
	b, _ := Source("big_stacks")
	assert.NotEqual(t, a[0], b[0])
}
