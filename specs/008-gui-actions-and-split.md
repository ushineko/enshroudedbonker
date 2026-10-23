# Spec 008 — One action strip, a draggable Last build, and an Overview a new player can follow

**Status**: INCOMPLETE
**Issue**: #6

## Context

Issue #6 lists four problems in the window, found in desk checks of 0.0.1-alpha:

1. Overview has Build, Deploy, Rollback and Refresh in a strip along the bottom;
   Build has Build, Cancel and Deploy in a toolbar under its heading. Going from
   one section to the other, Build and Deploy change place.
2. The Last build table under Build is fixed at 180 px while the output log takes
   the rest of the height, so the per-mod report is a few rows behind a scroller.
3. The Game card's "Run the compatibility check" button is disabled until emm is
   installed and the vanilla copy is held, and nothing says so.
4. The cards run in the pipeline's order, but the first one (Game) asks for the
   compatibility check, which needs the two cards below it.

The design system (fynedesygn `docs/design-system.md`) already covers each of
these: controls that start work sit at a fixed edge ("A control that starts work
stays in position"); a pane worth reading sits under a divider the user can drag,
whose position the shell keeps (`Shell.VSplit`, "Dividers"); a button the user
cannot use is disabled, not hidden; and the Cancel of a job is on the busy popup,
not in the toolbar behind it. `Shell.VSplit` is in the pinned fynedesygn v0.1.83,
so no library change is needed.

## Requirements

- **R1.** Overview and Build draw the same action strip along the bottom of the
  section, built by one function: Build, Deploy…, Rollback…, Refresh, and, while
  the gate verdict is `unchecked`, Build without a dry run. The conditional button
  is last, so its presence moves no other button. Each button is enabled under
  the same conditions in both sections.
- **R1a.** Build without a dry run needs the game, emm and the vanilla copy, as
  Build does; only the gate is skipped.
- **R2.** The Build section's toolbar under the heading goes away. Its Cancel
  button is not carried over: a build runs under `BusyCancellable`, whose modal
  popup already has a Cancel and covers the toolbar while a build runs.
- **R3.** Build is `Border(heading, strip, nil, nil, VSplit(...))`: steps and log
  on top, the Last build report below, under a divider keyed `"report"`. The
  report table fills its pane instead of sitting in a fixed 180 px region. The
  log pane's height becomes its minimum. With no build yet, the lower pane says so.
- **R4.** The compatibility check moves out of the Game card into its own card,
  Compatibility, between Vanilla copy and Deployed, so the cards read in the order
  a new player does them.
- **R5.** When the check cannot run yet, its button stays (disabled) and a note
  under it names what is missing: emm, the vanilla copy, or both, and that they
  are above.

## Acceptance Criteria

- [x] AC1: Overview and Build each show the strip buttons in the same order, and
  none of them is inside a scroller (`fynetest.ScrolledButtons`). Test.
- [x] AC2: Build without a dry run appears in both sections when the verdict is
  `unchecked` and in neither otherwise, and is the last strip button. Test.
- [x] AC2a: Build without a dry run is disabled while the vanilla copy is missing.
  Test.
- [x] AC3: The Build section has no Cancel button and no toolbar above the log.
  Test.
- [x] AC4: The Last build table sits inside a split in the Build section, and a
  position dragged on one build of the section is the position of the next. Test.
- [x] AC5: The compatibility-check button comes after the Vanilla copy card and
  is not in the Game card. Test.
- [x] AC6: With emm missing, the vanilla copy missing, or both, the button is
  disabled and the note names exactly what is missing; with both present the
  button is enabled and no note is drawn. Test.
- [x] AC7: `make test` and `make lint` pass.
- [ ] AC8: Checked in the running window (see E2E Test Plan).

## Risks & Assumptions

- Assumes that drawing the Build-only "Build without a dry run" on Overview too
  is wanted; the alternative breaks "same strip in both".
- Overview's Build button switched to the Build section before starting; the
  shared strip keeps that, and on Build the switch is a no-op.
- Build's Build button was previously enabled whenever no work ran and left the
  refusal to core; it now uses Overview's conditions (game, emm, vanilla copy,
  buildable gate). Core still refuses on its own.
- Rollback: revert the merge commit. No data or settings format changes; the new
  split position is one more key in the shell's existing splits store.

## E2E Test Plan

Environment: this machine, `make install`, the game installed and deployed.

1. Open Overview, then Build: Build, Deploy…, Rollback…, Refresh sit in the same
   place along the bottom of both. (AC1)
2. On Build, drag the divider above Last build up; switch to Overview and back,
   then quit and reopen: the divider is where it was left. (AC4)
3. Start a build: the popup's Cancel stops it; no toolbar Cancel exists. (AC3)
4. Overview reads Game, emm, Vanilla copy, Compatibility, Deployed. (AC5) The
   disabled-button note (AC6) needs a machine without emm or a vanilla copy and is
   covered by the test, not by hand.

### Prod visibility

Not applicable: a desktop program with no telemetry.
