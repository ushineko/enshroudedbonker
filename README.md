# enshroudedbonker

**Version**: 0.0.1-alpha

Enshrouded mod builder for Linux (Steam, Proton). It runs EML Lua mods — its own
built-in tweaks and ones you import — through upstream's
[emm](https://github.com/Brabb3l/kfc-parser) against a copy of the vanilla game
archive, then deploys the result into the game and rolls it back on request.
There is a command line (`enshroudedbonker`) and a desktop window
(`enshroudedbonker-gui`) that do the same things.

Windows support is planned; the alpha is Linux only.

**Current state: [docs/STATUS.md](docs/STATUS.md).** Spec:
[specs/002-alpha-0.0.1.md](specs/002-alpha-0.0.1.md).

## What it touches

- **Builds never touch the game.** They run in a scratch copy under
  `$XDG_DATA_HOME/enshroudedbonker/workspace`, always starting from a vanilla copy
  of `enshrouded.kfc` and `enshrouded.kfc_resources` (about 225 MB per game build).
- **Deploy** writes `enshrouded.kfc`, `enshrouded.kfc_resources` and the empty
  `enshrouded_NNN.dat` containers emm adds, each atomically, and refuses while the
  game is running. **Rollback** writes the vanilla pair back and removes the
  containers.
- **Saves remember.** Stacks and chests bigger than vanilla stay in a save after a
  rollback. Single player and worlds you host only; do not join other people's
  servers with a patched game.

## Game updates

Every game update changes the archive. Each build is checked against a
compatibility ledger keyed on the archive's build string and the executable's
hash. A build the ledger does not know is dry-run first (`compat check`): a tweak
that no longer applies is disabled for that build, the rest carry on. An install
that is half updated is refused.

## Install

```sh
./install.sh            # CLI, window, launcher entry and icon into ~/.local
./install.sh --no-gui   # CLI only (no CGO needed)
```

emm is built from the pinned kfc-parser tag with your `git` and `cargo`:

```sh
enshroudedbonker tools build-emm
```

Or set `emm_path` to an emm binary you already have
(`enshroudedbonker config set emm_path /path/to/emm`).

## Use

```sh
enshroudedbonker status               # the game, its build, emm, the vanilla copy
enshroudedbonker pristine capture     # keep a vanilla copy (from an unpatched install)
enshroudedbonker tweaks list
enshroudedbonker mods enable big_stacks chest_slots
enshroudedbonker tweaks set big_stacks STACK_MULTIPLIER 20
enshroudedbonker build
enshroudedbonker deploy               # quit the game first
enshroudedbonker rollback             # back to vanilla
```

If the install is already patched, restore it with Steam's *Verify integrity of
game files*, or import a backup with `pristine import <dir>`.

## Built-in tweaks

| Group | Tweak | What it does |
| --- | --- | --- |
| Inventory | `big_stacks` | Stack sizes ×10 (capped) |
| Inventory | `chest_slots` | Chest slots ×2 (capped at 96) |
| Resources | `more_resources` | Gathering, harvesting and terrain yields ×3; raises the resource-drop world setting's ceiling |
| Loot | `more_loot` | Enemy drop slots and stack amounts ×2 |
| Loot | `better_loot` | Rarer item weights and drop chances up |
| Crafting | `salvage_returns` | Salvage returns ×2 (runes); raises the recycling setting's ceiling |
| Crafting | `cheaper_upgrades` | Vendor upgrade costs ×0.5 |
| Crafting | `crafting_time` | Crafting time ×0.1 |
| Crafting | `factory_storage_access` | Craft from storage at every station |
| World | `fast_travel_markers` | Dungeons and locations become fast-travel points |

Each is a plain EML mod under `internal/tweaks/mods/`; it works when dropped into
upstream EML's `mods/` directory as is. Parameters are the `local NAME = value`
lines at the top.

## Third-party mods

`mods import <dir|zip>` copies an EML mod (`mod.json` plus `src/mod.lua`) into your
library; it starts disabled. emm runs mods in a random order, so two mods that
change the same thing give unpredictable results; the build lists such overlaps.

## Develop

```sh
make test        # go test -race -tags parity ./...
make build       # CLI, CGO_ENABLED=0
make build-gui   # window, needs CGO and OpenGL/X11/Wayland headers
make lint
```

The recon behind the design is in [specs/001-feasibility-recon.md](specs/001-feasibility-recon.md)
and [docs/findings.md](docs/findings.md); the scripts are in `recon/`.

## Licence

MIT. emm (kfc-parser) is GPL-3.0 and is run as a separate program, never linked.

## Changelog

### 0.0.1-alpha

- First build of the tool: CLI and window over one headless core.
- Ten built-in tweaks with adjustable parameters.
- emm built from the pinned kfc-parser v0.2.0 tag.
- Compatibility gate: ledger, dry run and per-tweak disable.
- Vanilla copy, shadow builds, atomic deploy and rollback.
- Third-party EML mod import (directory or zip).
