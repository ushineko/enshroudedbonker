# Glider handling: higher lift limit

**Issue**: #31

## Context

The maintainer has Glider handling's `LIFT` and `UPDRAFT_START` at their maximum of 5 and
wants more of each. `UPDRAFT_START` scales `updraftStrengthChargeStartValue`, which spec
006 R4 caps at 1 on the assumption that it is a fraction of a full charge. Vanilla values
are 0.2–0.35, so a multiplier of 5 already reaches the cap on every glider, and raising the
maximum alone would change nothing.

The emm schema for build 1287369 has no other updraft strength in data:
`gliderUpstreamFactor` and `gliderUpstreamUpwardsFactor` are on `keen.ecs.DynamicLocomotion`,
which is runtime state, and `GliderUpstreamZone` has no fields.

## Requirements

- **R1.** `LIFT` maximum 10 (was 5).
- **R2.** `UPDRAFT_START` keeps its maximum of 5 and the cap at 1 (spec 006 R4). An
  experiment with the maximum at 10 and no cap showed no change in game past a full charge
  (AC4), so the cap now records a tested result rather than an assumption.
- **R3.** Defaults are unchanged, so a player who never moved these sliders gets the same
  build. The mod's version goes to 0.2.0.

## Acceptance Criteria

- [x] AC1: The CLI (`tweaks list`, `tweaks set`) and the window's Tweaks page offer `LIFT`
  up to 10. Both read the same `-- @param` header (`forms.SliderOptions` takes `p.Max`), so
  one change covers both. `tweaks list` shows `0.5..10.0`.
- [x] AC2: `make test` and `make lint` pass.
- [x] AC3 (in game, real install): with `LIFT` 10 the glide is flatter than with 5, with
  diminishing returns near 10, so no higher maximum is wanted. Maintainer, 2026-11-06.
- [x] AC4 (in game, real install): with `UPDRAFT_START` 10 and no cap, the updraft behaved
  as at 5: nothing changes past a full charge. The maximum and the cap were restored.
  Maintainer, 2026-11-06.

## Risks & Assumptions

- **Saved values stay valid.** Only maxima rise, so no saved value falls outside a range.
- **Rollback:** revert the commit; the previous mod is restored as is.

## E2E Test Plan

Environment: this machine, `make install` from the branch, the game installed through Steam.

1. `enshroudedbonker tweaks list` shows `0.5..10.0` for `LIFT`; the Tweaks page slider
   reaches 10. (AC1)
2. Set `LIFT` 10, press Apply mods, and glide from a high point. Compare with 5. (AC3)
3. Experiment, since reverted: `UPDRAFT_START` 10 with no cap, glide through an updraft.
   (AC4)

Results, 2026-11-06, build 1287369, reported by the maintainer: lift is flatter at 10 with
diminishing returns; updraft charge past 1 changes nothing.

### Prod visibility

Not applicable: a desktop tool with no telemetry.

## Executive Summary

Glider handling's lift maximum goes from 5 to 10. Raising the updraft charge maximum was
tried and dropped, because the game does nothing with a start value past a full charge.
The change is one line in the mod's parameter header.

## Status: COMPLETE
