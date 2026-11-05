package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/steam"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// Whether the player's mods are in the game (spec 010 R7).
const (
	ModsApplied         = "applied"
	ModsSettingsChanged = "settings_changed"
	ModsUpdateRemoved   = "update_removed"
	ModsNotApplied      = "not_applied"
	ModsModified        = "modified"
	ModsNeedsVerify     = "needs_verify"
	ModsNoneEnabled     = "none_enabled"
	ModsNoGame          = "no_game"
)

// VerifyURL asks Steam to verify the game's files.
const VerifyURL = "steam://validate/" + steam.AppID

// ModsInfo is the state and the sentence both front ends show for it.
type ModsInfo struct {
	State    string `json:"state"`
	Sentence string `json:"sentence"`
	// Skipped names enabled tweaks that do not work on this game build.
	Skipped []string `json:"skipped,omitempty"`
}

func modsInfo(s *session, res *StatusResult) ModsInfo {
	m := classifyMods(s, res)
	if res.Install.RunningKnown && res.Install.Running && m.State != ModsApplied {
		m.Sentence = "Quit Enshrouded to apply your mods."
	}
	return m
}

func classifyMods(s *session, res *StatusResult) ModsInfo {
	if res.Fingerprint == nil || res.Gate.Verdict == compat.Inconsistent ||
		(!res.Pristine.Present && len(res.Pristine.Patched) > 0) {
		return ModsInfo{State: ModsNeedsVerify, Sentence: "The game's files need restoring in Steam."}
	}
	wanted, skipped := withoutDisabled(res.Enabled, res.Gate)
	d := res.Deployed
	switch {
	case res.State == build.StateDeployed:
		if settingsChanged(s, d, wanted) {
			return ModsInfo{State: ModsSettingsChanged, Sentence: "Your mod settings changed since the last deploy.",
				Skipped: skipped}
		}
		return ModsInfo{State: ModsApplied, Sentence: appliedSentence(len(d.Mods), skipped), Skipped: skipped}
	case res.State == build.StateOther:
		return ModsInfo{State: ModsModified, Skipped: skipped,
			Sentence: "Another tool changed the game's archive. A copy is kept when you apply."}
	case len(res.Enabled) == 0:
		return ModsInfo{State: ModsNoneEnabled, Sentence: "No mods are enabled. Choose some under Tweaks or Mods."}
	case d != nil && d.Fingerprint.ID() != res.Fingerprint.ID():
		return ModsInfo{State: ModsUpdateRemoved, Sentence: "A game update removed your mods.", Skipped: skipped}
	}
	return ModsInfo{State: ModsNotApplied, Sentence: "Your mods are not in the game.", Skipped: skipped}
}

func appliedSentence(n int, skipped []string) string {
	if len(skipped) > 0 {
		verb := "does"
		if len(skipped) > 1 {
			verb = "do"
		}
		return fmt.Sprintf("%d of %d mods are in the game. %s %s not work on this game build.",
			n, n+len(skipped), strings.Join(skipped, ", "), verb)
	}
	if n == 1 {
		return "Your mod is in the game."
	}
	return "Your " + strconv.Itoa(n) + " mods are in the game."
}

// settingsChanged compares what was deployed with what the settings ask for. A
// record from before parameters were kept compares names only.
func settingsChanged(s *session, d *build.Deployed, wanted []string) bool {
	have := slices.Sorted(slices.Values(d.Mods))
	if !slices.Equal(have, slices.Sorted(slices.Values(wanted))) {
		return true
	}
	if d.Params == nil {
		return false
	}
	for _, n := range wanted {
		if !tweaks.Has(n) {
			continue
		}
		params, _ := knownParams(n, s.cfg.ParamsFor(n))
		if !maps.Equal(params, d.Params[n]) {
			return true
		}
	}
	return false
}

// ApplyRequest gets the enabled mods into the game.
type ApplyRequest struct{ Request }

// ApplyResult is what reached the game.
type ApplyResult struct {
	Deployed *build.Deployed `json:"deployed"`
	Manifest *build.Manifest `json:"manifest"`
	Warnings []string        `json:"warnings,omitempty"`
	// Skipped names enabled tweaks left out: they do not work on this game build.
	Skipped []string `json:"skipped,omitempty"`
}

/*
Apply does whatever is missing and deploys (spec 010 R1): emm, the vanilla copy,
the compatibility check, the build, the deploy. A compatibility check that failed
before runs again, since that is the one thing a press can still change.

Everything up to the deploy happens outside the game, so stopping anywhere before
it leaves the install as it was. The deploy itself is not cancellable: it writes
the index last, and a partial one is worse than a finished one.
*/
func Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	s, err := open(req.Request)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := s.requireInstall(); err != nil {
		return ApplyResult{}, err
	}
	if running, _ := s.running()(); running {
		return ApplyResult{}, build.ErrGameRunning
	}
	fp, err := s.fingerprint()
	if err != nil {
		return ApplyResult{}, err
	}
	if d := compat.Decide(fp, nil); d.Verdict == compat.Inconsistent {
		return ApplyResult{}, errors.New(d.Reason)
	}
	if len(s.cfg.EnabledNames()) == 0 {
		return ApplyResult{}, errors.New("no mods are enabled; enable some, or remove the mods to go back to vanilla")
	}

	ev := req.Events
	ev.progress(1, 4, "getting ready")
	if err := s.ensureReady(ctx); err != nil {
		return ApplyResult{}, err
	}
	if ctx.Err() != nil {
		return ApplyResult{}, fmt.Errorf("apply: %w", ctx.Err())
	}

	ev.progress(2, 4, "checking the game build")
	gate, err := s.gate()
	if err != nil {
		return ApplyResult{}, err
	}
	if gate.Verdict == compat.Unchecked || gate.Verdict == compat.Unsupported {
		ev.logf(LevelInfo, "checking every tweak against this game build")
		if _, err := CompatCheck(ctx, CompatCheckRequest(req)); err != nil {
			return ApplyResult{}, err
		}
	}

	built, err := s.build(ctx, false, true)
	if err != nil {
		return ApplyResult{Manifest: built.Manifest, Skipped: built.Skipped}, err
	}
	var notRun []string
	for _, m := range built.Manifest.Mods {
		if !m.Ran {
			notRun = append(notRun, m.Name)
		}
	}
	if len(notRun) > 0 {
		return ApplyResult{Manifest: built.Manifest, Skipped: built.Skipped},
			fmt.Errorf("emm did not run %s; nothing was written to the game", strings.Join(notRun, ", "))
	}
	if ctx.Err() != nil {
		return ApplyResult{Manifest: built.Manifest, Skipped: built.Skipped}, fmt.Errorf("apply: %w", ctx.Err())
	}

	ev.progress(4, 4, "writing into the game")
	dep, err := Deploy(ctx, DeployRequest(req))
	if err != nil {
		return ApplyResult{Manifest: built.Manifest, Skipped: built.Skipped}, err
	}
	return ApplyResult{Deployed: dep.Deployed, Manifest: built.Manifest, Warnings: built.Warnings,
		Skipped: built.Skipped}, nil
}

// getEmm is GetEmm, replaced in tests so they do not download anything.
var getEmm = GetEmm

// ensureReady gets emm and the vanilla copy if either is missing.
func (s *session) ensureReady(ctx context.Context) error {
	if _, _, err := s.emm(); errors.Is(err, emm.ErrNotInstalled) {
		if _, err := getEmm(ctx, GetEmmRequest{Request: s.req}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	_, err := s.pristine()
	if !errors.Is(err, build.ErrNoPristine) {
		return err
	}
	s.req.Events.logf(LevelInfo, "keeping a vanilla copy of this game build")
	if _, err := PristineCapture(ctx, PristineCaptureRequest{Request: s.req}); err != nil {
		if errors.Is(err, build.ErrLooksPatched) {
			return fmt.Errorf("the game's files need restoring: run Steam's Verify integrity of game files, "+
				"then apply again (%w)", err)
		}
		return err
	}
	return nil
}
