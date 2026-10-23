# Spec 007 — Publish releases from CI

**Status**: INCOMPLETE
**Issue**: #13

## Context

The README's install section sends players to the Releases page, and spec 004
left "publishing the installers on a GitHub Release" to a follow-up. Nothing has
been published, and the repository has no CI: there is no `.github/` directory.
CLAUDE.md describes a Release job in `.github/workflows/build.yml` (tag checked
against `VERSION`, notes from the changelog) that was never written. It also
mentions an Arch package, which this repository does not have.

Running `make release` to prepare for one found that it fails on `main`: the
Windows CLI cross-build stops with "updates to go.mod needed", because `go.mod`
was never tidied for cobra's Windows-only dependency (`mousetrap`).

The workflow follows terrariabonker's (`build.yml`, its spec 053) for the build and
installer smoke tests, and ototo's for the `VERSION` check and changelog notes.

## Requirements

- **R1.** `.github/workflows/build.yml` runs on pushes to `main`, `v*` tags, PRs
  to `main` and by hand.
- **R2.** A `test` job: `make test` (race detector, parity tag), `make lint`, and
  a `CGO_ENABLED=0` build of the CLI.
- **R3.** A `build` job (Linux): `make release` and `make installer`; the binaries
  start and report their version; the command-line installer runs `--dry-run`,
  then a real per-user install whose CLI reports the built version, then the
  uninstaller leaves nothing behind. It uploads the Linux tarball and both
  installers, not the Windows CLI tarball (the program does not run on Windows).
- **R4.** A tag build passes `VERSION=` to make, so its binaries carry the plain
  version; the smoke test fails if they do not.
- **R5.** A `release` job on `v*` tags, after `test` and `build`: refuses a tag
  that is not `v` + `VERSION`; writes one `SHA256SUMS` for everything attached;
  takes the notes from that version's `### <version>` entry in README.md and
  fails when there is none; publishes with GitHub's generated notes underneath;
  marks a version containing `-` as a prerelease.
- **R6.** `go.mod`/`go.sum` tidied, so `make release` builds every target.
- **R7.** CLAUDE.md describes what the job actually builds (no Arch package). The
  0.0.1-alpha changelog entry covers the tweaks added since it was written.

## Acceptance Criteria

- [x] AC1: locally, `make release VERSION=0.0.1-alpha` and
  `make installer VERSION=0.0.1-alpha` succeed; both installers report
  `enshroudedbonker 0.0.1-alpha installer (fynstall 0.2.5)` and the CLI reports
  `0.0.1-alpha`.
- [x] AC2: the notes step's awk, run on README.md for `0.0.1-alpha`, prints the
  changelog entry.
- [ ] AC3: the workflow passes on the PR (test and build jobs).
- [ ] AC4: pushing tag `v0.0.1-alpha` publishes a prerelease with the Linux
  tarball, both installers and `SHA256SUMS`, and the changelog entry as notes.

## Risks & Assumptions

- **Rollback**: delete the Release and the tag (`gh release delete`, `git push
  --delete origin <tag>`), or revert the workflow commit.
- **Supply chain**: the workflow uses `actions/checkout@v4`, `setup-go@v5`,
  `upload/download-artifact@v4` and `softprops/action-gh-release@v2`, the same
  pins as the sibling repositories. `go mod tidy` adds only indirect requirements
  of existing dependencies (cobra's Windows `mousetrap`, Fyne's web back end) and
  drops unused `go.sum` lines.
- **The runner's installer test** installs into the runner's own home; nothing of
  a player's is involved.

## E2E Test Plan

1. Open the PR; the `test` and `build` jobs pass (AC3).
2. After merge, tag `main` with `v0.0.1-alpha` and push the tag. The `release` job
   publishes; the Release page shows the attachments and notes (AC4).
3. Download the installer from the Release, check it against `SHA256SUMS`, and run
   `--version`.

### Prod visibility

The Release page and the Actions run for the tag.
