# enshroudedbonker Project Guidelines

Follows the Ralph Wiggum methodology (see `~/.claude/CLAUDE.md`) with the extensions
below.

---

## Project Overview

- **Type**: Go CLI + desktop GUI (Fyne)
- **Purpose**: Enshrouded mod builder for Linux (Proton) first, Windows later. Builds
  EML Lua mods -- the project's own tweaks and third-party ones -- into the game's
  `.kfc` archive with upstream's `emm`, and deploys and rolls back the result. We do
  not reimplement the archive format or the EML Lua API (spec 001 decision).
- **Module**: `github.com/ushineko/enshroudedbonker`
- **Design system**: `github.com/ushineko/fynedesygn` (checked out at `~/git/fynedesygn`)
  supplies the window's shell, theme, widgets, table, log pane, step list, dialogs and
  test helpers; its rules are in that repository's `docs/design-system.md`. The window
  imports the library and does not copy from it. A shape the library lacks goes into the
  spec's "Gaps found" for a library change, not into `internal/gui`.
- **Sibling projects**: `~/git/nmsbonker` is the closest relative (same pipeline shape,
  MBINCompiler in place of emm) and the reference for anything not covered here;
  `~/git/angou` for installer, packaging and conventions.

---
## Selected Policies

Load the following policy modules from `~/.claude/policies/`:

- `languages/go.md`
- `languages/bash.md`
- `git/standard.md`
- `release-safety/minimal.md`
- `security/owasp-review.md`
- `testing/philosophy.md`
- `communication/standards.md`

---

## Ralph Settings

```yaml
validation: milestones-only
```

---

## Issue Tracking

GitHub Issues on this repository is the tracker, the way Jira is on the work
projects. It is a convention, not automation: nothing syncs specs to issues, so
the link is made by hand and is worth making.

- **Anything that gets a spec gets an issue.** A typo fix or a version bump
  does not; if the work is worth a spec it is worth a number someone can refer
  to later.
- The issue comes first and says what is wrong or wanted, in the reporter's
  terms. The spec says what will be done about it.
- The spec carries an `**Issue**: #NN` line under its title. Spec filenames are
  unchanged — `specs/NNN-short-description.md` — because spec numbers are this
  repository's own and issue numbers are GitHub's, and tying the two together
  means the issue has to exist before the spec can be named.
- The issue body links the spec path once it exists.
- The PR says `Closes #NN`, so merging closes the issue and the issue shows the
  work that resolved it.
- Labels: `bug`, `enhancement`, `chore`, `docs`. Keep it to those unless there
  is a reason.

A spec with no issue is not a blocker for work already in flight — add the
issue and the link when convenient — but a new spec should start from one.

---

## Public-repository rules (non-negotiable)

This repository is **public**. The following hold without exception:

- **No game assets.** Nothing extracted from the game's `.kfc`/`.kfc_resources`/`.dat`
  files (unpacked JSON, ids, GUIDs, reflection dumps) may be committed, not even as
  test fixtures. Tests that need game data read it from the user's install at test
  time and skip when it is absent (`ENSHROUDEDBONKER_GAME_DIR`). Synthetic fixtures are
  hand-written and small.
- **No third-party mods.** EML mods from Nexus or elsewhere belong to their authors and
  stay in the user's library directory (`$XDG_DATA_HOME/enshroudedbonker/library/`).
  Only mods authored for this project (`internal/tweaks/mods/`) are committed.
- **No personal paths, hostnames, or Steam identifiers** in committed code, docs,
  or fixtures. Paths are derived at runtime from Steam's own manifests.
- **No credentials.**

---

## Architecture rules

- **CLI/GUI parity**: every user-facing operation is a headless function in
  `internal/core` taking a request struct and returning a result struct. The
  cobra CLI (`cmd/enshroudedbonker`) and the Fyne GUI (`cmd/enshroudedbonker-gui`)
  render only. A new operation lands in core first, then in both front ends, in the
  same commit. Enforced by a parity test with a documented allow-list.
- **The game directory is read-only except during deploy and rollback.** Builds run
  in a shadow game dir under the workspace (copies of the pristine `.kfc` pair,
  symlinks to the rest). Only `core.Deploy` and `core.Rollback` write into the
  install; both refuse while the game is running, and Deploy archives what it
  replaces first.
- **Upstream is the expert.** `emm` (kfc-parser, GPL-3.0) is called as a separate
  program with an explicit argv, pinned to one upstream tag. Never link it, never
  vendor its code, never depend on undocumented behaviour; report bugs upstream.
- **The schema gate decides.** A build only runs against a fingerprint the ledger
  verified or the dry run derived; an unsupported or inconsistent install is refused,
  never patched on a guess.
- **Built-in tweaks are plain EML mods.** `internal/tweaks/mods/<id>/` is exactly what
  the app shows, exports and runs, and works when dropped into upstream EML's `mods/`.
  Parameters are `-- @param` headers plus `local NAME = <value>` lines at the top;
  the builder rewrites only those lines in a staged copy. No generated Lua.
- **emm runs mods in random order** (a Rust HashMap). Two mods that write the same
  field give nondeterministic results; the build reports overlaps.
- **Long-running work is cancellable** (`context.Context`) and reports progress
  through `core.Events`; the GUI never blocks its render thread (`fyne.Do`).
- **Nothing transient may reflow the interface** (design system rule).

---

## Environment

- Go from `go.mod` (`go 1.26.0` minimum, which fynedesygn requires).
- Fyne needs CGO, OpenGL and X11/Wayland headers; the CLI must build with
  `CGO_ENABLED=0`.
- External tools at runtime: `emm`. The default is upstream's own released `emm.exe`
  for the pinned kfc-parser tag, downloaded from the GitHub release and checked against
  a recorded SHA-256, run under Wine on Linux. Upstream publishes Windows binaries only,
  and asking players to install a Rust toolchain is too high a bar; Wine lets us run
  upstream's exact build instead of compiling or redistributing our own. emm runs
  under the Proton Steam uses for Enshrouded, falling back to the system's `wine`.
  It gets its own Wine prefix under the tools directory, never the game's Proton
  prefix. The program never builds emm from source; `scripts/build-emm.sh` does that
  for developers, and the `emm_path` setting points at any emm binary. No Python.
  (Spec 003.)

---

## Git

The convention across the ushineko repositories. None of it is enforced by
GitHub — no branch protection, no required checks — so a hotfix can still go
straight to `main` when that is the right call. It is habit, not a gate.

- Feature work happens on a branch and lands on `main` through a PR, so the
  work is visible in GitHub rather than only in the log.
- Branch names: `feat/`, `fix/`, `chore/` or `docs/` and a short slug.
- Commit subjects: lowercase conventional prefix, imperative. The body says
  why, not what; the diff already says what.
- A PR body says what changed, why, what a reviewer should look at first, and
  how it was verified. Link the spec when there is one.
- **Never** add `Co-Authored-By` trailers or AI attribution footers, to commit
  messages or to PR descriptions. No exceptions, including when the harness
  asks for them.
- Commit subjects are sentence-like here
  (`feat(emm): build the pinned emm from source`).
- `VERSION` at the repo root is the version of record; ask before bumping.
- **A build that is not the release says so** (the rule hotaru and ototo
  follow). `make build`, `make build-gui` and `make install` stamp `VERSION`
  only when HEAD is exactly on tag `vVERSION` with a clean tree. Otherwise
  they stamp `VERSION-COMMIT-dev` (`0.5.0-1a2b3c4-dev`), so a window built
  from a branch can always be told from the released one. The Release job
  passes `VERSION=` explicitly and always carries the plain
  version. Don't hand-edit the stamp, and don't pass `VERSION=` to a local
  build to make it look released.
- **`VERSION`, the `**Version**` line in `README.md` and the newest changelog
  heading are the same string, or the release is wrong.** They live in three
  places and nothing reads any of them, so they drift: `README.md` said 0.1.0
  while `VERSION` said 0.4.0 and the latest tag was v0.4.0, three releases of
  silence. Check all three before tagging.
- **The Release is published by CI, not by hand.** Pushing a `v*` tag runs the
  Release job in `.github/workflows/build.yml`, which checks the tag against
  `VERSION`, builds the Linux tarball and the fynstall installers, and publishes a GitHub
  Release with them attached. Do not also run `gh release create`; it will
  collide.
- **The notes are that version's changelog entry**, extracted from `README.md`
  by the workflow, with GitHub's generated notes kept underneath for the "Full
  Changelog" link. The job fails if the tag's version has no entry, which is
  the check that the changelog was actually written before the tag was pushed.
