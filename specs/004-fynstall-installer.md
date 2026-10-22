# Spec 004 — Install without a toolchain: a fynstall installer

**Status**: COMPLETE
**Issue**: #5

## Context

The only way to install enshroudedbonker is `install.sh` from a checkout. It
builds both binaries from source, so a player needs Go and, for the window, a C
toolchain plus the OpenGL and X11/Wayland headers. `make release` produces
tarballs, but no Release has been published and the tarball still leaves the
player to copy files by hand. This program is for players, not developers. Spec
003 removed the Rust toolchain from a player's setup, and this spec removes Go
and the C toolchain.

[fynstall](https://github.com/ushineko/fynstall) (v0.2.5)
builds a self-contained installer from a `fynstall.yaml`, with a Fyne wizard and
a command-line variant. terrariabonker already ships one (its spec 053, with
`fynstall.yaml` and a pinned `FYNSTALL_VERSION` in the Makefile), and this spec
follows that design unless a point below says otherwise.

What an install writes on Linux, per user (fynstall's `docs/config.md` and
`docs/platforms.md`):

| | Path |
| --- | --- |
| Programs | `~/.local/share/io.ushineko.enshroudedbonker/bin/` (`{programs}/{id}`) |
| On `PATH` | symlinks in `~/.local/bin` |
| Launcher | `~/.local/share/applications/io.ushineko.enshroudedbonker.desktop` |
| Icon | `~/.local/share/icons/hicolor/<size>/apps/io.ushineko.enshroudedbonker.png` |
| Removal | `uninstall` in the install directory |

## Requirements

Publishing the installers on a GitHub Release is out of scope here and follows
this spec.

- **R1. `fynstall.yaml`** at the repository root, Linux only for now (the program
  does not run on Windows yet; the project overview says Windows comes later).
  It describes a per-user install (`scopes: [user]`) of:
  - `enshroudedbonker` and `enshroudedbonker-gui` under `bin/`, both linked onto
    `PATH`;
  - `README.md` and `LICENSE` under `share/doc/`;
  - one desktop entry whose ID is the app ID, matching the window's Wayland
    `app_id` (`internal/gui` `appID`), with the same name, comment, categories
    and keywords as `packaging/io.ushineko.enshroudedbonker.desktop`;
  - `ui.launch` set to the window, so the finish page offers to start it.
- **R2. The player's data is never touched.** `keep_on_uninstall` names
  `{config}/enshroudedbonker` and `{data}/enshroudedbonker`. The data directory
  holds the mod library, the downloaded `emm.exe` and its Wine prefix, the build
  workspace, the deploy archive and the vanilla copies, which a rollback needs.
  An uninstall or upgrade that removed it could leave a player unable to restore
  the game's archive. The install directory `{data}/io.ushineko.enshroudedbonker`
  is a sibling of `{data}/enshroudedbonker`, not a parent, so the two do not
  overlap (confirmed by AC4).
- **R3. Icon.** fynstall needs a square PNG of at least 512 px. The repository has
  only `packaging/enshroudedbonker.svg`. A 512 px PNG rendered from the SVG is
  committed beside it (it is the project's own art, not a game asset), with a
  note in the Makefile or `packaging/` saying how it was rendered, so the two can
  be kept in step.
- **R4. `make installer`** builds both binaries, installs the pinned fynstall on
  first use (`FYNSTALL_VERSION` in the Makefile, the way the linter is pinned, not
  in `go.mod`), runs `fynstall validate`, and builds the wizard installer and the
  `-cli-installer` into `dist/` (`--with-cli-only`).
- **R5. Version held together.** `fynstall.yaml`'s `app.version` is a literal. A
  test fails when it differs from `VERSION`. `VERSION` is `0.0.1-alpha`, which
  fynstall's `1.2.3-rc.1` form appears to accept; `fynstall validate` confirms it.
- **R6. `install.sh`, `uninstall.sh`, `make install` and `make release` stay.**
  They are the developer path. Where an `install.sh` install and an installer
  install meet on one machine, the result is predictable (see Risks).
- **R7. README.** The player-facing install section leads with the installer:
  where to download it, the wizard and the command-line variant, upgrading,
  uninstalling, and where the player's data stays. `install.sh` moves to a
  "from source" section.

## Acceptance Criteria

- [x] AC1. `fynstall validate` passes on `fynstall.yaml`. It accepts
  `0.0.1-alpha`.
- [x] AC2. `make installer` on Linux leaves `…-linux-amd64-installer` and
  `…-linux-amd64-cli-installer` in `dist/`, built by the pinned fynstall, and
  each reports the version in `VERSION`. Both say
  `enshroudedbonker 0.0.1-alpha installer (fynstall 0.2.5)`, and each comes with
  an uninstaller.
- [x] AC3. The command-line installer's `--dry-run` lists exactly the files in
  R1, the two `PATH` links, the desktop entry and the icon sizes, and nothing
  under `~/.config/enshroudedbonker` or `~/.local/share/enshroudedbonker`.
  It lists 13 files: the four payload files, `uninstall`, seven icon sizes and the
  entry, plus fynstall's receipt and index record.
- [x] AC4. A real per-user install and uninstall on this machine, each checked
  against what it said it would write. After the uninstall, a populated
  `~/.local/share/enshroudedbonker` (library, tools, deploy archive) and
  `~/.config/enshroudedbonker` are byte-identical to before.
  Done in a throwaway `HOME` with a populated data and config tree: after
  install and uninstall, and after install, a same-version repair and
  `--uninstall`, every path under `~/.local` and `~/.config` matches the
  snapshot from before (type, size, mtime, mode, hash). The installed entry
  passes `desktop-file-validate`, and both binaries run through their links.
  On the maintainer's real home (2026-10-21), the wizard install and the
  launcher entry's Uninstall changed nothing outside the install's own files,
  except what using the window wrote (a build in the workspace, the emm Wine
  prefix, `config.json`).
- [x] AC5. The window started from the installed launcher entry shows the
  enshroudedbonker icon on KDE Wayland, and `enshroudedbonker status` works by
  name from a new terminal. Desk-checked by the maintainer with the wizard
  installer, 2026-10-21.
- [x] AC6. An installer install over an existing `install.sh` install, then the
  uninstaller: the `install.sh` files it replaced are put back, or the README
  says what to run first. Measured, not assumed.
  Installer over `install.sh`: the installer saves the CLI, the window and the
  entry, and its uninstaller restores them byte-identical. The reverse order
  fails: with `install.sh` run over an installer install, the installer's
  uninstaller deletes the `install.sh` binaries and entry, because it removes
  whatever is at the paths it created. That is a fynstall gap (see Risks). The
  README says to remove one before installing the other.
- [x] AC7. The version test fails when `fynstall.yaml` and `VERSION` differ
  (mutant killed: `0.0.2-alpha` in `fynstall.yaml` fails it).
- [x] AC8. The README's install section covers the installer, upgrade, removal
  and where the data stays.
- [x] AC9. `make test` and `make lint` pass; the CLI still builds with
  `CGO_ENABLED=0`.

## Risks & Assumptions

- **Rollback**: revert the commit. Nothing in the program changes at run time;
  the installer is a new artifact beside the existing install paths.
- **`install.sh` and the installer collide on names.** `install.sh` writes real
  files to `~/.local/bin/enshroudedbonker{,-gui}` and the desktop entry at the
  same path fynstall uses. fynstall's documentation says it saves what it
  replaces and restores it on uninstall; AC6 checks that. The icons differ:
  `install.sh` installs `enshroudedbonker.svg` and its entry says
  `Icon=enshroudedbonker`, while fynstall installs PNGs named after the app ID.
- **fynstall's uninstaller removes a path it created even when something else
  has replaced it since.** Measured with fynstall v0.2.5 (AC6). The fix belongs in
  fynstall: remove a created link only while it still points at the install,
  and a created file only while its hash matches the receipt. Until then the
  README tells players not to mix the two install methods.
- **The wizard installer needs cgo and the graphics libraries to start.** A
  machine without them runs the `-cli-installer`. Players who run the game have
  a desktop, so the wizard is the default download.
- **The installers are not signed.**
- **No Release pipeline exists.** The project CLAUDE.md describes a Release job
  in `.github/workflows/build.yml`, but the repository has no workflow. Publishing
  the installers on a Release is out of scope here and left for a spec that adds
  the workflow; this spec ends with the installers in `dist/`.
- **Assumed: one fynstall version for build and runtime.** An installer is pinned
  to the fynstall that built it, so `FYNSTALL_VERSION` must be a released tag
  (v0.2.5), not a checkout.

## E2E Test Plan

On this machine (CachyOS, KDE Wayland), with the game installed and a populated
data directory from earlier specs:

1. `make installer`; run each installer with `--version` (AC2).
2. Snapshot `~/.local/bin`, `~/.local/share/applications`, the hicolor icon
   tree, `~/.config/enshroudedbonker` and `~/.local/share/enshroudedbonker`
   (file list, size, mtime, hash).
3. `--dry-run` of the command-line installer, compared with R1 (AC3).
4. Install with the wizard; diff against the snapshot (AC4). Start the window
   from the launcher and check the icon; run `enshroudedbonker status` in a new
   terminal (AC5).
5. Uninstall; diff against the snapshot. The data directories must be
   unchanged (AC4).
6. `./install.sh`, then the installer, then its uninstaller; diff (AC6).

### Prod visibility

Not applicable: a desktop program with no telemetry. A player's report on the
issue tracker is the only signal.
