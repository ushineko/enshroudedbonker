package gamefiles

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A synthetic build string with the shape the archive index carries. The
// numbers are made up.
const synthBuild = "1234567|^/game38/branches/release_1.0|2026-10-16T12:58:37.402390Z"

// A synthetic commit blob: 40 hex digits, NUL padding, a dotted version, NUL.
const synthCommit = "0123456789abcdef0123456789abcdef01234567"

func blob(parts ...string) []byte {
	var b bytes.Buffer
	for _, p := range parts {
		b.WriteString(p)
	}
	return b.Bytes()
}

func writeFile(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, b, 0o600))
	return p
}

func TestFindVersion_InBinaryNoise(t *testing.T) {
	b := blob("\x00\x01KFC2\xff\xfe", "junk|^/x|", synthBuild, "\x00\x00\x07trailing")
	v, err := FindVersion(b)
	require.NoError(t, err)
	assert.Equal(t, synthBuild, v.Raw)
	assert.Equal(t, "1234567", v.Revision)
	assert.Equal(t, "^/game38/branches/release_1.0", v.Branch)
	assert.Equal(t, "2026-10-16T12:58:37.402390Z", v.Built)
	assert.Equal(t, "1234567 (release_1.0)", v.Short())
}

func TestFindVersion_RejectsLookalikes(t *testing.T) {
	for _, s := range []string{
		"",
		"1234|^/game38/branches/release_1.0|2026-10-16T12:58:37Z",      // revision too short
		"1234567|/game38/branches/release_1.0|2026-10-16T12:58:37Z",    // branch without ^
		"1234567|^/game38/branches/release_1.0|2026-10-16",             // no time
		"1234567|^/game38 branches/release_1.0|2026-10-16T12:58:37.4Z", // space in branch
	} {
		_, err := FindVersion([]byte(s))
		assert.ErrorIs(t, err, ErrNoVersion, "%q", s)
	}
}

func TestVersionShort_EmptyAndNoSlash(t *testing.T) {
	assert.Equal(t, "", Version{}.Short())
	assert.Equal(t, "1 (^main)", Version{Raw: "x", Revision: "1", Branch: "^main"}.Short())
}

func TestReadVersion_LimitHidesLateString(t *testing.T) {
	pad := strings.Repeat("\x00", 4096)
	p := writeFile(t, "enshrouded.kfc", blob(pad, synthBuild))

	v, err := ReadVersion(p, 0)
	require.NoError(t, err)
	assert.Equal(t, synthBuild, v.Raw)

	_, err = ReadVersion(p, 1024)
	require.ErrorIs(t, err, ErrNoVersion, "the limit must bound how much is read")

	_, err = ReadVersion(filepath.Join(t.TempDir(), "missing"), 0)
	require.Error(t, err)
}

func TestReadCommit_SyntheticBlob(t *testing.T) {
	p := writeFile(t, "enshrouded.exe", blob("MZ\x90\x00", "padding", synthCommit, "\x00\x00\x00", "0.0.15.0", "\x00rest"))
	commit, engine, err := ReadCommit(p)
	require.NoError(t, err)
	assert.Equal(t, synthCommit, commit)
	assert.Equal(t, "0.0.15.0", engine)
}

func TestReadCommit_RequiresNULFraming(t *testing.T) {
	for name, b := range map[string][]byte{
		"no NUL between":    blob(synthCommit, "0.0.15.0\x00"),
		"no trailing NUL":   blob(synthCommit, "\x00", "0.0.15.0"),
		"three-part number": blob(synthCommit, "\x00", "0.15.0\x00"),
		"upper-case hex":    blob(strings.ToUpper(synthCommit), "\x00", "0.0.15.0\x00"),
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := ReadCommit(writeFile(t, "e.exe", b))
			require.ErrorIs(t, err, ErrNoVersion)
		})
	}
	_, _, err := ReadCommit(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestHashFile_MatchesSHA256(t *testing.T) {
	data := []byte("synthetic archive bytes")
	got, err := HashFile(writeFile(t, "f", data))
	require.NoError(t, err)
	assert.Equal(t, sum(data), got)

	empty, err := HashFile(writeFile(t, "e", nil))
	require.NoError(t, err)
	assert.Equal(t, sum(nil), empty)

	_, err = HashFile(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestHashPrefix_ProvesAppendOnly(t *testing.T) {
	pristine := []byte("ORIGINAL-RESOURCES")
	patched := append(append([]byte(nil), pristine...), []byte("APPENDED")...)
	p := writeFile(t, "res", patched)

	got, err := HashPrefix(p, int64(len(pristine)))
	require.NoError(t, err)
	assert.Equal(t, sum(pristine), got)

	rewritten := writeFile(t, "res2", append([]byte("X"), patched[1:]...))
	got, err = HashPrefix(rewritten, int64(len(pristine)))
	require.NoError(t, err)
	assert.NotEqual(t, sum(pristine), got)

	_, err = HashPrefix(writeFile(t, "short", pristine[:3]), int64(len(pristine)))
	require.Error(t, err, "a file shorter than the prefix must not hash as if it matched")
}

func TestEqual(t *testing.T) {
	a := writeFile(t, "a", []byte("same bytes"))
	b := writeFile(t, "b", []byte("same bytes"))
	c := writeFile(t, "c", []byte("same bytez"))
	d := writeFile(t, "d", []byte("longer bytes"))

	for _, tc := range []struct {
		x, y string
		want bool
	}{{a, b, true}, {a, c, false}, {a, d, false}, {a, a, true}} {
		got, err := Equal(tc.x, tc.y)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "%s vs %s", filepath.Base(tc.x), filepath.Base(tc.y))
	}
	_, err := Equal(a, filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestEqual_AcrossChunkBoundary(t *testing.T) {
	const chunk = 1 << 20
	big := bytes.Repeat([]byte{0xA5}, chunk+17)
	other := append([]byte(nil), big...)
	other[chunk+3] = 0x5A
	exact := bytes.Repeat([]byte{1}, chunk)

	x, y, z := writeFile(t, "x", big), writeFile(t, "y", append([]byte(nil), big...)), writeFile(t, "z", other)
	eq, err := Equal(x, y)
	require.NoError(t, err)
	assert.True(t, eq)
	eq, err = Equal(x, z)
	require.NoError(t, err)
	assert.False(t, eq, "a difference in the second chunk must be seen")

	e1, e2 := writeFile(t, "e1", exact), writeFile(t, "e2", exact)
	eq, err = Equal(e1, e2)
	require.NoError(t, err)
	assert.True(t, eq, "a file exactly one chunk long ends on an EOF after a full read")
}

