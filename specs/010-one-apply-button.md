# Spec 010 — One Apply button

**Issue**: #29

## Context

Each Steam update overwrites `enshrouded.kfc`, which removes the mods. The game had
four builds between 2026-10-21 and 2026-11-04. Restoring the mods takes up to four
presses in the right order (capture, compatibility check, build, deploy), spread over
five cards of explanatory text.

A partial update adds a dead end. Build 1287369 replaced `enshrouded.exe` and
`enshrouded.kfc` and left `enshrouded.kfc_resources`, the patched copy from the last
deploy, in place. `steamReplacedPair` requires both files to be rewritten, so the
leftover containers counted as patch signs and capture refused. With no vanilla copy
for the new build, build, deploy and rollback were also blocked.

Steam's Verify then restored a resource file whose hash (`2f54eb9e…`) equals the
vanilla copy held for the previous build. That confirms the file did not change
between those builds, and that the correct vanilla pair can be assembled without Verify.

## Requirements

### Apply

- **R1.** `core.Apply` takes a request struct, returns a result struct, and does what
  is missing, in this order:
  1. Refuse if the game is running, before any other work.
  2. Get emm if it is not installed.
  3. Capture the vanilla copy if none is held for this build (R4).
  4. Run the compatibility check if the gate verdict is `unchecked`, or `unsupported`
     from a check that failed earlier, since a press can still change that.
  5. Build the enabled mods, leaving out tweaks the gate disabled for this build (R2).
  6. Deploy the build.
- **R2.** An enabled built-in that the gate disabled for this build is skipped, and the
  remaining mods are deployed. The result names each skipped tweak. Every other failure
  stops the run before the game is written, including a library mod failing to build
  and a mod emm did not run.
  If every enabled mod is skipped, the run stops and nothing is written.
- **R3.** The run is cancellable until the deploy starts. Once files are being written
  it runs to the end, because the deploy writes the index last and a partial deploy
  leaves the old index usable.
- **R3a.** The CLI gains `enshroudedbonker apply`, and `Actions()` lists it (parity test).

### Partial updates

- **R4.** Capture accepts an install where Steam replaced only the index. All of these
  must hold:
  - the deploy record exists and the installed fingerprint differs from its fingerprint;
  - `enshrouded.kfc` differs from the deployed one and was written at or after the
    manifest's `LastUpdated`, less `updateSlack`;
  - `enshrouded.kfc_resources` hashes to the deployed resource file;
  - the vanilla copy for the deployed fingerprint is held, and the install's resource
    file starts with exactly those bytes (`ResSize`, `ResSHA256`).

  The new vanilla copy is the install's `enshrouded.kfc` plus the held resource file.
  Its `source` names the build the resources came from. Containers listed in the deploy
  record's `Added` count as leftovers. An `enshrouded.kfc.bak` remains a patch sign.
- **R5.** Any case R4 does not cover still refuses, as it does today. This includes an
  install with no manifest or no deploy record.
- **R6.** After a capture or import, vanilla copies are pruned to the newest two by
  capture time. The copies for the installed and the deployed fingerprints are never
  pruned, so at most three can remain.

### State

- **R7.** `StatusResult` gains one state and one sentence, computed in core so both front
  ends say the same thing:

  | State | When | Example sentence |
  | --- | --- | --- |
  | `applied` | the install is what was deployed and matches the settings | Your 12 mods are in the game. |
  | `settings_changed` | deployed, but the enabled mods or parameters differ | Your mod settings changed since the last deploy. |
  | `update_removed` | a deploy record exists for a different build | A game update removed your mods. |
  | `not_applied` | no deploy record, or the install is vanilla | Your mods are not in the game. |
  | `modified` | the archive is neither vanilla nor the deployed one | Another tool changed the game's archive. A copy is kept when you apply. |
  | `needs_verify` | no vanilla copy, and capture would refuse, or the install is mid-update | The game's files need restoring in Steam. |
  | `none_enabled` | nothing is enabled and nothing is deployed | No mods are enabled. Choose some under Tweaks or Mods. |
  | `no_game` | the install was not found | Enshrouded was not found. Set its folder under Advanced. |

  A running game overrides the sentence with "Quit Enshrouded to apply your mods." and
  does not change the state, except in `applied`, where there is nothing to apply.
- **R8.** The deploy record gains the parameters of each mod (`params`, optional), which
  `settings_changed` compares against the settings. A record without them compares mod
  names only. Tweaks the gate disabled for this build are left out of the comparison.

### Window

- **R9.** Overview reads, from the top: the sentence, the main button, the secondary
  buttons, the deployed mods and any skipped tweak, one line of advice (saves keep bigger
  stacks after removal; do not join other people's servers with a patched game), and the
  steps and log of the current or last run.
- **R10.** The main button is **Apply mods**, with no confirmation dialog. In `needs_verify`
  the same button, in the same position, reads **Verify in Steam** and opens
  `steam://validate/1203620`. It is disabled while the game is running.
- **R11.** The secondary buttons are **Remove mods…** (today's Rollback, keeping its
  confirmation) and **Refresh**. Remove mods… is disabled when there is nothing to remove.
- **R12.** A new **Advanced** section replaces the Build section. It holds the Game, emm,
  Vanilla copy, Compatibility and Deployed cards with their existing buttons, the Last
  build report under its divider, and Build, Build without a dry run and Deploy…. A manual
  build moves to Overview to show its log, as it moves to Build today.
- **R13.** The README's Use section leads with `apply` and keeps the separate commands as
  the longer route.

## Acceptance Criteria

Apply:

- [x] AC1: From a state with no vanilla copy and an unchecked build, one `Apply` performs
  every step and deploys. With emm missing, it asks for the download first. Test. (The
  download is stubbed in that test; `GetEmm` has its own online test.)
- [x] AC2: From an up-to-date state, `Apply` skips emm, capture and the check, and still
  builds and deploys. Test.
- [x] AC3: With the game running, `Apply` returns the refusal and neither workspace nor
  install changes. Test.
- [x] AC4: A built-in the gate disabled is skipped: the others deploy and the result names
  it. Test.
- [x] AC5: A failing library mod stops the run and the install is byte-identical. Test.
- [x] AC6: With every enabled mod disabled by the gate, the run stops and writes nothing.
  Test.
- [x] AC7: Cancelling before the deploy leaves the install byte-identical. Test.
- [x] AC8: `enshroudedbonker apply` exists and the parity test passes with `apply` in
  `Actions()`. Test.

Partial updates:

- [x] AC9: The partial-update state is captured, and the new vanilla copy holds the
  install's index and the held resource file, with the containers reported as leftovers.
  Test.
- [x] AC10: Capture refuses when any one R4 condition fails, one test case per condition,
  and when `enshrouded.kfc.bak` exists. Test.
- [x] AC11: After a partial-update capture, build and deploy, Rollback leaves the install
  with the vanilla pair and no leftover containers. Test.
- [x] AC12: Pruning keeps the newest two and never removes the installed or the deployed
  fingerprint's copy. Test.

State:

- [x] AC13: Each R7 state is reported from a fixture of that state, with its sentence. Test.
- [x] AC14: A changed parameter gives `settings_changed`; a record with no `params` compares
  names only; a gate-disabled tweak does not cause `settings_changed`. Test.

Window:

- [x] AC15: Overview draws one high-importance button, plus Remove mods… and Refresh, and no
  Build, Deploy, Capture, Import or compatibility-check button. Test.
- [x] AC16: The main button keeps its position and size in `needs_verify` and while the game
  is running, and is disabled in the latter. Test.
- [x] AC17: Advanced draws the cards and controls in R12, and `SectionNames()` has Advanced
  and not Build. Test.
- [x] AC18: Every operation in `Actions()` is reachable from a section. Test.

Whole:

- [x] AC19: `make test` and `make lint` pass, and the CLI builds with `CGO_ENABLED=0`.
- [x] AC20: Real emm and a real install: Apply patches the game and the mods work in game
  (see E2E Test Plan).
- [ ] AC21: A real Steam update that leaves `enshrouded.kfc_resources` alone is handled by
  one press, with no Verify (see E2E Test Plan). This depends on the game's release
  schedule, so it may be recorded after the merge.

## Risks & Assumptions

- **Capture is now implicit.** A press can capture an install as vanilla with no one
  reading a prompt. The guards are the existing patch signs, the deployed-index check, and
  the build's check of its output against the vanilla bytes. An archive patched by another
  tool that leaves no signs would be captured as vanilla, as a manual capture would today.
- **R4 assumes Steam leaves a file alone only when its depot content did not change.** The
  hash match on build 1287369 supports this, from one observed update. If it is wrong, the
  game would get a resource file from the previous build, with an index that expects
  another. Verify remains the recovery, and Remove mods… would restore the same mismatch.
- **No moment between build and deploy.** This reverses the decision recorded on
  `buildBuild`. A mod that runs but misbehaves is deployed. Remove mods… undoes it, and
  Advanced keeps the separate route.
- **A partial mod set can be deployed silently if the sentence is missed.** Skipped tweaks
  are stated on Overview and in the result, and status stays warning-coloured.
- **`settings_changed` does not notice a library mod's files changing,** only its enablement.
- **Pruning deletes vanilla copies of old builds.** Nothing uses them except R4, which needs
  only the deployed fingerprint's copy.
- **`--section Build` stops working.** Assumed acceptable in an alpha.
- **`steam://validate` needs Steam's handler registered.** If opening it fails, the window
  says to use Steam's Verify integrity of game files.
- **Spec 008's tests are rewritten where this changes them,** because the shared strip and
  card order are superseded.
- **Rollback:** revert the merge commit. The deploy record's new field is optional, and
  vanilla copies stay valid. Copies already pruned are not restored.

## E2E Test Plan

Environment: this machine, `make install`, the game installed through Steam.

1. Open Overview with the game deployed: the sentence reads `applied`, and one main button
   is visible. (AC15)
2. Change a tweak's parameter: the sentence reads `settings_changed`. Press Apply mods and
   check the change in game. (AC14, AC20)
3. Start the game, press Refresh: the button is disabled and the sentence says to quit. (AC16)
4. Press Remove mods…: the state becomes `not_applied`. Press Apply mods: the mods return.
   (AC11, AC20)
5. Open Advanced: the cards, the report and the separate buttons work. (AC17, AC18)
6. At the next Steam update, record which files changed and press Apply mods once. If the
   resource file was left alone, this covers AC21. If both files changed, it covers the
   ordinary update and AC21 stays open.

Recorded so far, before the in-game steps:

- Real emm under Wine, through `core.Apply`, on a scratch copy of the game (real executable
  and containers by link, the archive pair copied from the vanilla copy, its own workspace):
  captured, checked, built and deployed in 29 s, 3058 patches, state `applied`. The real
  install's archive pair was hash-identical afterwards. The running-game check was overridden
  for this run because the game was open.
- Overview rendered to an image in the `update_removed` state, checked by eye.
- 2026-11-04, build 1287369: the build was installed with `make install` and the steps above
  were handed over for a run on the real install. The player reported that it works, in game.
  That report covers AC20; it did not itemise steps 1 to 5.
- Step 6 is pending: it needs a Steam update that leaves the resource file alone, so AC21
  stays open and this spec stays INCOMPLETE after the release.

### Prod visibility

Not applicable: a desktop program with no telemetry.

## Alternatives Considered

- Considered re-applying automatically in the background after an update; rejected because
  it writes into the game without a press, and needs a running process.
- Considered keeping Verify as the only route for partial updates; rejected because it
  brings back a manual step on updates that are likely to recur.
- Considered removing the separate buttons from the window; rejected because recovering from
  a backup would need the terminal, and parity would need allow-list exceptions.

## Technical Notes

Test strategy, for agreement before tests are written:

- **Level:** core tests over synthetic fixtures and a stub emm for AC1–AC14, in the existing
  `internal/core` and `internal/build` styles. Layout tests with `fynetest` for AC15–AC18.
- **Contracts:** what the run does to the install, the workspace and the result, not which
  internal functions it calls. AC5, AC6 and AC7 compare install bytes.
- **Mock boundary:** emm and the game-running check only. File writes are real, into temp
  directories.
- **Real downstream:** AC20 and AC21, by hand, because the game cannot run in CI.

`Build` needs a way to take the reduced mod list. An unexported helper shared by `Build` and
`Apply` keeps the public behaviour of `Build`, which still refuses a gate-disabled tweak.

The main button needs one fixed width covering both labels (design system: nothing transient
reflows the interface). Any shape fynedesygn lacks goes under "Gaps found" here, for a library
change.

## Executive Summary

Overview now has one Apply mods button and one sentence for the game's state. The press does
whatever is missing (emm, vanilla copy, compatibility check, build, deploy), and an update
that leaves `enshrouded.kfc_resources` alone no longer needs a Verify. Reviewers should look
first at `partialUpdate` in `internal/build/pristine.go`, since it accepts an install with
patch signs as vanilla, then at `Apply` in `internal/core/apply.go`. AC21 waits for a real
partial Steam update.

## Status: INCOMPLETE
