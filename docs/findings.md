# Recon findings

Evidence for [spec 001](../specs/001-feasibility-recon.md). Build under test:
Enshrouded 1.0, `1269579|^/game38/branches/release_1.0|2026-10-16T12:58:37Z`
(git `ef984944dc33`), Windows x64 build run under Proton. Files dated 2026-10-17.

## Q1. Can the schema be read statically from the on-disk exe?

**Yes.**

- **Steam DRM wrapper present, but `.text` is not encrypted.** The entry point
  is in `.bind` (rva `0x29e0310`; section entropy 7.96, i.e. packed stub). But
  `.text` has entropy 6.40 (normal for x64 code), and 299 of 300 randomly
  sampled `.pdata` functions disassemble cleanly. `.rdata` is untouched. So no
  Steamless step is needed for static work. (`recon/static_exe.py`)
- **Reflection tables are static `.rdata` data.** The string `maxStackSize` is
  referenced by 4 absolute pointers, all in `.rdata`, and by no `lea` in code.
  So types are not registered by running code; they sit in tables.
  - Field descriptor, stride `0x30`: `{name*, name_len, type*, data_offset, …}`.
    For example `keen::ds::StackSizeRange` has `minStackSize` at 0 and
    `maxStackSize` at 2.
  - Type descriptor: `{qualified_name*, len, …, size/align, field_count, hash,
    fields*}`.
- **kfc-parser extracts all of it on Linux, from the file, in 0.14 s.** That's
  12,646 types: 8,373 structs, 1,177 typedefs, 826 enums, 654 blob arrays,
  637 object references, plus 23 smaller categories. The README's "Windows x64
  only" note refers to the target binary, not the host.
- **There are two families of every type.** `keen::X` is the serialized
  (blob) form, with `BlobArray` and `BlobString`. `keen::ds::X` is the runtime
  form, with `DsArray`. Both carry the same `internalHash`, which acts as a
  per-type schema digest that does not depend on the form. Field offsets can
  differ between forms: `LootableItemInfo.stackSizeMax` is at 26 in the blob
  form and 90 in the runtime form. `ItemInfo.maxStackSize` is at 20 in both.

## Q2. Can the archive be patched safely, and does it round-trip?

**Yes, at the data level. Not byte-for-byte.**

- **Container.** `enshrouded.kfc` is the index (`KFC4` magic, 9.4 MB). It
  covers 91,812 resources, 146 resource types and 55 content `.dat` files.
  `enshrouded.kfc_resources` holds the resource bodies in zstd chunks.
  - Reflected data such as `ItemInfo` needs only the `.kfc` and
    `.kfc_resources` files. The 49 GB of `.dat` files are bulk content
    (meshes, textures).
- **The archive carries its build version.** The `.kfc` header embeds the
  same `1269579|…release_1.0|…` string as the exe. The header also keys its
  resource bundles by `Type::internal_hash`. So the archive declares the
  schema of every type it contains. That is the hook for a schema gate.
- **kfc-parser already supports 1.0.** The magic changed `KFC3` → `KFC4` on
  2026-10-16, the day before the build. v0.2.0 was tagged 2026-10-17. It
  unpacks all 91,812/91,812 resources with 0 failures in 56 s.
- **Unchanged repack.** Re-packing all 4,262 `ItemInfo` records unchanged
  leaves the original `.kfc_resources` bytes untouched and appends 1,085,440
  bytes. Unpacking the result gives JSON identical to the original for every
  record.
- **Patch test.** `maxStackSize` ×10 (capped at 65,535) on the 2,267 stackable
  items. After repack and unpack, 4,262/4,262 records have the expected value
  and 0 records have any other field changed.
- **Caveat: repack is not deterministic.** Two identical repacks give
  different hashes, probably because of thread ordering. A port should write
  deterministically so builds can be cached and compared.
- **Current stack sizes.** Stack size 1 accounts for 1,995 items, 50 for 941,
  20 for 586, 250 for 244, 100 for 109, 500 for 103, 10 for 101 and 5000
  for 76.

(`recon/kfc_roundtrip.sh`)

## Q3. Live memory

**Open; needs the game running.**

- **Not yet established on the live process.** The `[memory] Alloc block`
  lines in the log place the engine's allocator blocks at fixed bases
  (`0x010000000000`, `0x020000000000`, `0x030000000000`, `0x040000000000`).
  `recon/live_probe.py` checks those against `/proc/<pid>/maps`. It then
  searches memory for six real `ItemInfo` record heads: `u32 itemId`, the GUID
  in Microsoft byte order, then `u16 maxStackSize`.
- **The probe pattern is confirmed on disk.** It matches a record in an
  uncompressed chunk of `.kfc_resources`; most chunks are zstd-compressed.
  Memory holds the decompressed form, so all six probes should be findable.
- **The scanner works.** It finds a planted record in a stand-in process.
- **The cloud VM can't do this step.** The linked shell is a VM with its own
  kernel, so it cannot see the game's processes. The probe must run on the
  host:

      sudo python3 recon/live_probe.py

## Licensing note

kfc-parser is GPL-3.0-or-later. Reusing its code in a Go reader makes that
reader GPL. The format docs in `kfc-parser/docs/src/game/kfc` and the
reflection layout above are facts, so a clean-room reimplementation from them
is an option if the project needs a different license.

## EML (Lua mods)

- EML mods (`mod.json` + `src/mod.lua`) run **before the game starts**. Each mod
  edits typed resources through `game.assets`, then EML writes the archive and
  keeps a `.kfc.bak`. Runtime mods are still a work in progress upstream, so
  the model is AMUMSS, not runtime hooks.
- `emm` (kfc-parser `mod-manager-cli`) builds on Linux, and
  `emm run -g <dir> --patch` works offline with no DLL proxy or Proton. I ran
  `recon/eml/stack_x10` against 1.0 on a copy of the install: it took 1.1 s
  and wrote 2,267 patches. After unpacking, 4,262 of 4,262 records were
  correct and no other field had changed. Only the changed records were
  appended (+467 KB).
- EML runs on Lua 5.4. The official example uses `//`, and the API exposes
  64-bit values, so gopher-lua (Lua 5.1, numbers are floats) can't host EML
  mods. A pure-Go Lua 5.4 exists (arnodel/golua) but is untested here.
- No `emm` release targets 1.0 yet; upstream points to CI build artifacts.

## Later findings

The in-game test session of 2026-10-19 (eight tweaks, build 7) is recorded in
[STATUS.md](STATUS.md) and spec 002 R5.4.

The character save format (`character0.ksd`: header, chunk layout, inventory
records, and the equipped backpack's row count) was decoded on 2026-10-29 for
spec 010, the shelved backpack tweak. It is written up in
[spec 010, "Save-format findings"](../specs/010-backpack-slots.md#save-format-findings-2026-10-29).
