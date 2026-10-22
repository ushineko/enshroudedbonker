package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
)

func fpFor(raw, exeSHA string) Fingerprint {
	v, err := gamefiles.FindVersion([]byte(raw))
	if err != nil {
		panic(err)
	}
	return Fingerprint{Kfc: v, ExeCommit: strings.Repeat("a", 40), ExeSHA256: exeSHA}
}

// synthetic is a build no ledger will ever list.
func synthetic() Fingerprint {
	return fpFor("1234567|^/game38/branches/release_1.0|2026-01-01T00:00:00.000000Z", strings.Repeat("ab", 32))
}

func TestLedger_EmbeddedParsesAndEntriesAreComplete(t *testing.T) {
	entries := Ledger()
	require.NotEmpty(t, entries)
	for _, e := range entries {
		_, err := gamefiles.FindVersion([]byte(e.Version))
		require.NoError(t, err, "ledger version %q must be a build string the reader can find", e.Version)
		assert.Len(t, e.ExeSHA256, 64, e.Version)
		assert.NotEmpty(t, e.EmmTag, e.Version)
		_, err = time.Parse("2006-01-02", e.Verified)
		assert.NoError(t, err, "verified date of %s", e.Version)
	}
}

func TestLookup_MatchingEntryIsVerified(t *testing.T) {
	e := Ledger()[0]
	fp := fpFor(e.Version, e.ExeSHA256)
	got, ok, half := Lookup(fp)
	assert.True(t, ok)
	assert.False(t, half)
	assert.Equal(t, e, got)

	d := Decide(fp, nil)
	assert.Equal(t, Verified, d.Verdict)
	assert.True(t, d.Buildable())
	require.NotNil(t, d.Ledger)
	assert.Equal(t, e.Disabled, d.Disabled)
}

func TestLookup_KfcMatchExeMismatchIsInconsistent(t *testing.T) {
	e := Ledger()[0]
	fp := fpFor(e.Version, strings.Repeat("0", 64))
	_, ok, half := Lookup(fp)
	assert.False(t, ok)
	assert.True(t, half)

	// Even a dry run that passed must not unlock a half-updated install.
	d := Decide(fp, &DryRun{OK: true})
	assert.Equal(t, Inconsistent, d.Verdict)
	assert.False(t, d.Buildable())
	assert.NotEmpty(t, d.Reason)
}

func TestLookup_UnknownIsUnchecked(t *testing.T) {
	fp := synthetic()
	_, ok, half := Lookup(fp)
	assert.False(t, ok)
	assert.False(t, half)

	d := Decide(fp, nil)
	assert.Equal(t, Unchecked, d.Verdict)
	assert.False(t, d.Buildable())
	assert.Nil(t, d.Ledger)
}

func TestDecide_DryRunAllOKIsDerived(t *testing.T) {
	fp := synthetic()
	dry := &DryRun{Fingerprint: fp, OK: true, Tweaks: []TweakResult{{Name: "a", OK: true}, {Name: "b", OK: true}}}
	d := Decide(fp, dry)
	assert.Equal(t, Derived, d.Verdict)
	assert.True(t, d.Buildable())
	assert.Empty(t, d.Disabled)
	assert.Same(t, dry, d.DryRun)
}

func TestDecide_SomeTweaksFailIsDerivedWithDisabled(t *testing.T) {
	fp := synthetic()
	dry := &DryRun{Fingerprint: fp, OK: true, Tweaks: []TweakResult{
		{Name: "a", OK: true}, {Name: "b", OK: false, Problem: "x"}, {Name: "c", OK: false},
	}}
	d := Decide(fp, dry)
	assert.Equal(t, Derived, d.Verdict)
	assert.True(t, d.Buildable())
	assert.Equal(t, []string{"b", "c"}, d.Disabled)
}

func TestDecide_FailedDryRunIsUnsupported(t *testing.T) {
	fp := synthetic()
	d := Decide(fp, &DryRun{Fingerprint: fp, OK: false, Error: "emm exploded"})
	assert.Equal(t, Unsupported, d.Verdict)
	assert.False(t, d.Buildable())
	assert.Contains(t, d.Reason, "emm exploded")
}

func TestFingerprintID_SeparatesHotfixesOfOneRevision(t *testing.T) {
	a := synthetic()
	b := a
	b.ExeSHA256 = "ffffffffffff" + a.ExeSHA256[12:]
	assert.Equal(t, "1234567-abababababab", a.ID())
	assert.NotEqual(t, a.ID(), b.ID())
	assert.Equal(t, "1234567-abc", Fingerprint{Kfc: a.Kfc, ExeSHA256: "abc"}.ID(), "short hashes are not sliced past their end")
}

func TestSaveLoadDryRun_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "compat", "nested")
	fp := synthetic()
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	in := DryRun{Fingerprint: fp, EmmVersion: "emm 0.0.0", When: when, OK: true,
		Tweaks: []TweakResult{{Name: "a", Summary: "did 3", OK: true}, {Name: "b", Problem: "nothing"}}}
	require.NoError(t, SaveDryRun(dir, in))

	got, err := LoadDryRun(dir, fp)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, in, *got)

	other := fp
	other.ExeSHA256 = strings.Repeat("cd", 32)
	none, err := LoadDryRun(dir, other)
	require.NoError(t, err, "no dry run for a fingerprint is not an error")
	assert.Nil(t, none)
}

func TestLoadDryRun_CorruptIsAnError(t *testing.T) {
	dir := t.TempDir()
	fp := synthetic()
	require.NoError(t, os.WriteFile(filepath.Join(dir, fp.ID()+".json"), []byte("{"), 0o600))
	_, err := LoadDryRun(dir, fp)
	require.Error(t, err)
}

func TestRead_SyntheticInstall(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "enshrouded.exe")
	kfc := filepath.Join(dir, "enshrouded.kfc")
	commit := "0123456789abcdef0123456789abcdef01234567"
	require.NoError(t, os.WriteFile(exe, []byte("MZ\x00"+commit+"\x00\x000.0.1.0\x00"), 0o600))
	require.NoError(t, os.WriteFile(kfc, []byte("KFC\x00"+synthetic().Kfc.Raw+"\x00"), 0o600))

	fp, err := Read(exe, kfc, "42")
	require.NoError(t, err)
	assert.Equal(t, synthetic().Kfc, fp.Kfc)
	assert.Equal(t, commit, fp.ExeCommit)
	assert.Equal(t, "0.0.1.0", fp.ExeEngine)
	assert.Equal(t, "42", fp.BuildID)
	h, _ := gamefiles.HashFile(exe)
	assert.Equal(t, h, fp.ExeSHA256)

	require.NoError(t, os.WriteFile(kfc, []byte("no build string"), 0o600))
	_, err = Read(exe, kfc, "")
	require.ErrorIs(t, err, gamefiles.ErrNoVersion)
}
