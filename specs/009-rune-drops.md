# Spec 009 — Runes from enemy corpses

**Status**: COMPLETE
**Issue**: #22

## Context

Runes, the currency of vendor upgrades, come from chests, salvaging and a few
enemies. Salvage cannot be raised without raising upgrade prices (#20), so the
issue asks for runes from enemy kills at a configurable amount.

Recon, 2026-10-23, game build 1273043 (`release_1.0`), by probe mods run with
`emm` against a copy of the vanilla archive:

- Enemy templates (`Enemy_*`) carry random loot: a slot count and loot labels
  that draw from the global pool (`LootableItemsResource`). The rune is in that
  pool with no labels, so it never rolls.
- A throwaway mod gave every enemy template with random loot a fixed drop list
  (`ecs::DefaultInventory`) holding only runes. In game nothing dropped:
  monsters do not hand out a fixed list on death.
- What an enemy leaves to loot is a separate template, `LootContainer_Enemy_*`
  (`DestroyOnLoot`, an inventory, an interaction offer). There are 84, each with a
  fixed drop list (61 distinct lists, some shared), and every list's root group is
  plain rather than a selector, so a stack added there drops alongside the rest.
  They cover humanoids and wildlife; 26 already hold runes (1–3 to 6–10 per slot).
- A second throwaway mod added 20–30 runes at chance 1 to those 61 lists. In game a
  skeleton dropped 67 runes with the world's loot-amount setting raised: the
  added stack is scaled by that setting like any other drop.
- The rune item is `BalancingTable.perkCostItem`, the one vendor upgrades charge,
  so the mod finds it at run time and commits no item id.

## Requirements

- **R1.** A built-in tweak `rune_drops` (group Loot) that appends one rune stack
  to the fixed drop list of every `LootContainer_Enemy_*` template, once per
  distinct list.
- **R2.** Parameters: `RUNES_MIN` and `RUNES_MAX` (int, 1–100, defaults 2 and 5)
  and `CHANCE` (float, 0.05–1, step 0.05, default 1). A minimum above the maximum
  is taken as both being the maximum's value rather than an error.
- **R3.** The rune is `BalancingTable.perkCostItem`, resolved at run time.
- **R4.** Existing stacks, including the vanilla rune stacks, are not changed.
- **R5.** The build line reports the amount, the chance and how many lists and
  corpses were changed.
- **R6.** The README tweak table lists it, and its description says the world's
  loot-amount setting scales the amount.

## Acceptance Criteria

- [x] AC1: the tweak parses, every parameter rewrites exactly one line, and the
  repository tests pass (`make test`, `make lint`; the per-tweak tests cover it).
- [x] AC2 (integration, real archive): a build at defaults reports
  `rune_drops: 2-5 runes at 1.0 added to 61 drop lists (84 enemy corpses)`.
- [x] AC3 (e2e, in game): an enemy corpse drops the added runes, alongside its
  usual loot. Confirmed by the maintainer, 2026-10-23, with the second throwaway
  mod (the same change at 20–30 runes): a skeleton dropped 67 runes and a banshee
  a single stack of 57, with the world's loot-amount setting raised. The
  banshee's vanilla list holds T2 runes in stacks of 3–6, so a stack of 57 is the
  added one. The tweak at defaults is the same change with a smaller amount.

## Risks & Assumptions

- **Rollback**: disable the tweak and rebuild, or revert the commit. Runes already
  picked up stay in the save.
- **Coverage**: only enemies that leave a lootable corpse. Enemies whose loot comes
  only from random rolls are not covered; reaching them means giving the rune pool
  entry loot labels, a separate change.
- **Scaling**: the amount is scaled by the world's loot-amount setting (seen in
  game), and possibly by other loot scaling not yet observed. The parameter is a
  base amount.
- **Shared lists**: a list shared by several corpse templates gets one stack, so
  each of those corpses drops it once.
- **Overlap**: `more_loot`, `chest_slots` and other tweaks also write
  `TemplateResource`, and `better_loot` writes `DefaultInventoryResource` stack
  chances. This tweak only reads templates and appends a stack with chance 1 by
  default. `better_loot` raises only chances below 1, so with a lower `CHANCE`
  the two compound depending on emm's order. The build reports the overlap.

## E2E Test Plan

Environment: the maintainer's Linux/Proton install, single player.

1. Install from the branch (`make install`), remove `rune_corpse_experiment`,
   enable `rune_drops`, build and deploy. `enshroudedbonker report` shows the AC2
   line.
2. In game, kill a skeleton warrior or mage, or an animal, and open its corpse:
   runes are there with its usual loot (AC3).

### Prod visibility

Not applicable: a desktop tool with no server side. The build report line from
step 1 is the observable record.
