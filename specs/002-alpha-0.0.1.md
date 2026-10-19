# Spec 002 — v0.0.1-alpha: an `emm`-driven mod builder with built-in tweaks, Linux first

**Status**: DRAFT. Recon tweaks deployed and partly e2e-tested (build 7, 2026-10-19); no Go code yet
**Issue**: — (open one before work starts; see nmsbonker's issue convention)

## Context

Spec 001 settled feasibility and the decision. Enshrouded's data is patched
offline by upstream EML's `emm run --patch` (kfc-parser, GPL-3.0). We call it
as an external tool, the way nmsbonker calls MBINCompiler. We do not
reimplement the archive format, the serializer or the EML Lua API. Upstream is
the expert and is moving fast; we don't diverge from it.

This spec is the first release that does something for a player. It manages
EML mods, ships a small set of built-in tweaks, and builds, deploys and rolls
back against the installed game. It runs on Linux, with the game under
Proton. Every architecture, repository and git rule in nmsbonker's
`.claude/CLAUDE.md` applies here unchanged. The UI is on fynedesygn, the
installers come from fynstall, and the module is
`github.com/ushineko/enshroudedbonker`.

### Facts this spec relies on (verified 2026-10-18, build 1269579 / 1.0)

- Steam app ids: client `1203620`, dedicated server `2278520`.
- `enshrouded.exe` and `enshrouded.kfc` both embed the build string
  `1269579|^/game38/branches/release_1.0|<timestamp>`. That string is the
  fingerprint.
- `emm` builds on Linux from kfc-parser `v0.2.0` (tag only, no published
  release). `emm run -g <dir> --patch` applies EML mods offline with no DLL
  proxy and no Proton setup. It writes `enshrouded.kfc` and appends to
  `enshrouded.kfc_resources`, keeps `enshrouded.kfc.bak`, and writes
  `.cache/lua/` definition files. It also creates nine **empty** containers,
  `enshrouded_055.dat`–`enshrouded_063.dat`, and registers them in the
  `.kfc`. The game refuses to start if they're missing ("could not find core
  resource package"). Found in the first in-game test. Deploy must carry
  every new file (R4.3), and rollback must remove them.
- The six required tweaks (R5.4) patched a copy cleanly together: 2,848
  patches in one `emm` run.
  Verified by unpacking the result: only the intended fields changed.
  Neither has been tried in game yet.
- Vanilla fast travel already starts from anywhere on the map, but only to
  Flame Altars, Ancient Spires and the Cinder Vault. The game blocks it while
  in the Shroud, falling or underwater. Those blocks are code; no data field
  exists for them in 12,646 reflected types. So "fast travel from anywhere"
  is not data-patchable. What *is* data is which marker types are
  destinations.

### What the community changes (Nexus, 2026-10-18)

Taken from the top files and the two largest tweak collections, Enshrouded
Patcher and EMBER. In rough order of how often each recurs:

1. Stack size
2. Loot and gather multipliers
3. Free craft and crafting time
4. Fast-travel destinations
5. Skill points and level caps
6. Chest and storage slots
7. Altar limit and build zone
8. Terrain mining speed and pickaxe range
9. Fog of war radius
10. Glider and updraft
11. Shroud fog density
12. Skip intro

Cheat Engine tables and save files are the other large categories. Both are
out of scope.

## What this is not

- **Not a runtime trainer.** No memory editing, and nothing that needs the
  game running. "Fast travel from inside the Shroud" belongs to a later
  terrariabonker-style spec.
- **Not a dedicated-server tool yet.** The target abstraction has client and
  server from day one (R1), but 0.0.1 builds and deploys only the client.
- **Not a Nexus client.** It doesn't download third-party mods.
- **Not a save editor.** Saves are backed up by copy only (R8).
- **Not supported on Windows in 0.0.1.** The code is kept portable and CI
  cross-builds it, but nothing is claimed until a later spec tests it on
  Windows.
- **Not a quick-stack feature.** 1.0 already has an area-deposit system: a
  trigger prop deposits into up to 32 chests, each chest has an opt-in
  toggle, magic chests default to on, and the UI has "deposit stacks" and
  "deposit all" labels plus a "deposit stacks" menu toggle. Data can only
  change which storage takes part. A keybind that stacks from anywhere would
  be code, so it belongs to a later runtime spec. Check what the game already
  does first.
- **Not an EML reimplementation.** A mod that works under `emm` works here,
  and one that doesn't is upstream's to fix.

## Requirements

### R1 — Find the game (`internal/steam`)

1. Find the install from Steam's own `libraryfolders.vdf` and
   `appmanifest_1203620.acf`, across every library: native, Flatpak and Snap
   Steam, and second drives. Same approach as nmsbonker.
2. Validate the install by the presence of `enshrouded.exe`, `enshrouded.kfc`
   and `enshrouded.kfc_resources`.
3. `detect` lists every place it looked and why each was rejected.
4. Read the Steam build id from the manifest.
5. Model a **target** as `{kind: client|server, dir, appid}`. Only `client`
   is enabled in 0.0.1.

### R2 — Fingerprint and compatibility table (`internal/compat`)

1. **Fingerprint.** It is the tuple:
   - the build string read from the exe (scan `.rdata` for the
     `NNNNNNN|^/game38/` pattern)
   - the build string read from the `.kfc` header
   - the Steam build id
   - the exe's SHA-256

   If the exe and `.kfc` build strings disagree, the install is
   **inconsistent**. Refuse, and advise verifying the game files.
   `emm` writes `.cache/types.json` with the same version string and an
   `is_dirty` flag. Read both as a cross-check, but our own read stays
   authoritative.
2. **Ledger.** A committed compatibility table maps a fingerprint to
   `{emm version, verdict, verified-on date, tweak availability}`. Like
   terrariabonker's layout table, it's data in one file, declared once and
   imported.
3. **Verdicts.**
   - **verified**: the fingerprint is in the ledger and was checked in game.
   - **derived**: not in the ledger, but the schema probe (R2.4) passes for
     every enabled tweak.
   - **unsupported**: the probe fails, or `emm` can't read the archive.
4. **Schema probe.** `emm` runs, with the `export` capability, on a scratch
   copy of the install. The probe is a project-authored EML mod. For each
   built-in tweak, it checks that each resource type and field the tweak
   writes exists, with the expected primitive kind. It writes a JSON report
   via `io.export`. A tweak whose fields are missing is disabled for that
   build; the others stay available.
5. **Gating.** Build refuses on `unsupported` and on `inconsistent`. On
   `derived`, it warns and asks for confirmation in both CLI and GUI.

### R3 — `emm` as a tool (`internal/emm`)

1. **Pinned.** The project pins one upstream kfc-parser commit or tag in one
   place.
2. **Distribution.** Our CI builds `emm` from the pinned upstream source for
   linux-amd64 (later windows-amd64). It publishes the binary as an asset on
   our own GitHub Release, with upstream's `LICENSE` and a source link. That
   satisfies GPL-3.0 for redistributing the binary. The tool downloads that
   asset into the tools directory and checks its SHA-256 against the value
   recorded alongside the pin.
3. **Local build.** `tools build-emm` builds from source with a local
   `cargo`, for development and as a fallback.
4. **Check.** `tools check` runs `emm --version` and the schema probe (R2.4)
   against a scratch copy. It reports the result as nmsbonker's `tools check`
   reports MBINCompiler.
5. **Calls.** The tool is called only through an explicit argv: no shell, no
   string interpolation. Its output goes to the log pane and the build
   report. Lines are parsed only for documented messages (`Applied N
   patches`, mod start and error lines), and the parsing is tolerant: an
   unrecognised line is kept, never fatal.

### R4 — Workspace, pristine copy and the shadow game dir (`internal/build`)

1. **Pristine copy.** On first sight of a fingerprint, copy `enshrouded.kfc`
   and `enshrouded.kfc_resources` into
   `$XDG_DATA_HOME/enshroudedbonker/pristine/<fingerprint>/`, checking first
   that they are unmodified. A `.kfc` with an EML backup beside it, or whose
   resource file is larger than the recorded size, is not pristine. The game
   dir is never the source of truth again.
2. **Shadow dir.** Builds run in a shadow game dir under the workspace:
   - copies of the pristine `.kfc` and `.kfc_resources`
   - read-only symlinks to `enshrouded.exe` and every `enshrouded_NNN.dat`
   - a `mods/` directory holding the staged mods (R5, R6) in build order

   `emm run -g <shadow> --patch` runs there.
3. **Writes are tracked.** After a build, every file in the shadow dir that
   is new or changed is the build output. This catches `.dat` files if a
   content mod ever writes them. The output's names and hashes go into a
   build manifest.
4. **Read-only rule.** The game dir is read-only except during Deploy (R7),
   as in nmsbonker.

### R5 — Built-in tweaks are plain EML mods (`internal/tweaks`)

1. Each tweak is a normal EML mod directory: `mod.json`, `src/mod.lua` and
   `README.md`. It's committed under `internal/tweaks/mods/`, embedded, and
   is exactly what the app shows, exports and runs. Someone without this
   repository must be able to drop it into a `mods/` folder and run it under
   upstream EML.
2. **Parameter contract, carried over from nmsbonker:**
   - Header comments: `-- @tweak name=… group=…`, `-- @desc …`, and
     `-- @param NAME label=… min=… max=… step=… default=…`.
   - Top-level assignments `local NAME = <number>` (or `true`/`false`) before
     any other code.
   - The builder rewrites only those lines in the staged copy. No generated
     Lua and no templates.
3. Each tweak prints one summary line (`<id>: <what> <count>`). The report
   picks it up.
4. **Tweaks in 0.0.1.** Eight tweaks were built together and deployed to the
   maintainer's install as "build 7" (2026-10-19; see `docs/STATUS.md`). The
   *In game* column is the e2e reading so far. The sources are in
   `recon/eml/<id>/`.

   | Tweak (`recon/eml/…`) | Writes | Parameters | In game |
   | --- | --- | --- | --- |
   | Big stacks (`stack_x10`) | `keen::ItemInfo.maxStackSize` (`u16`) where `> 1` (2,267 items) | `STACK_MULTIPLIER` 10, cap 65535 | ✅ working |
   | More chest slots (`chest_slots`) | `InventorySetup.generic/availableSlotCount`, `InventorySetupOverwrite.genericSlotCount` (`u8`) on the 12 `Prop_Decoration_T0..T6_Storage_*` templates | `SLOT_MULTIPLIER` 2, `SLOT_CAP` 96 | ✅ working (new chests) |
   | More resources (`more_resources`) | (a) `PickupItem.pickupAmount` (`u16`) on 132 `LootPickup_Gravity_*` / `LootPickup_Material_*` / `Scatter_Loot_*` pickups; (b) `DefaultInventoryResource` `countMin/countMax` for 109 inventories that harvestables reference; (c) `TerraformingEfficiencyRegistryResource.terrainConfigs[].terrainPerLootItemExchangeRate` ÷ (116 terrain materials; `buildingConfigs` untouched); (d) `GameSettingsPresetsResource.maxValues.resourceDropStackAmountFactor` → 10, plus slider steps in `FbUiBundle.difficultySettings.settingValues.dropAmount.steps` | `PICKUP_MULTIPLIER` 3, `HARVEST_MULTIPLIER` 3, `TERRAIN_FACTOR` 3, `DROP_MAX` 10 | ✅ (a)(b)(c) working: stone, wood, resin, bushes/fibre, terrain. (d) slider not yet checked |
   | More loot (`more_loot`) | (a) `RandomLootSlotCount.slotsMin/Max` (`u8`) on 406 `Enemy_*` templates, capped at 16; (b) `LootableItemsResource.items[].stackSizeMin/Max/MaxScaled` (`u16`) for every item whose `ItemInfo.maxStackSize > 1` (2,253) | `ENEMY_SLOT_MULTIPLIER` 2, `LOOT_STACK_MULTIPLIER` 2 | ✅ working (spores, cloth drop 2). (b) also affects chests |
   | Salvage returns (`salvage_returns`) | (a) `BalancingTable.trashStart/trashEnd` (`u32`, vanilla 5/20) ×N, *believed* to be the rune payout for salvaging; (b) `maxValues.perkUpgradeRecyclingFactor` → 2.0 plus slider steps (upgrade-refund world setting; vanilla max 1.0, Default 0.5) | `SALVAGE_MULTIPLIER` 2, `RECYCLING_MAX` 2.0 | ⚠️ (a) not explicitly confirmed. The final "working" came after a test asking about drops and salvage together, so ask again. (b) unchecked |
   | Fast-travel destinations (`fast_travel_markers`) | `MapMarkerRegistryResource.mapMarkers[].isFastTravelDestination` for `Dungeons`, `Locations` (13 marker types) | `DUNGEONS`, `LOCATIONS` | ❓ not tested |
   | Faster crafting (`crafting_time`) | `RecipeRegistryResource.recipes[].craftingDuration` (ns; 368 timed recipes) | `CRAFT_TIME_FACTOR` 0.1 | ❓ not tested |
   | Craft from station storage (`factory_storage_access`) | adds `InventoryCraftingStock { canReceiveAreaDeposit = false }` to the 33 `FactoryStation` templates via EML's `table.insert` | none | ❓ not tested |
   | Skill points per level | `BalancingTable.skillPointsPerLevel` (vanilla 2) | `SKILL_POINTS` | not built |
   | Altar limit | `BalancingTable.altarsPerFlameLevel[]` | `ALTAR_MULTIPLIER`, `ALTAR_CAP` | not built |

   **Wrong turns, recorded so they aren't repeated:**
   - **`ResourceNodePickupDrops.drops[].stackAmount` is not the yield.**
     Patched ×3 in build 1 with no visible effect. A node's yield is the
     number of pickups it spawns times each pickup's own
     `PickupItem.pickupAmount`.
   - **Harvestables don't drop pickups.** Fibre bushes, berries and other
     plants you press E on carry a `DefaultInventory` component that points
     at a `DefaultInventoryResource`.
   - **Terrain is a per-material exchange rate**, not a drop table.
   - **Most material loot entries are 1–1.** Selecting loot entries by
     `stackSizeMax > 1` skipped spores and cloth (build 6). Select by
     `ItemInfo.maxStackSize > 1` instead.
   - **World-setting sliders have fixed step lists** in `FbUiBundle`.
     Raising `maxValues` alone doesn't make a higher value selectable.
   - **A generic recursive walk over all templates aborts `emm`.** It runs
     out of Lua reference slots (Q5), so use direct paths.

   **More chest slots** affects only newly built chests: placed ones keep
   their saved size until rebuilt. Bigger chests persist in the save after
   rollback (R8.2). The game also has a save-size budget for storage
   (`increaseSizeProhibitedSavegameBudget`), which a large `SLOT_CAP` may run
   into.

   **More loot (b)** shares `LootableItemsResource` with chests. Splitting
   enemy loot from chest loot would need the loot-label tree; not attempted.

   **Overlap rule (R6.3) and reads.** `more_loot` *reads*
   `ItemInfo.maxStackSize`, which `stack_x10` *writes*. The run order doesn't
   matter here: any stackable item is above 1 either way. The overlap check
   should count writes against writes, and flag only a read that depends on
   the written value.
5. **Fast-travel README.** It must state what the tweak does and does not
   do. It adds destinations; it does not lift the in-Shroud, falling or
   underwater blocks. Carry EMBER's warning too: travelling to a marker can
   leave a gravestone unrecoverable, or land you outside the playable map.

### R6 — Third-party EML mods (`internal/library`)

1. **Import.** `mods import <dir|zip>` copies an EML mod into
   `$XDG_DATA_HOME/enshroudedbonker/library/`, validating `mod.json`:
   required fields, id grammar, and capabilities limited to `patch`/`export`.
   A mod requiring `runtime` is imported but marked unsupported.
2. **Enable and order.** Mods can be enabled, disabled and ordered. Built-in
   tweaks and library mods share one build order, as in nmsbonker's single
   build-order list.
3. **Load order.** `emm` has no load order. It keeps mods in a Rust
   `HashMap` (`runner/state.rs`), so the order changes from run to run;
   recon saw two different orders for the same mods. Consequences:
   - Two mods that write the same field give a nondeterministic result.
     Build detects overlapping resource types between enabled mods and
     refuses until one is disabled, or the user accepts it. Built-in tweaks
     must not overlap each other.
   - The UI has no ordering control, only enable/disable.
   - Raise deterministic ordering or dependencies upstream (`dependencies`
     is marked WIP in the manifest docs) rather than working around it.
4. **No third-party code is committed.** Same rule as nmsbonker.

### R7 — Build, deploy, rollback, status (`internal/core`)

All of these are headless core operations with request/result structs, with
CLI/GUI parity enforced by a parity test, as in nmsbonker.

1. **Build.**
   1. Fingerprint the install.
   2. Gate (R2.5).
   3. Stage the enabled mods with their parameters into a fresh shadow dir.
   4. Run `emm`.
   5. Collect the outputs (R4.3).
   6. Write the build report.

   It's cancellable through `context.Context` and reports progress through
   events.
2. **Deploy.**
   - Refuses while `enshrouded.exe` is running.
   - Refuses unless the build's fingerprint matches the install's current
     one.
   - Archives the game's current `.kfc`, `.kfc_resources` and any file the
     build replaces into `archive/<timestamp>/`.
   - Writes each output file atomically (temp file plus rename, same
     filesystem).
   - Records a deploy manifest.
3. **Rollback.** Restore the pristine pair for the current fingerprint and
   remove anything the deploy manifest added. Restoring from `archive/`
   entries is also available.
4. **Status.**
   - Install, fingerprint and verdict.
   - `emm` version.
   - Deployed or not, and whether the deploy is stale. It's stale when the
     game's `.kfc` no longer matches the deploy manifest, i.e. Steam updated
     or verified the game.
5. **Game updates.** When status sees a new fingerprint, the old deploy is
   gone (Steam replaced the files). Mark it, take a new pristine copy, and
   offer a rebuild.

### R8 — Saves

1. Before the first deploy for a fingerprint, back up the save folder by copy
   only. Its location under Proton (`compatdata/1203620/pfx/…` or Steam
   `userdata/…/1203620/`) is open question Q2.
2. Stack-size and slot changes persist in a save after a rollback: items
   above vanilla limits stay. The tweak's README says so, and the Deploy
   confirmation says so once.

### R9 — Report

Per mod:
- the summary line (R5.3)
- `emm`'s `Applied N patches`
- errors and warnings
- the build manifest (output files with old and new sizes and hashes)

A per-field conflict report, showing which mods wrote the same field, is
explicitly deferred. 0.0.1 detects only mods that patch the same resource,
when `emm`'s logs make that visible.

### R10 — GUI (fynedesygn) and CLI

1. **CLI.** cobra, with `--json` on every command. It builds with
   `CGO_ENABLED=0` and doesn't link the GUI.
2. **GUI sections.**
   - **Overview:** game, fingerprint, verdict, `emm`, deploy state, actions.
   - **Tweaks:** one build-order list, with parameter sliders from `@param`.
   - **Mods:** the library, with import, enable and order.
   - **Build & deploy:** log pane, report, deploy and rollback.
   - **About.**

   Every shape comes from fynedesygn. Any gap goes into "Gaps found" for a
   library change.
3. **Copy voice and README for players:** no internals in the README. Depth
   goes in `docs/` and `specs/`.

### R11 — Packaging

1. A fynstall manifest, `fynstall.yaml`, with `make installer` producing the
   Linux installers and CLI-only installer. The fynstall version is pinned in
   the `Makefile`.
2. **Windows.** CI builds a Windows installer from the same manifest. It's
   marked unsupported for 0.0.1, and the README says so.
3. **Version files.** `VERSION` is `0.0.1-alpha`. `VERSION`, the README's
   `**Version**` line and the newest changelog heading are the same string.
   Releases are published by CI from a `v*` tag, as in nmsbonker.

### R12 — Public-repository hygiene

1. No game data in the repository. `recon/probes.json` from spec 001 holds
   item ids and GUIDs taken from game data. Before the repository goes
   public, replace it with a generator that reads the install at run time.
2. Tests needing game data read the install from `ENSHROUDEDBONKER_GAME_DIR`
   and skip when it's absent. Synthetic fixtures are small and hand-written.
3. No personal paths, Steam ids or hostnames in committed files.

## Acceptance criteria

- [ ] AC1. `detect` finds the Linux install and reports the fingerprint;
  `status --json` carries it.
- [ ] AC2. `tools check` installs `emm` from our release asset (hash checked)
  and the schema probe passes on 1.0. A synthetic missing-field fixture
  disables just that tweak.
- [ ] AC3. A build with the six required tweaks succeeds in the shadow dir. The game dir is byte-identical before and
  after the build (hashes recorded in the report).
- [ ] AC4. Deploy refuses while the game is running and when the
  fingerprints differ. Otherwise it archives and writes atomically, and
  status says deployed.
- [ ] AC5 (e2e, real install, pasted into the PR):
  - an item that stacks to 50 in vanilla stacks to 500 in game
  - a Dungeons or Locations marker offers fast travel and the travel
    completes
  - with More resources at defaults, harvesting a node gives ×3 its
    vanilla yield
  - the world's "Resource drop amount" setting can be raised above 2×,
    and the increase shows in game
  - Faster crafting: a smelter job finishes in a tenth of its vanilla time
  - Craft from station storage: a workbench recipe consumes an ingredient
    held only in a station's output
  - More chest slots: a newly built chest shows double its vanilla slots
  - each optional tweak shipped in 0.0.1 is shown working
- [ ] AC6. Rollback restores the pristine pair, and the game's `.kfc` hash
  equals the pristine hash. The game launches and shows vanilla stacks.
- [ ] AC7. After a Steam "Verify integrity", status reports the deploy as
  stale. After a simulated new build id, it reports a new fingerprint.
- [ ] AC8. A third-party EML mod imported from a zip builds alongside the
  tweaks. A mod writing the same resource type as an enabled tweak is
  reported as an overlap before `emm` runs.
- [ ] AC9. CLI/GUI parity test passes. `make test` passes headless with no
  game. `make lint` and `govulncheck ./...` are clean.
- [ ] AC10. The Linux fynstall installer installs, launches the window and
  uninstalls cleanly. The CLI-only installer works over SSH.
- [ ] AC11. The three version strings agree. The README is player-facing
  and states Linux-only and alpha.

## Risks & assumptions

- **Upstream moves.** `emm` has no release for 1.0 and is under heavy
  development, so its interface or behaviour may change between pins. The
  mitigations are pinning, the probe on every new pin, and parsing only
  documented log lines. If something breaks, report it upstream rather than
  working around it locally.
- **The game is server-authoritative.** Client-side data patches apply in
  single player and sessions you host. Joining someone else's server with a
  patched client may mismatch or do nothing. 0.0.1 targets single player and
  self-hosting, and says so.
- **Fast-travel side effects** (gravestones, off-map landings) are upstream
  game behaviour, documented rather than mitigated.
- **Archive growth.** Every build starts from the pristine copy, so
  `.kfc_resources` never accumulates appends across builds. It grows by the
  patched records only.
- **GPL boundary.** `emm` runs as a separate program and is never linked.
  When we redistribute its binary, its license and source link travel with
  it.

## Alternatives considered

- **The EML proxy DLL (`dbghelp.dll`) instead of the offline patch.** It
  patches at every launch and needs `WINEDLLOVERRIDES` set in Steam's launch
  options. Rejected for 0.0.1: the offline route keeps Steam's launch options
  untouched, makes the build observable and testable, and fits nmsbonker's
  build/deploy model. Revisit when EML's `runtime` capability lands.
- **A native Go archive reader and serializer.** Rejected (spec 001
  decision).
- **Hosting EML mods in Go Lua.** Rejected (spec 001): it duplicates EML's
  API and diverges from upstream.

## Open questions

- **Q1.** ~~How does `emm` order mods?~~ Answered: it doesn't (R6.3).
- **Q2.** Where are Enshrouded saves under Proton, and does Steam Cloud
  sync them? It decides R8.
- **Q3.** Does `emm` need the `.dat` files for resource-only mods? (Spec 001
  ran without them.) The shadow dir symlinks them anyway, so this only
  affects the tests.
- **Q4.** Should our release attach upstream's `emm`, or should a user's
  first run build it? R3.2 assumes we attach it; confirm before CI work.
- **Q5.** EML mods must target their data precisely. A generic walk over
  every template's value tree ran `emm` out of Lua reference slots and
  aborted it (seen during recon). Built-in tweaks should use targeted
  access paths, and the docs should tell third-party authors the same.
