# Status — 2026-10-19

Where enshroudedbonker stands after the first recon session. Read this first
when picking the work back up.

## Summary

- **0.0.1-alpha tool built** on branch `feat/alpha-core`: headless core,
  cobra CLI (`enshroudedbonker`), fynedesygn window (`enshroudedbonker-gui`),
  parity test, the ten tweaks as embedded EML mods with `@param` headers, emm
  built from the pinned kfc-parser v0.2.0 tag, compatibility ledger with the
  1.0 build verified, vanilla copy, shadow builds, atomic deploy and rollback.
- **Verified so far** against copies of the 1.0 archive in a scratch install:
  compat check (all ten tweaks apply), build of all ten (3014 patches), deploy,
  rollback back to the vanilla hashes. Not yet run against the real install
  or checked in game (spec 002 AC5).
- **Approach** (spec 001): upstream kfc-parser/EML `emm` patches the archive
  offline as an external program. We don't reimplement the format or the Lua
  API, and we don't diverge from upstream.
- **Not in 0.0.1:** save backup before deploy (R8.1 deferred), Windows,
  GitHub repo and issue (to be created by the maintainer).

## Moving the real install onto the tool

The install is patched by hand (below), so `pristine capture` will refuse it.
Import the vanilla backup instead, then build and deploy with the tool:

```sh
enshroudedbonker pristine import work/backup-20261019T063510
enshroudedbonker tools get-emm        # or: config set emm_path <emm>
enshroudedbonker mods enable big_stacks chest_slots ...   # build 8's set
enshroudedbonker build && enshroudedbonker deploy
```

## The maintainer's game install is patched

`~/.local/share/Steam/steamapps/common/Enshrouded` is running **build 8** (deployed 2026-10-19 14:36; build 7 plus Better loot and Cheaper upgrades):

| File | State |
| --- | --- |
| `enshrouded.kfc` | patched, sha256 `5e81c5f27b1dd3b8…` |
| `enshrouded.kfc_resources` | original + appended patch records, sha256 `85df25c9b6c7906c…` |
| `enshrouded_055.dat` … `enshrouded_063.dat` | nine **empty** files `emm` adds and registers in the `.kfc`. The game won't start without them. |

- **Vanilla originals:** `work/backup-20261019T063510/`. Expected hashes are
  `a030bcdb…` (`.kfc`) and `cf47db07…` (`.kfc_resources`). Game build
  1269579 (`release_1.0`, 2026-10-16).
- **Undo:** quit the game, then run `work/restore.sh`, or use Steam "Verify
  integrity of game files". `restore.sh` also removes the nine empty `.dat`
  files.
- **Updates:** a Steam update or verify replaces the patched files. Rebuild
  against the new build before redeploying; never reuse old outputs.
- **Saves:** stack sizes and chest sizes persist in the world save even after
  rollback.

### How builds were made (manual, pending the tool)

1. Copy the vanilla `enshrouded.exe`, `enshrouded.kfc` and
   `enshrouded.kfc_resources` into a scratch dir.
2. Put the mods in `<dir>/mods/`.
3. Run `emm run -g <dir> --patch`. `emm` is built from kfc-parser `v0.2.0`
   with `cargo build --release -p mod-manager-cli`.
4. Unpack the result with kfc-parser `cli-legacy` and diff it against
   vanilla. Every build so far changed only the intended fields.
5. Deploy. The bridge can't send a 215 MB file, but `emm` only appends to
   `.kfc_resources`, so `work/buildN/` holds the patched `.kfc` and just the
   appended tail. `work/deployN.sh` rebuilds the resource file from the
   backup plus that tail, adds the empty `.dat` files, and prints hashes.

## Tweaks — e2e results on the real install

| Tweak | Result |
| --- | --- |
| Big stacks ×10 | ✅ working |
| More chest slots ×2 (cap 96) | ✅ working (newly built chests) |
| More resources: pickups ×3, harvestables ×3, terrain ÷3 | ✅ working (stone, wood, resin, fibre bushes, terrain) |
| More loot: enemy rolls ×2, loot stacks ×2 | ✅ working (spores, cloth drop 2) |
| Salvage returns ×2 (`trashStart/End`) | ⚠️ not explicitly confirmed. Rune payout for salvaging gear is *assumed* to come from these two balancing fields. Ask. |
| World-setting sliders (resource drop to 10×, upgrade recycling to 200%) | ❓ not checked |
| Fast-travel destinations (Dungeons, Locations) | ❓ not tested |
| Faster crafting ×0.1 | ❓ not tested |
| Craft from station storage | ❓ not tested |
| Better loot: rarity weights Rare ×1.5, Epic ×2, Legendary ×2 (`LootableItemsResource.globalRarityDropRates`); uncertain item-set chances ×2, capped at 1 (433 `DefaultInventoryResource` entries); gem slots ×1 | ❓ build 8, not tested |
| Cheaper upgrades: `BalancingTable.perkCostFactor{WeaponsShield,Armor,Rings}` ×0.5; world setting `perkCostFactor` min 0.25 → 0.1, plus slider steps | ❓ build 8, not tested |

Field-level detail, and the wrong turns taken getting there, are in spec 002
R5.4.

## Findings worth keeping

- **`emm` runs mods in random order.** It keeps them in a Rust `HashMap`.
  Two mods writing the same field give nondeterministic results, so the tool
  must detect write overlaps.
- **`emm`'s output files.** It writes `.kfc`, appends to `.kfc_resources`,
  keeps `.kfc.bak`, writes `.cache/`, and **adds nine empty `.dat`
  containers**. Deploy must carry every new file.
- **Array append in EML mods** works through `table.insert(arr, v)` or
  `table.insert(arr, pos, v)`. Assigning `arr[#arr + 1]` fails.
- **Variant components** expose `.type` and `.value`.
- **Built-in quick-stack.** 1.0 already has an area-deposit system,
  including "deposit stacks", which may cover quick stack. Not verified in
  game.
- **Fast travel from inside the Shroud** is blocked in code, not data. It
  needs a runtime or memory-patch spec later.
- **Public-repo hygiene.** `recon/probes.json` (game-derived IDs) was removed
  before the first commit; `live_probe.py` now takes `--probes` explicitly.

## Next steps

1. Finish the e2e checks still open above: salvage, sliders, fast travel,
   crafting, station storage.
2. Answer spec 002's open questions Q2–Q4: save location, `.dat` need, `emm`
   distribution.
3. Open the GitHub issue for spec 002, then start implementation:
   `internal/steam`, then `compat` and the fingerprint, then `emm`
   tooling, then the build/deploy core, then the GUI.
4. Convert the recon mods into built-in tweaks with `@tweak`/`@param`
   headers (R5.2).
