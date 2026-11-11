# Spec 010 — More backpack slots per tier

**Status**: SHELVED — the backpack grid does not scroll, and lowering a template breaks owned backpacks (see Decision)
**Issue**: #26

## Context

Each backpack tier adds 8 inventory slots. Chests can be enlarged
(`chest_slots`), the backpack cannot. The issue asks for a flat number of extra
slots at each tier, so every backpack upgrade is worth more.

Recon, 2026-10-29, game build 1273043 (`release_1.0`), by probe mods run with
`emm` against a copy of the vanilla archive:

- Backpacks are `ItemInfo` resources with `equipment.slot == "Backpack"` and the
  flags `IsContainer` and `HasPIDE`. There are seven: `Backpack_T0`,
  `Backpack_T1_NEW`, `Backpack_T2_NEW`, `Backpack_T3_NEW`, and an older series
  `Backpack_T1`, `Backpack_T2`, `Backpack_T3`.
- None has perks, impact values or an equipment program. The slot count lives
  in a `TemplateResource` of the same name as the item's `debugName`, whose
  `keen::ecs::InventorySetup` holds `genericSlotCount` and `availableSlotCount`
  (equal): T0 8, T1_NEW 16, T2_NEW 24, T3_NEW 32; old T1 8, T2 16, T3 24. The
  `_NEW` series with T0 is the 8-per-tier ladder players see; the old series is
  presumably left over from an earlier version, though characters may still hold
  one.
- The item's `pidEntity` GUID did not match the template's `objectId` in the
  probe, so items and templates are paired by name.
- Each backpack item carries exactly one `uiValues` entry, whose value equals the
  template's slot count. It is the tooltip number and is display-only.
- The player template (`1_Player`) has its own `InventorySetup` of 40 slots.
  What that 40 means (body inventory, a cap on backpack slots, or neither) is not
  known yet.

## Requirements

- **R1.** A built-in tweak `backpack_slots` (group Inventory) that sets each
  backpack template's slot count to its vanilla count plus `EXTRA_PER_TIER × (N + 1)`,
  where N is the tier digit in the template name (`Backpack_T<N>…`). At the
  default this gives T0 16, T1 32, T2 48, T3 64: each upgrade grants
  `8 + EXTRA_PER_TIER` slots instead of 8.
- **R2.** Parameters: `EXTRA_PER_TIER` (int, 1–32, step 1, default 8) and
  `SLOT_CAP` (int, 32–255, step 8, default 128). The result is capped at `SLOT_CAP`
  (the field is a u8).
- **R3.** `genericSlotCount` and `availableSlotCount` are both written, as
  `chest_slots` does.
- **R4.** The backpack item's tooltip value is set to the same count. The entry
  is found as the item's `uiValues` entry whose value equals the vanilla slot
  count, so no loca id is committed. An item with no such entry is skipped and
  counted in the report.
- **R5.** Both series are changed, by the same formula.
- **R6.** The build line reports the bonus and how many backpacks and tooltips
  were changed.
- **R7.** The README tweak table lists it. Its description says whether an owned
  backpack changes or has to be crafted again, and what removing the tweak does
  to items in the extra slots, as settled by the E2E plan.

## Acceptance Criteria

- [x] AC1: the tweak parses, every parameter rewrites exactly one line, and the
  repository tests pass (`make test`, `make lint`; the per-tweak tests cover it).
- [x] AC2 (integration, real archive): a build at defaults reports
  `backpack_slots: +8 per tier on 7 backpacks (7 tooltips)`. Run 2026-10-29
  with `emm` against a copy of the vanilla archive: that line, 14 patches
  applied. A probe reading the patched archive back found the `_NEW` series at
  16/32/48/64 slots and the old series at 24/40/56, tooltips equal.
- [ ] AC3 (e2e, in game): a T1 backpack shows 32 slots in the inventory screen
  and in its tooltip, and items can be put in all of them. Whether the backpack
  already worn changes or a newly crafted one is needed is recorded here. Observed
  2026-10-29: the T1 backpack already worn showed 32 slots, no recraft needed.
  Every slot was not filled; an item was placed in the last slot.
- [ ] AC4 (e2e, in game): with the tweak disabled and the build redeployed, the
  effect on items in the extra slots is observed and recorded here, and the
  tweak's description says it. **Failed 2026-10-29**: see In-game results.

## In-game results (2026-10-29)

Maintainer's main character, single player, second-tier backpack
(`Backpack_T1_NEW`, vanilla 16), deployed with the other twelve tweaks. Counts
are of the inventory grid, 8 columns wide.

**Vanilla, after restoring the character file**: 24 slots with no backpack (the
player's own inventory), 40 with it equipped. The backpack adds its template
count on top of the player's 24. Restoring `character0.ksd` alone fully resets
the state; the world save was not restored.

**Test 1, `EXTRA_PER_TIER = 8` (template 16 → 32)**

1. Tweak on: the backpack already worn grew; the tooltip said 32. An item went
   into the last slot; save and exit.
2. Tweak off (template 32 → 16): the grid grew to at least 8×11 (88), running
   off the bottom of the screen and hiding the hotbar. The item kept its
   relative position.
3. Tweak on again: still 88 or more. Character restored from the game's own
   save rotation (00:02 that day).

**Test 2, `EXTRA_PER_TIER = 1` (template 16 → 18)**

1. Tweak on: the grid read as 6 rows ("48"), tooltip 18. 24 + 18 = 42 is 5 full
   rows and 2 slots in a sixth, so this is most likely 42: an increase gives the
   player's 24 plus the new template. Not confirmed by counting the last row.
   Re-equipping gave the same; reloading with nothing changed gave the same.
2. Tweak off (template 18 → 16): at least 8×11 (88) again, off screen.
   Un-equipping removed the extra rows and left the player's 24.
3. Character restored from the same 00:02 file: 24 / 40, vanilla.

**Test 3, decreases with and without a row change** (save bytes read after
each step)

1. `EXTRA_PER_TIER = 2` (template 20): 48 slots, all usable; tooltip 20; byte
   `03`. The game rounds a backpack up to whole rows.
2. `EXTRA_PER_TIER = 1` (template 18, same 3 rows): 48 slots; tooltip 18; byte
   `03`. A decrease that keeps the row count is harmless.
3. `EXTRA_PER_TIER = 5` (template 26): 56 slots (7 rows); tooltip 26; byte `04`.
   The UI shrinks slightly to fit the 7th row; an 8th pushes the grid off
   screen. 56 is the most a usable inventory can hold at that resolution.
4. `EXTRA_PER_TIER = 1` (template 18, 4 rows → 3): off screen again. A newly
   crafted backpack of the same type showed 48 and its tooltip 18. Swapping
   between the two in that session, the old one stayed oversized.
5. After a save, quit and reload with the new backpack equipped, the old one,
   re-equipped, showed 48, and the save read `03` with it equipped.

The save from step 4 before the new backpack was crafted had rotated out of the
game's backups, so the value the decrease wrote (16 or 18) was not read.

**Conclusion**: raising a backpack template behaves as intended and is stable
across reloads and re-equips. Lowering it, by any amount, leaves an equipped
backpack with far more slots than either value and pushes the hotbar off
screen. The save explains why (next section).

## Save-format findings (2026-10-29)

Decoded from the maintainer's character saves (`character0.ksd` and the game's
own backup rotation), read-only copies, nothing written back. Layout as far as
it was needed:

- **File**: 48-byte header (`SV2C`, version 2, total file size, a GUID, a
  timestamp, chunk count 5; other fields not identified, possibly a checksum),
  then 5 chunks.
- **Chunk**: `S2BH`, u32 version (2), u8 type, u8 compressed flag, u32 count,
  then the body: one zstd frame when compressed, raw otherwise. Types seen:
  0x0a character header (name, GUID, timestamp), 0x09 inventory, 0x14 (raw, 388
  bytes), 0x0f (count 100), 0x10 `FGOW`, about 1.6 MB decoded, the fog-of-war
  map.
- **Inventory (chunk 0x09)**: records `[slot u8][itemId u32][amount u16][type
  u8][data]`. Data by type: 0x00 none; 0x11 three bytes (level and two more);
  0x13 eleven (level, two u32 durability values, two bytes); 0x04 eight (water
  containers); flag 0x20 adds five (a socketed gem's itemId and level).
  Equipment uses slots 0x00–0x0a, the backpack being 0x0a. The bag list follows,
  with its own slot numbers, then the material selections.
- **Backpack rows**: between the equipped backpack record and the first bag
  record is an 8-byte header. Its seventh byte is the backpack's size in rows
  of 8 slots. The grid shows the player's 24 slots plus 8 × that byte:

| Save | Template | Byte | Grid |
|---|---|---|---|
| Every save before the tweak (back to 10-27) | 16 | `02` | 24 + 16 = 40 |
| Test 2, tweak on | 18 | `03` | 24 + 24 = 48 |
| After each decrease (32 → 16, 18 → 16) | 16 | `10` (16) | 24 + 128 = 152 |

  Only the equipped backpack's rows are saved: a backpack in the bag is a
  plain type-0x00 record with no size, and is rebuilt from its template on
  load. On an increase the game stores rows (18 or 20 slots → 3, 26 → 4). A
  decrease that keeps the row count changes nothing. A decrease that drops a row
  stores a far larger value: `10` (16) after both decreases to a 16-slot
  template, read back as 16 rows. Whether that value is the new template's slot
  count or a constant 16 is not settled (the step 4 save was lost). The value is
  never re-derived for the equipped backpack, so re-raising the template does
  not fix it.
- **In-game recovery**: un-equip the oversized backpack, save, reload, equip it
  again. It comes back at its template's size (test 3, step 5). Without a
  reload: equip a correctly sized backpack of the same type and discard the
  oversized one (done after the final revert). Restoring the character file
  also works.
- Other header bytes: the third and sixth vary with the bag's contents (the
  sixth is close to, but not exactly, the number of occupied bag slots). Not
  needed here and not identified.

## Decision: shelved

1. **UI**: the backpack grid does not scroll (chest grids do). At the
   maintainer's resolution 7 rows (56 slots) fit, with the UI shrinking slightly
   for the 7th; an 8th pushes the hotbar off screen. The vanilla top backpack
   already makes 7 rows (32 + the player's 24), so the headroom is one extra row
   on the lower tiers and none on the top tier.
2. **Rollback**: any decrease that drops a row, which no tweak can prevent,
   breaks the equipped backpack until the player un-equips it, saves and
   reloads.

The tweak code is not merged. The format notes stand for later work, such as a
save repair or other save-side recon.

## Risks & Assumptions

- **Rollback**: disable the tweak and rebuild, or revert the commit. What
  happens to items stored in the extra slots on rollback is AC4. If they are lost,
  the description warns to empty the extra slots before removing the tweak.
- **Existing backpacks**: placed chests keep their old size until rebuilt, and a
  backpack is a container entity too, so an owned backpack may keep its vanilla
  size until crafted again (AC3).
- **Player inventory cap**: if the player's 40-slot `InventorySetup` caps the
  total, slots past it may not appear. If AC3 shows that, raising `1_Player` is
  added to this spec as a requirement, not guessed at now.
- **UI layout**: the backpack screen is untested at large counts. The default
  cap of 128 is a guess, to be lowered if AC3 shows a layout problem.
- **Tier from name**: the formula depends on the `Backpack_T<N>` naming. A
  renamed or new backpack is not matched and does not appear in the AC2 count,
  which is how a game update that changes this would show.
- **Overlap**: `chest_slots`, `more_loot`, `rune_drops` and others also write
  `TemplateResource`, but on other templates (`Prop_Decoration_T*_Storage_*`,
  loot containers). No other tweak writes `ItemInfo.uiValues`; `big_stacks` writes
  `ItemInfo.maxStackSize` of stackable items, and backpacks do not stack.

## E2E Test Plan

Environment: the maintainer's Linux/Proton install, single player.

1. Install from the branch (`make install`), enable `backpack_slots` at
   defaults, build and deploy. `enshroudedbonker report` shows the AC2 line.
2. In game, open the inventory with the current backpack (the second tier,
   T1_NEW). If it shows 32 slots and its tooltip says 32, the owned backpack
   changed. If not, craft a new T1 backpack, equip it and check again (AC3).
3. Fill every slot to confirm the extra slots hold items (AC3).
4. Leave items in the extra slots, disable the tweak, build and deploy, and
   load the save. Record whether the items are still there, in hidden slots, or
   gone (AC4). Re-enable the tweak afterwards to check whether hidden items come
   back.

### Prod visibility

Not applicable: a desktop tool with no server side. The build report line from
step 1 is the observable record.
