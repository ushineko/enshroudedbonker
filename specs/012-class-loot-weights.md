# Spec 012 — Class weights for random gear

**Status**: COMPLETE
**Issue**: #33

## Executive Summary

A new built-in tweak, `class_loot_weights`, takes a share of each class's gear out
of random loot by marking entries non-gear, keeping at least one per kind. The
loot data has no player-class input; the weapon pool is about half warrior gear,
which explains the reports. Review the keep-rule in `mod.lua` first, and the
assumption it rests on (see Risks), which the in-game check confirmed.

## Context

Players report that random gear seldom matches the class they play: a mage
finds a usable staff or wand about once every ten levels, while bows, swords and
daggers drop constantly. A theory online holds that the game withholds drops for
the class being played.

Recon, 2026-11-12, game build 1287369 (`release_1.0`), by probe mods run with
`emm` against a copy of the vanilla archive:

- Random gear comes from `LootableItemsResource`. A drop first picks a slot by
  `shufflebagCategoryWeights` (weapon 62, armor 24, ring 8, shield 6; three
  label overrides use 77.6/8.4/8/6), then an item of that `shufflebagType`.
- Each gear entry carries `classAlignmentType` (Warrior, Ranger, Mage or None).
  Weapons: 152 Warrior (one-handed 78, two-handed 43, greatsword 31), 65 Ranger
  (dagger 33, bow 32), 59 Mage (wand 32, staff 27). Armor holds about 19–20 per
  class per slot. Rings are 8/10/12 and shields 19/0/7 (Warrior/Ranger/Mage). A
  handful of entries in each slot have no class. No weapon entry has loot labels,
  and every weapon's level range is 1–55, so neither labels nor level gate the
  draw.
- Class enters loot only through `LootModifierInfo.classAlignment`. Of about 785
  loot settings found, 12 `DefaultLootLabelCollection` entries set it: a weapon
  set and an armor set for each class. All the others are None. The skill tree's
  class enum (`keen::ClassType`) is a separate type from the loot one
  (`keen::ItemClassAlignmentType`), and no data links them. The data has no
  setting for the player's class. The runtime code is unread (it is encrypted on
  disk).
- The data has no per-item weight. If items are drawn evenly within a slot, a
  staff or wand is about 13% of all random gear, and the warrior skew of the pool
  explains the reports without any class targeting.
- The pool holds no duplicate item ids. An entry whose `shufflebagType` is None
  and that has no labels does not roll as random loot. Spec 009 saw this with
  the rune.

## Requirements

- **R1.** A built-in tweak `class_loot_weights` (group Loot), with one parameter
  per class: `WARRIOR_WEIGHT`, `RANGER_WEIGHT` and `MAGE_WEIGHT` (float, 0–1, step
  0.05). Defaults are 0.4, 1.0 and 1.0, which brings the three classes' weapon
  counts roughly level (60/65/59).
- **R2.** For each class whose weight is below 1, the tweak keeps
  `round(n × weight)` entries, at least one, from every group of random gear
  with the same slot, weapon category, armor slot and class. It takes the
  removed entries out of random loot by setting their `shufflebagType` to None.
  The entries are not deleted, so lookups by id elsewhere still work. The kept
  entries are spread evenly through the group, so a whole tier is not dropped.
- **R3.** At least one entry stays in every group, so the 12 class-locked
  loot settings and the shield slot never draw from an empty pool.
- **R4.** Entries with no class, non-gear entries and the slot weights are left
  as they are.
- **R5.** The build line reports the weights, the weapon counts per class before
  and after, and the number of entries taken out of random loot.
- **R6.** The README tweak table lists the tweak. Its description says that it
  thins the pool (pulled items stop dropping at random), that it cannot boost a
  class, and that it applies to armor, rings and shields as well as weapons.

## Acceptance Criteria

- [x] AC1: the tweak parses, every parameter rewrites exactly one line, and the
  repository tests pass (`make test`, `make lint`; the per-tweak tests cover it).
- [x] AC2 (integration, real archive): a build with the defaults runs under `emm`
  and reports weapons `152/65/59 -> 60/65/59` (Warrior/Ranger/Mage).
- [x] AC3 (integration, real archive): with weights 0/0/1, a build reports one
  entry kept per Warrior and Ranger group (weapons `152/65/59 -> 3/2/59`).
- [x] AC4 (e2e, in game): with weights 0/0/1, on a throwaway world, most random
  weapons from chests and enemies are staves and wands. Confirmed by the
  maintainer, 2026-11-12: with the tweak deployed at 0/0/1 alongside the other
  built-ins, random weapons were mostly mage weapons. Other classes' weapons
  still turn up, which fits the one entry kept per group and the fixed drop
  lists the tweak does not touch.

## Risks & Assumptions

- **Assumption**: the game draws a random gear item only from entries whose
  `shufflebagType` matches the slot it picked. AC4 tests this. If the game reads
  `classAlignmentType` or the category masks some other way, the tweak may do
  nothing, and AC4 shows that.
- **Assumption**: items are drawn evenly within a slot. If the game draws by
  weapon category first, thinning within a category changes nothing until a
  category is reduced to its last entry, and the effective weights are coarser
  than the sliders. AC4 still shows the direction.
- **Pulled items**: weapons taken out of the pool never drop at random. Fixed
  drop lists, vendors and crafting are unaffected.
- **Rollback**: disable the tweak and rebuild, or revert the commit. The save
  holds nothing it changes.
- **Overlap**: `better_loot` writes `globalRarityDropRates` in the same resource.
  The fields are disjoint, so emm's run order does not matter.

## E2E Test Plan

Environment: the maintainer's Linux/Proton install, single player, a throwaway
world (or a backed-up save).

1. Install from the branch (`make install`), enable `class_loot_weights` with
   weights 0/0/1, build and deploy. `enshroudedbonker report` shows the AC3 line.
2. In game, open about ten random chests and loot enemies of the same level.
   Note the weapons: most should be staves and wands (AC4).
3. Disable the tweak, rebuild and deploy, and confirm the vanilla install comes
   back.

### Prod visibility

Not applicable: a desktop tool with no server side. The build report line from
step 1 is the observable record.
