# Spec 001: Feasibility recon

**Status**: IN PROGRESS (Q3 needs a live run)

## Context

Decide whether an Enshrouded tool can be built in the style of nmsbonker (data
patching) and terrariabonker (live memory). It needs a schema gate that checks
the installed build before anything is written. Evidence lives in
[docs/findings.md](../docs/findings.md).

## Questions and acceptance criteria

### Q1. Schema from the exe, statically
- [x] Find out whether the Steam DRM wrapper blocks static reads of `.text` and `.rdata`. (Result: it doesn't.)
- [x] Reflection tables are static data, with a known descriptor layout
- [x] Full type set extractable on Linux, without running the game (12,646 types)
- [x] A per-type schema digest exists (`internalHash`)

### Q2. Archive patching
- [x] Read the `KFC4` index and decode every resource with 0 failures
- [x] Unchanged repack is semantically identical. (It is not byte-identical: it appends.)
- [x] A targeted patch (`ItemInfo.maxStackSize`) changes only that field
- [x] Archive and exe share a build fingerprint
- [ ] Patched archive loads in game and shows the new stack sizes (needs the game)

### Q3. Live memory
- [ ] `/proc/<pid>/mem` readable under Proton on the host (root if `ptrace_scope` ≥ 1)
- [ ] Fixed allocator blocks present at the logged bases
- [ ] Decoded `ItemInfo` records found in memory, and in which block

## Out of scope

Writing to the live game or to the real install. Every test here runs on
copies or reads only.

## Decision (2026-10-18)

Use upstream kfc-parser/EML (`emm`) as an external tool, the way nmsbonker
uses MBINCompiler. Do not reimplement the archive format, the serializer or
the EML Lua API. Upstream is under active development and is the expert on
the format; diverging from it costs more than it saves. EML Lua mods are the
mod format, including our own tweaks. Native code is limited to what EML
doesn't do: build fingerprint and schema gate, fetching and pinning `emm`,
mod enable/order, pristine backup, deploy/rollback, re-patch after game
updates, and a per-field conflict report.
