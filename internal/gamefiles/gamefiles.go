/*
Package gamefiles reads the few facts about the game's own files that the tool
needs without parsing the archive format (spec 002 R2.1): the build string the
executable and the archive both carry, content hashes, and whether an install
looks patched.

Everything else about the format belongs to upstream's emm. This package reads
bytes and nothing it reads is interpreted beyond a string match.
*/
package gamefiles

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

/*
versionRE matches the build string Keen's pipeline stamps into the archive
index, enshrouded.kfc, e.g.

	1269579|^/game38/branches/release_1.0|2026-10-16T12:58:37.402390Z

revision | branch | build timestamp. Found by inspection (spec 001); it is
what emm prints as the type registry's version too. The executable does not
carry it (an early note said it did; it was wrong). The executable carries the
source commit instead, see commitRE.
*/
var versionRE = regexp.MustCompile(`([0-9]{5,10})\|(\^/[\x21-\x7e]{1,200}?)\|([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z)`)

/*
commitRE matches the source commit the executable was built from: 40 hex
digits, NUL padding, then a dotted four-part version. The same commit is what
the game writes to enshrouded.log at start-up ("version ef98…").
*/
var commitRE = regexp.MustCompile(`([0-9a-f]{40})\x00+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)\x00`)

// ReadCommit reads the source commit and engine version from the executable.
func ReadCommit(path string) (commit, engine string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}
	m := commitRE.FindSubmatch(b)
	if m == nil {
		return "", "", fmt.Errorf("%s: %w", path, ErrNoVersion)
	}
	return string(m[1]), string(m[2]), nil
}

// ErrNoVersion reports a file that carries no build string.
var ErrNoVersion = errors.New("no build string found")

// Version is a parsed build string.
type Version struct {
	Raw      string `json:"raw"`
	Revision string `json:"revision"`
	Branch   string `json:"branch"`
	Built    string `json:"built"`
}

// Short is "1269579 (release_1.0)": what a status line has room for.
func (v Version) Short() string {
	if v.Raw == "" {
		return ""
	}
	b := v.Branch
	if i := strings.LastIndex(b, "/"); i >= 0 {
		b = b[i+1:]
	}
	return v.Revision + " (" + b + ")"
}

// FindVersion scans a byte slice for the build string.
func FindVersion(b []byte) (Version, error) {
	m := versionRE.FindSubmatch(b)
	if m == nil {
		return Version{}, ErrNoVersion
	}
	return Version{Raw: string(m[0]), Revision: string(m[1]), Branch: string(m[2]), Built: string(m[3])}, nil
}

// ReadVersion reads the build string from the first limit bytes of a file
// (0 = the whole file). The archive index carries it in its header.
func ReadVersion(path string, limit int64) (Version, error) {
	f, err := os.Open(path)
	if err != nil {
		return Version{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	var r io.Reader = f
	if limit > 0 {
		r = io.LimitReader(f, limit)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return Version{}, fmt.Errorf("read %s: %w", path, err)
	}
	v, err := FindVersion(b)
	if err != nil {
		return Version{}, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// HashFile is a file's SHA-256, hex.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashPrefix hashes the first n bytes of a file. Used to prove that a patched
// resource file still starts with the pristine one: emm appends and never
// rewrites what was there.
func HashPrefix(path string, n int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	got, err := io.Copy(h, io.LimitReader(f, n))
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	if got != n {
		return "", fmt.Errorf("%s is %d bytes, shorter than %d", path, got, n)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Equal reports whether two files have the same bytes. Sizes first, so the
// common "different" answer costs a stat.
func Equal(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", a, err)
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", b, err)
	}
	if ia.Size() != ib.Size() {
		return false, nil
	}
	fa, err := os.Open(a)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", a, err)
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", b, err)
	}
	defer fb.Close()
	const chunk = 1 << 20
	ba, bb := make([]byte, chunk), make([]byte, chunk)
	for {
		na, ea := io.ReadFull(fa, ba)
		nb, eb := io.ReadFull(fb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		if ea != nil || eb != nil {
			return errors.Is(ea, io.EOF) || errors.Is(ea, io.ErrUnexpectedEOF), nil
		}
	}
}
