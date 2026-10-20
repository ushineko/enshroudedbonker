/*
Package compat is the schema gate (spec 002 R2): which game build is installed,
whether this tool has been shown to work against it, and the verdict a build is
gated on.

The decision is made from three sources, in order of authority:

 1. the ledger, a committed table of builds that were checked in game;
 2. a dry run, recorded per fingerprint in the workspace: every built-in tweak
    applied by emm to a scratch copy of the archive, each reporting what it
    changed;
 3. nothing, which is "unchecked" and refuses to build until a dry run says
    otherwise.

An archive whose build the ledger knows, paired with an executable the ledger
does not, is an install mid-update or half-restored, and is refused.
*/
package compat

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/gamefiles"
)

// Verdicts.
const (
	Verified     = "verified"
	Derived      = "derived"
	Unchecked    = "unchecked"
	Unsupported  = "unsupported"
	Inconsistent = "inconsistent"
)

/*
Fingerprint identifies one installed build (R2.1): the archive's build string,
the executable's source commit and hash, and Steam's build id.
*/
type Fingerprint struct {
	Kfc       gamefiles.Version `json:"kfc_version"`
	ExeCommit string            `json:"exe_commit"`
	ExeEngine string            `json:"exe_engine,omitempty"`
	ExeSHA256 string            `json:"exe_sha256"`
	BuildID   string            `json:"steam_build_id,omitempty"`
}

// ID names the fingerprint on disk: the archive's revision and a prefix of the
// executable's hash, so two builds of one revision (a hotfix) are told apart.
func (f Fingerprint) ID() string {
	h := f.ExeSHA256
	if len(h) > 12 {
		h = h[:12]
	}
	return f.Kfc.Revision + "-" + h
}

// Short is the build as a status line shows it.
func (f Fingerprint) Short() string { return f.Kfc.Short() }

// Read takes the fingerprint of an install.
func Read(exe, kfc, buildID string) (Fingerprint, error) {
	fp := Fingerprint{BuildID: buildID}
	var err error
	// The archive index carries the string in its header; 1 MiB is far more
	// than enough and keeps a corrupt file from being slurped.
	if fp.Kfc, err = gamefiles.ReadVersion(kfc, 1<<20); err != nil {
		return fp, err
	}
	if fp.ExeCommit, fp.ExeEngine, err = gamefiles.ReadCommit(exe); err != nil {
		return fp, err
	}
	if fp.ExeSHA256, err = gamefiles.HashFile(exe); err != nil {
		return fp, err
	}
	return fp, nil
}

// LedgerEntry is one build checked in game.
type LedgerEntry struct {
	Version   string `json:"version"`
	ExeSHA256 string `json:"exe_sha256"`
	EmmTag    string `json:"emm_tag"`
	Verified  string `json:"verified"`
	Notes     string `json:"notes"`
	// Disabled lists built-in tweaks known not to work on this build.
	Disabled []string `json:"disabled,omitempty"`
}

//go:embed ledger.json
var ledgerJSON []byte

// Ledger is the committed table.
func Ledger() []LedgerEntry {
	var doc struct {
		Entries []LedgerEntry `json:"entries"`
	}
	if err := json.Unmarshal(ledgerJSON, &doc); err != nil {
		panic("compat: embedded ledger.json does not parse: " + err.Error())
	}
	return doc.Entries
}

// Lookup finds the ledger entry for a fingerprint. half reports an entry whose
// archive build matches but whose executable does not: an install part-way
// through an update.
func Lookup(fp Fingerprint) (entry LedgerEntry, ok, half bool) {
	for _, e := range Ledger() {
		if e.Version != fp.Kfc.Raw {
			continue
		}
		if e.ExeSHA256 == fp.ExeSHA256 {
			return e, true, false
		}
		half = true
	}
	return LedgerEntry{}, false, half
}

// TweakResult is what one built-in reported in a dry run.
type TweakResult struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	OK      bool   `json:"ok"`
	Problem string `json:"problem,omitempty"`
}

// DryRun is a recorded dry run (R2.4).
type DryRun struct {
	Fingerprint Fingerprint   `json:"fingerprint"`
	EmmVersion  string        `json:"emm_version"`
	When        time.Time     `json:"when"`
	OK          bool          `json:"ok"`
	Error       string        `json:"error,omitempty"`
	Tweaks      []TweakResult `json:"tweaks"`
}

func dryRunPath(dir string, fp Fingerprint) string {
	return filepath.Join(dir, fp.ID()+".json")
}

// SaveDryRun records a dry run.
func SaveDryRun(dir string, d DryRun) error {
	if err := config.MkdirAll(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dry run: %w", err)
	}
	if err := os.WriteFile(dryRunPath(dir, d.Fingerprint), b, 0o600); err != nil {
		return fmt.Errorf("write dry run: %w", err)
	}
	return nil
}

// LoadDryRun reads the recorded dry run for a fingerprint, if any.
func LoadDryRun(dir string, fp Fingerprint) (*DryRun, error) {
	b, err := os.ReadFile(dryRunPath(dir, fp))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read dry run: %w", err)
	}
	var d DryRun
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse dry run: %w", err)
	}
	return &d, nil
}

// Decision is the gate's answer.
type Decision struct {
	Verdict string `json:"verdict"`
	// Reason is one sentence a front end can show as is.
	Reason string `json:"reason"`
	// Ledger is the matching entry, when there is one.
	Ledger *LedgerEntry `json:"ledger,omitempty"`
	// DryRun is the recorded dry run, when there is one.
	DryRun *DryRun `json:"dry_run,omitempty"`
	// Disabled lists built-ins that must not be built on this install.
	Disabled []string `json:"disabled,omitempty"`
}

// Buildable reports whether the gate lets a build through.
func (d Decision) Buildable() bool { return d.Verdict == Verified || d.Verdict == Derived }

// Decide combines the sources.
func Decide(fp Fingerprint, dry *DryRun) Decision {
	e, ok, half := Lookup(fp)
	if half {
		return Decision{Verdict: Inconsistent,
			Reason: fmt.Sprintf("the archive is build %s but the executable is not the one that build "+
				"shipped with; the install is mid-update or partly restored. Let Steam finish, or "+
				"verify the game files.", fp.Short())}
	}
	d := Decision{DryRun: dry}
	if ok {
		d.Verdict, d.Ledger, d.Disabled = Verified, &e, e.Disabled
		d.Reason = "build " + fp.Short() + " was checked in game on " + e.Verified + "."
		return d
	}
	switch {
	case dry == nil:
		d.Verdict = Unchecked
		d.Reason = "build " + fp.Short() + " is not in the ledger and has not been dry-run yet."
	case !dry.OK:
		d.Verdict = Unsupported
		d.Reason = "the dry run against build " + fp.Short() + " failed: " + dry.Error
	default:
		d.Verdict = Derived
		d.Reason = "build " + fp.Short() + " is not in the ledger, but every built-in " +
			"applied in a dry run. Check the result in game."
		for _, t := range dry.Tweaks {
			if !t.OK {
				d.Disabled = append(d.Disabled, t.Name)
			}
		}
	}
	return d
}
