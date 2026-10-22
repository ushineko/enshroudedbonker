package tweaks

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

/*
The parameter contract (spec 002 R5.2), carried over from nmsbonker with one
change: EML mods are ordinary Lua chunks, so the tunable constants are
`local NAME = <value>` lines rather than globals, and a value may be a boolean.

A built-in declares each parameter with a `-- @param` header line naming its
label, bounds and kind, and assigns it once near the top. The builder rewrites
that one line in a staged copy; nothing else in the pipeline knows a parameter
exists, and the file itself is never touched.
*/

// Parameter kinds, as spelled in a header's kind= attribute.
const (
	KindInt   = "int"
	KindFloat = "float"
	KindBool  = "bool"
)

// Param is one tunable constant.
type Param struct {
	Name    string  `json:"name"`
	Label   string  `json:"label"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Step    float64 `json:"step"`
	Default float64 `json:"default"`
	Kind    string  `json:"kind"`
	// Current is the value in force: Default until an override says otherwise.
	Current float64 `json:"current"`
}

// Header is the `-- @tweak` block.
type Header struct {
	Name  string `json:"name,omitempty"`
	Group string `json:"group,omitempty"`
	Desc  string `json:"desc,omitempty"`
}

const number = `[-+]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][-+]?[0-9]+)?`

//nolint:gochecknoglobals // compiled once
var (
	tweakLineRE = regexp.MustCompile(`^\s*--\s*@tweak\s+(.*)$`)
	descLineRE  = regexp.MustCompile(`^\s*--\s*@desc\s?(.*)$`)
	paramLineRE = regexp.MustCompile(`^\s*--\s*@param\s+([A-Za-z_][A-Za-z0-9_]*)\s*(.*)$`)
	attrRE      = regexp.MustCompile(`([a-z]+)\s*=\s*(?:"([^"]*)"|(` + number + `)|([A-Za-z]+))`)
	assignRE    = regexp.MustCompile(`^(\s*)local\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(` + number + `|true|false)\s*(--.*)?$`)
)

// ErrNoAssignment reports an override for a name the mod does not assign.
var ErrNoAssignment = errors.New("no `local NAME = value` line to substitute")

// ParseHeader reads the `-- @tweak` and `-- @desc` lines.
func ParseHeader(src []byte) Header {
	var h Header
	var desc []string
	for _, line := range strings.Split(string(src), "\n") {
		if m := tweakLineRE.FindStringSubmatch(line); m != nil {
			for _, a := range attrRE.FindAllStringSubmatch(m[1], -1) {
				switch a[1] {
				case "name":
					h.Name = attrText(a)
				case "group":
					h.Group = attrText(a)
				}
			}
			continue
		}
		if m := descLineRE.FindStringSubmatch(line); m != nil {
			desc = append(desc, strings.TrimSpace(m[1]))
		}
	}
	h.Desc = strings.Join(desc, " ")
	return h
}

// Parameters lists the declared parameters, each with the value the mod's own
// assignment carries as its default (the assignment is the authority; a header
// that has drifted from it would make Reset restore a number the mod never
// had).
func Parameters(src []byte) []Param {
	assigned := assignments(src)
	var out []Param
	for _, line := range strings.Split(string(src), "\n") {
		m := paramLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		p := Param{Name: m[1], Label: m[1], Kind: KindInt, Step: 1}
		for _, a := range attrRE.FindAllStringSubmatch(m[2], -1) {
			switch a[1] {
			case "label":
				p.Label = attrText(a)
			case "min":
				p.Min = attrNumber(a)
			case "max":
				p.Max = attrNumber(a)
			case "step":
				p.Step = attrNumber(a)
			case "default":
				p.Default = attrNumber(a)
			case "kind":
				switch attrText(a) {
				case KindFloat:
					p.Kind = KindFloat
				case KindBool:
					p.Kind = KindBool
				}
			}
		}
		if p.Kind == KindBool {
			p.Min, p.Max, p.Step = 0, 1, 1
		}
		if v, ok := assigned[p.Name]; ok {
			p.Default = v
		}
		p.Current = p.Default
		out = append(out, p)
	}
	return out
}

// assignments maps the first `local NAME = value` of each name to its value.
func assignments(src []byte) map[string]float64 {
	out := map[string]float64{}
	for _, line := range strings.Split(string(src), "\n") {
		m := assignRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if _, seen := out[m[2]]; seen {
			continue
		}
		if v, ok := parseValue(m[3]); ok {
			out[m[2]] = v
		}
	}
	return out
}

func parseValue(s string) (float64, bool) {
	switch s {
	case "true":
		return 1, true
	case "false":
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// Clamp keeps a value inside a parameter's range and on its kind: a bool is 0
// or 1, an int is whole. It reports whether it had to change the value.
func (p Param) Clamp(v float64) (float64, bool) {
	orig := v
	switch p.Kind {
	case KindBool:
		if v != 0 {
			v = 1
		}
	case KindInt:
		v = math.Round(v)
	}
	if p.Max > p.Min {
		v = math.Max(p.Min, math.Min(p.Max, v))
	}
	return v, v != orig
}

// FormatValue renders a value as the Lua literal of its kind.
func FormatValue(v float64, kind string) string {
	switch kind {
	case KindBool:
		if v != 0 {
			return "true"
		}
		return "false"
	case KindInt:
		return strconv.FormatInt(int64(math.Round(v)), 10)
	}
	text := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

// Override rewrites the first `local NAME = value` line in a copy of the
// source, keeping its indentation and trailing comment.
func Override(src []byte, name string, value float64, kind string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		m := assignRE.FindStringSubmatch(line)
		if m == nil || m[2] != name {
			continue
		}
		rest := ""
		if m[4] != "" {
			rest = "   " + m[4]
		}
		lines[i] = m[1] + "local " + name + " = " + FormatValue(value, kind) + rest
		return []byte(strings.Join(lines, "\n")), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNoAssignment, name)
}

// OverrideAll applies every override, in name order so a failure names the
// same parameter on every run.
func OverrideAll(src []byte, values map[string]float64, kinds map[string]string) ([]byte, error) {
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	out := src
	for _, n := range names {
		next, err := Override(out, n, values[n], kinds[n])
		if err != nil {
			return nil, err
		}
		out = next
	}
	return out, nil
}

func attrText(m []string) string {
	switch {
	case m[2] != "":
		return m[2]
	case m[3] != "":
		return m[3]
	}
	return m[4]
}

func attrNumber(m []string) float64 {
	v, err := strconv.ParseFloat(attrText(m), 64)
	if err != nil {
		return 0
	}
	return v
}
