# Spec 006 — Glider handling

**Status**: COMPLETE
**Issue**: #11

## Context

Gliding heads downward more steeply than the player wants. The issue asks for a
flatter glide that goes further, with the other handling values adjustable for
tuning in game.

Recon, 2026-10-23, game build 1273043 (`release_1.0`), by probe mods run with
`emm` against a copy of the vanilla archive:

- Each glider is an `ItemInfo` whose `equipment.slot` is `"Glider"`; build 1273043
  has 4. Each carries its own `equipment.gliderConfig` (`keen.GliderConfig`):
  `accelerationForward`, `airResistanceLongitudinal`, `airResistanceLateral`,
  `airResistanceVertical`, `yawAngleSpeed`, `pitchAngleSpeed`, `rollAngleSpeed`,
  `updraftStrengthChargeStartValue`. Every other item carries the same default
  config, which no glider uses.
- A throwaway library mod that multiplied `airResistanceVertical` by 2.5 on the four
  gliders was tried in game: the glide was flatter and went a bit further.
- Gliding has no per-glider stamina cost in the data, and updraft columns
  (`GliderUpstreamZone`) carry no strength; neither is in scope.

## Requirements

- **R1.** A built-in tweak `glider_handling` (group World) with one float
  multiplier for each `GliderConfig` field: `LIFT` (vertical air resistance),
  `DRAG` (longitudinal), `SIDE_DRAG` (lateral), `THRUST` (forward acceleration),
  `YAW`, `PITCH`, `ROLL` and `UPDRAFT_START`.
- **R2.** Defaults: `LIFT` 2.5, the value tried in game; every other multiplier 1, so
  only lift changes until the player turns the others.
- **R3.** Multipliers scale each glider's own value, so the gliders keep their
  order relative to one another.
- **R4.** `updraftStrengthChargeStartValue` is capped at 1 after scaling (vanilla
  values 0.2–0.35 read as a charge fraction).
- **R5.** Only items whose `equipment.slot` is `"Glider"` are touched.
- **R6.** README tweak table lists it.

## Acceptance Criteria

- [x] AC1: the tweak parses, every parameter rewrites exactly one line, and the
  repository tests pass (`go test ./...`; the per-tweak tests cover it).
- [x] AC2 (integration, real archive): a build at defaults reports
  `glider_handling: lift x2.5, drag x1 on 4 gliders`.
- [x] AC3 (e2e, in game): at defaults the glide is flatter than vanilla.
- [x] AC4 (e2e, in game): changing `DRAG` or a turn rate and rebuilding changes
  how the glider flies in the expected direction. AC3 and AC4 confirmed by the
  maintainer, 2026-10-23.

## Risks & Assumptions

- **Rollback**: disable the tweak and rebuild, or revert the commit.
- **Field meanings** are read from their names, confirmed only for `LIFT` (AC3).
  The others are exposed for the player to tune, with defaults that change nothing.
- **`UPDRAFT_START` as a fraction** is an assumption from the vanilla values; the
  cap keeps a wrong guess from producing values far out of range.
- **Multiplayer**: the glider config is read by whoever simulates the player;
  joining an unpatched server may ignore it (same as the other tweaks).

## E2E Test Plan

Environment: the maintainer's Linux/Proton install, single player.

1. Install from the branch (`make install`), disable `glider_experiment`, enable
   `glider_handling`, build and deploy. `enshroudedbonker report` shows the AC2 line.
2. In game, glide from a high point and compare the descent with vanilla (AC3).
3. Change `DRAG` (for example to 0.7) or `YAW` (to 1.5) with `tweaks set`, rebuild,
   redeploy and glide again (AC4).

### Prod visibility

Not applicable: a desktop tool with no server side. The build report line from
step 1 is the observable record.
