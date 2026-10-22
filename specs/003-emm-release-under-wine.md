# Spec 003 — Run upstream's released `emm.exe` under Wine

**Status**: COMPLETE
**Issue**: #3

## Context

Spec 002 R3 has the tool build `emm` from the pinned kfc-parser tag with the
player's own `git` and `cargo` (`tools build-emm`). Spec 002 justified this with
"upstream has no release for 1.0". That was wrong: kfc-parser publishes GitHub
Releases, and v0.2.0 (our pin) was published 2026-10-16, the day game build
1269579 (`release_1.0`) shipped. The release assets are Windows-only: `emm.exe`,
`kfc-parser.exe`, `dbghelp.dll`, `dinput8.dll`.

Asking a player to install a Rust toolchain, plus about 470 MB of sources and
build output, so the game can have bigger stacks is too high a bar. Every player
of the game on Linux already has Proton, which is Wine. The project rule in
`.claude/CLAUDE.md` (Environment) was changed on 2026-10-19 to make upstream's
released `emm.exe`, run under Wine, the default. This spec implements that, and
removes the source build from the program: the goal is a fresh setup, end to end,
exactly as a new player sees it, with no Rust toolchain anywhere in that path.

### Feasibility (verified 2026-10-19, on copies of the 1.0 archive)

- `emm.exe` v0.2.0, downloaded from the release, matches the SHA-256 GitHub
  publishes for it (`b1133f6b8ea7000596cc7710901bd8307b48165b0e650c0487a611d90ea49305`).
- It reports `mod-manager-cli 0.2.0` under system Wine 11.19, Proton
  Experimental, Proton Hotfix and GE-Proton10-29. Proton's `files/bin/wine` runs
  directly, outside Steam's runtime container.
- A full build of all ten built-in tweaks applies 3014 patches under system Wine
  and under Proton Experimental's Wine, the same as the native build.
- Unpacking the ten resource types the tweaks touch (kfc-parser's `cli-legacy`)
  gives 16,956 records that are identical between the native build and both Wine
  builds. All three differ from vanilla in the same 3014 records.
- Output bytes are not comparable: two native runs on identical input already
  differ inside the appended region. Equivalence has to be checked on unpacked
  records.
- A fresh prefix opens a wine-mono install dialog. `WINEDLLOVERRIDES=mscoree,mshtml=`
  suppresses the mono and Gecko prompts; emm needs neither. With `DISPLAY` and
  `WAYLAND_DISPLAY` unset the run is headless, so it also works over SSH.
- Cost: about 8 s to create the prefix on first use and about 5 s of Wine startup
  per run after that. The prefix is 380–620 MB depending on the Wine build.

## Requirements

### R1 — Pin and download

1. The pin (`internal/emm`) records the tag and the SHA-256 of the release's
   `emm.exe`. Bumping the pin updates both, in one place.
2. `tools get-emm` (core: `GetEmm`) downloads `emm.exe` from
   `https://github.com/Brabb3l/kfc-parser/releases/download/<tag>/emm.exe` into
   `tools/emm-<tag>/`, through a temporary name. The file is moved into place only
   after its SHA-256 matches the pin; on a mismatch the download is deleted and the
   operation fails, naming both hashes.
3. Upstream's `LICENSE` is fetched from the tag's source and stored next to the
   binary.
4. The download is cancellable and reports progress through `core.Events`.
5. Nothing is downloaded without the user asking for it (the CLI command, or the
   GUI action on Overview).

### R2 — Choosing a Wine

1. Order (decided 2026-10-19):
   1. the Proton that Steam last ran Enshrouded with. The game's Proton prefix
      records it: Proton writes `compatdata/1203620/config_info`, whose lines
      after the version are paths into that Proton (`<proton>/files/share/fonts/`,
      `<proton>/files/lib/`). Its `files/bin/wine` is used when it still exists.
      This needs no table from Steam's internal tool names (`proton_9`,
      `proton_experimental`) to directories, and covers custom Protons in
      `compatibilitytools.d` the same way. (`CompatToolMapping` in `config.vdf`
      was considered first; on the maintainer's machine it has no entry for
      the game at all, because Steam picks the game's Proton itself.)
   2. the system's default `wine` on `PATH`.
2. Status reports which Wine was chosen and why, and the GUI Overview shows it.
3. When no Wine is found, the error names each reason (no prefix yet, the Proton
   since uninstalled, no `wine` on `PATH`) and suggests running the game once
   with Proton or installing `wine`.
4. The `wineserver` used to stop a prefix is the one beside the chosen `wine`.

### R3 — Running emm under Wine

1. emm runs with its own prefix, `tools/wineprefix/`. The game's Proton prefix
   (`compatdata/1203620`) is never used or written.
2. Environment: `WINEPREFIX` as above, `WINEDEBUG=-all`,
   `WINEDLLOVERRIDES=mscoree,mshtml=`, and `DISPLAY` and `WAYLAND_DISPLAY`
   removed. The rest of the environment is inherited.
3. Paths passed to emm are converted to Wine's `Z:` drive (`Z:` plus the
   absolute path with `\` separators). No call to `winepath` is needed.
4. The argv stays explicit, with no shell. The program is the Wine binary, the
   first argument is `emm.exe`, and emm's own arguments follow unchanged.
5. Log parsing (`Applied N patches`, mod and error lines) is unchanged. The
   Windows-style paths in emm's log lines are shown as they are.
6. A cancel stops the run and also stops the prefix's `wineserver`
   (`wineserver -k` with the same `WINEPREFIX`), so no Wine process outlives a
   cancelled build.

### R4 — Locating emm, and removing the source build

1. `emm.Locate` returns a runner (the program and any leading arguments), not
   just a path, in this order:
   1. the `emm_path` setting (a native binary, or a `.exe`, which is run under
      Wine);
   2. the downloaded `emm.exe` under Wine.
2. The source build leaves the program (decided 2026-10-19): `tools build-emm`,
   `core.BuildEmm`, `emm.Build`, the GUI's build action and the git and cargo
   checks are removed, with their tests and the parity allow-list entry. The
   "not installed" message names `tools get-emm` only.
3. Building from source survives as a developer script, `scripts/build-emm.sh`,
   which checks out the pinned tag, runs `cargo build --release --locked -p
   mod-manager-cli`, and prints the binary's path for the `emm_path` setting. The
   program never calls it, and the README's player sections don't mention it.

### R5 — Front ends and docs

1. The CLI gets `tools get-emm`. The GUI Overview's emm action downloads instead
   of building. The parity test covers the new operation.
2. The README's first-run steps become: install, `tools get-emm`, `pristine
   capture`, enable, build, deploy. Neither Rust nor git is a requirement.
   `install.sh`'s closing hints change to match.
4. Spec 002 is corrected where it relied on "no release for 1.0": R3.2, Q4 and
   the first Risks bullet point to this spec.

### R6 — Windows (later)

On Windows the same download runs natively, with no Wine. Nothing here may
assume Linux beyond R2 and R3.

## Acceptance Criteria

- [x] AC1. `tools get-emm` downloads `emm.exe` v0.2.0 from the real GitHub
  release, verifies the SHA-256, and reports `mod-manager-cli 0.2.0` from
  running it under the chosen Wine. `status` then reports the verified release
  and the Wine, without starting Wine (it re-checks the digest instead).
- [x] AC2. A download whose hash does not match the pin is rejected and
  deleted, and the error names both hashes (unit test with a local HTTP server).
- [x] AC3. Wine selection follows R2, unit-tested against a synthetic Steam
  tree: the Proton named in `config_info`, that Proton uninstalled, no
  `config_info`, a `config_info` naming no Proton, an unknown prefix, and no
  Wine at all. Each case reports its reason.
- [x] AC4. A fresh prefix is created with no dialog, with `DISPLAY` and
  `WAYLAND_DISPLAY` unset; verified over an SSH session with no display.
- [x] AC5. On the real install, a build of all ten tweaks with Wine emm applies
  3014 patches, and the unpacked records of the touched types equal those of a
  native emm build. This uses the real `emm.exe` and Wine, not a mock.
- [x] AC6. Cancelling a Wine build leaves no `wineserver` or `emm.exe` process
  for the tools prefix.
- [x] AC7. The game's Proton prefix is byte-identical before and after a Wine
  build (file list and modification times of `compatdata/1203620`).
  Verified 2026-10-21 on game build 1273043: 2301 entries, type, size and
  mtime identical across a full build (3014 patches) under Proton
  Experimental's Wine.
- [x] AC8. A deploy of the Wine-built output loads in game, and big stacks show
  (spec 002 AC5's first check). Checked by the maintainer on 2026-10-22, game
  build 1273043, with the ten built-ins built under Proton Experimental's Wine
  (3014 patches), as part of the AC11 run.
- [x] AC9. The parity test passes with `tools get-emm`, and no `build-emm`
  operation remains in core, the CLI or the GUI. `make test` passes
  headless with no Wine and no network (Wine and download tests skip when
  absent). `make lint` and `govulncheck ./...` are clean.
- [x] AC10. The README's first run names no Rust or git requirement; spec 002
  R3.2, R3.3, Q4 and Risks are corrected. `scripts/build-emm.sh` builds a binary
  that works through `emm_path`.
- [x] AC11. Fresh setup, end to end, as a new player: with empty config, data
  and cache directories and a vanilla install, the installer (spec 004; it
  replaced `install.sh` as a player's way in), then the GUI alone
  (no CLI, no hand-edited files) gets emm, captures the vanilla copy, enables
  the tweaks, builds and deploys, and the game shows big stacks. Every prompt,
  dialog and wait a new player meets is noted in the PR.
  Run by the maintainer on 2026-10-22. The game was rolled back to vanilla, the
  config, data and window-preference directories were moved aside (there was no
  cache directory), and the wizard installer from `make installer` installed the
  program. From the window alone: Get emm, capture the vanilla copy, the
  compatibility check, Build, Deploy, and the game showed the tweaks. Nothing
  was written outside the installer's files, the config and the data
  directory. There was no `tools/src`: a new player never gets the old source
  tree. One stumble: the Game card asks for the compatibility check while its
  button is disabled with no reason given, because the vanilla copy, two
  cards further down, does not exist yet (filed on #6).

## Risks & Assumptions

- **Proton outside its runtime.** Running Proton's `files/bin/wine` outside
  Steam's runtime container works on this machine, where the host libraries are
  recent. On another distro it could fail on a missing library. Mitigation: fall
  through to the next Wine in R2's order, and report each failure.
- **Untested platforms.** Steam Deck and Flatpak Steam (Proton under
  `~/.var/app/com.valvesoftware.Steam/…`) are not verified. Older Protons are not
  verified.
- **Upstream changes the release.** A re-uploaded asset changes its hash and the
  download is refused. This is intended: refuse rather than run an unverified
  binary.
- **Network.** The first install needs github.com. Offline players can still
  point `emm_path` at a copy of `emm.exe` or a native build.
- **No Wine at all.** A player with neither a Proton for the game nor system
  Wine can't build. On Linux this means a game that has never been run under
  Proton, which can't happen for a player who has played it.
- **Disk.** The prefix is 380–620 MB, smaller than the source build it replaces
  (about 470 MB of sources and build output).
- **GPL.** The tool downloads upstream's binary from upstream at the user's
  request, so this project redistributes nothing.
- **Rollback.** Revert the change. Removing `tools build-emm` is not additive,
  but `emm_path` plus `scripts/build-emm.sh` covers anyone who relied on it.

## E2E Test Plan

Environment: the maintainer's Linux machine with the real install and Proton
Experimental.

0. Fresh player (AC11, done last, after steps 1–6 pass). Quit the game, restore
   the install to vanilla (Steam "Verify integrity", or `work/restore.sh`), and
   move `~/.config/enshroudedbonker`, `~/.local/share/enshroudedbonker` and
   `~/.cache/enshroudedbonker` aside. Run `install.sh`, open the window, and do
   everything from the GUI. Record each screen, prompt and wait. Restore the
   moved directories afterwards if wanted. Restoring the install writes the
   game directory, so it waits for the maintainer's go-ahead.
1. Delete `tools/emm-v0.2.0/` and `tools/wineprefix/`. Run `tools get-emm`, then
   `status`. Expect emm `mod-manager-cli 0.2.0` and the Wine source named.
   (AC1)
2. Over `ssh localhost`, with no display, run `build` with all ten tweaks. Expect
   no dialog and `applied 3014 patches`. (AC4, AC5)
3. Unpack the touched types from that output and from a native build with
   `cli-legacy`, then `diff -rq`. Expect no differences. (AC5)
4. Start a build and cancel it; `pgrep -f 'wineserver|emm.exe'` finds nothing for
   the tools prefix. (AC6)
5. Record the `compatdata/1203620` file list and modification times before step
   2 and compare after it. (AC7)
6. Deploy, launch the game, check a 50-stack item stacks to 500. Then roll back.
   (AC8)

### Prod visibility

Not applicable: a desktop tool with no service or telemetry. Players report
problems through GitHub issues.

## Alternatives Considered

- Considered building emm from source on the player's machine (spec 002 R3.3);
  rejected as the default because a Rust toolchain is too high a bar for
  players. It survives only as a developer script (R4.3).
- Considered our own CI building a Linux emm and attaching it to our releases
  (spec 002 R3.2); rejected because it makes GPL compliance our job and runs a
  build upstream never tested, when upstream's own binary works under Wine.
- Considered using the game's own Proton prefix; rejected because the tool must
  not change the game's Wine environment.

## Open Questions

- **Q1.** ~~Automatic Wine pick or a setting?~~ Answered 2026-10-19: the Wine
  Steam uses for Enshrouded, falling back to the system's `wine` (R2). No
  `wine_path` setting.
- **Q2.** ~~Keep `tools build-emm`?~~ Answered 2026-10-19: removed from the
  program; a developer script keeps the source build (R4.2, R4.3).

## Executive Summary

The program downloads upstream's released `emm.exe` for the pinned kfc-parser
tag, checks its SHA-256, and runs it under the Proton Steam uses for the game in
a Wine prefix of its own. The source build is gone from the program. A new
player's setup, from the installer to big stacks in game, needs no toolchain
and no terminal.

## Status: COMPLETE
