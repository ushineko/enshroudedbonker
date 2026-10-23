# Spec 005 — Fast travel to every point of interest

**Status**: INCOMPLETE
**Issue**: #9

## Context

With `fast_travel_markers` enabled, the map offers fast travel to Flame Altars,
Ancient Spires, settlements and camps, and to almost nothing else. The issue asks
for fast travel to every point of interest.

Recon, 2026-10-23, game build 1273043 (`release_1.0`), by probe mods run with
`emm` against a copy of the vanilla archive:

- The map offers fast travel to a marker exactly when its type's
  `MapMarkerRegistryResource.mapMarkers[].isFastTravelDestination` is set. There
  are 49 marker types: 6 are destinations in vanilla, and the tweak adds 13 (the
  `Dungeons` and `Locations` sorting categories). The other 30 were never touched.
- The label entities behind the 13 added types have the same components as the
  vanilla Ancient Spire markers (`MapMarker_Tower_*_FastTravel`). No spawn point
  or other component decides whether travel is possible.
- A throwaway library mod that set the flag on 31 types was tried in game, and the
  points of interest that had no travel option gained one. The guess that a
  code-side location check (Shroud, underground) was blocking them was wrong.

The 30 remaining types, by sorting category:

| Category | Types | What uses them |
| --- | --- | --- |
| `FlameRelated` | 5 | flame shrines and temples, ancient obelisks, gem forges, night temples; water dispensers (`*_PLAYER` templates, built by the player) |
| `None` | 15 | town districts, area and lake names, biome entries (shown as text labels); quest search areas (shown as a range); tower labels; teleport platforms, crypt save points and some boss/border/VFX entities in one type with a size-0, hidden icon; the player's gravestone; test markers |
| `CustomMarker` | 10 | markers the player places |

## Requirements

- **R1.** Two new switches on `fast_travel_markers`, both on by default:
  - `FLAME_SITES`: every `FlameRelated` type, except a type that only player-built
    templates (name ending `_PLAYER`) use.
  - `AREAS`: every `None` type that has `showLabel` set and `showRange` clear.
- **R2.** `DUNGEONS` and `LOCATIONS` keep their meaning and defaults.
- **R3.** Never flagged: custom marker types; range markers (quest search areas);
  the teleporter type, the gravestone and the other unlabelled `None` types.
  Teleporters are left out because their one type also covers a boss, the
  playable-area border and VFX entities, and its icon is hidden, so it is unclear
  that the map can offer travel to it at all.
- **R4.** The selection uses only categories, registry fields and template-name
  patterns. No marker id, GUID or other game-derived value is committed
  (public-repository rule).
- **R5.** The tweak's description, `mod.json` and the README's tweak table say what
  it now covers.

## Acceptance Criteria

- [x] AC1: `fast_travel_markers` has `DUNGEONS`, `LOCATIONS`, `FLAME_SITES` and
  `AREAS`, all bool, default on (`go test ./internal/tweaks`, `TestBoolParamsFormatTrueFalse`).
- [x] AC2 (integration, real archive): run through `emm` against a copy of build
  1273043's vanilla archive at defaults, the mod reports `enabled 21 marker types`.
  Those are the 6 Dungeons, 7 Locations, 4 flame-site and 4 named-area types; no
  custom, range, teleporter, gravestone or water-dispenser type.
- [x] AC3: no marker id or GUID appears in the committed mod or this spec.
- [x] AC4: README tweak table and `mod.json` description updated.
- [x] AC5 (e2e, in game): with the tweak at defaults, the map offers fast travel
  to a flame shrine, an ancient obelisk, a gem forge and a named area or town, and
  travel to one of them completes. Done 2026-10-23: the maintainer checked a mix of
  destinations on the deployed build (report: `enabled 21 marker types`); all worked.
- [ ] AC6 (e2e, in game): with `FLAME_SITES` and `AREAS` off, those markers have
  no travel option and dungeons and locations still do.

## Risks & Assumptions

- **Rollback**: revert the commit, or switch `FLAME_SITES`/`AREAS` off and rebuild.
  Deploy and rollback are unchanged.
- **Off-map landings and gravestones**: more destinations means more chances of
  the side effects the tweak already warns about. Upstream game behaviour,
  documented rather than mitigated (spec 002).
- **Game updates** may add or recategorise marker types. The rules select by
  category and fields, so new types in the same categories are picked up. The
  count in AC2 is for build 1273043 only.
- **`AREAS` is the least tested group.** The in-game trial covered several POIs,
  but not every named-area type by name; AC5 asks for one.

## E2E Test Plan

Environment: the maintainer's Linux/Proton install, single player.

1. Quit the game. `enshroudedbonker mods disable ft_all_experiment` (the recon
   library mod), `mods enable fast_travel_markers`, `build`, `deploy`.
   `enshroudedbonker report` shows `fast_travel_markers: enabled 21 marker types`
   (AC2 on the real install).
2. In game, open the map and check the travel option on a flame shrine, an
   obelisk, a gem forge and a town or area label; travel to one (AC5).
3. Quit, `tweaks set fast_travel_markers FLAME_SITES false`, the same for `AREAS`,
   then build and deploy. The markers from step 2 have no travel option, and a
   dungeon still does (AC6). Set both back on.
4. `mods remove ft_all_experiment`.

### Prod visibility

Not applicable: a desktop tool with no server side. The build report line from
step 1 is the observable record.
