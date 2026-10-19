# enshroudedbonker

Recon for an Enshrouded (Steam, Proton on Linux) mod and trainer tool, in the
spirit of nmsbonker and terrariabonker. The recon scripts only read; the
manual test deploys under `work/` (gitignored) do patch the game, see STATUS.

**Current state: [docs/STATUS.md](docs/STATUS.md).**

Status and evidence: [specs/001-feasibility-recon.md](specs/001-feasibility-recon.md),
[docs/findings.md](docs/findings.md).

## Recon scripts

| Script | What it answers | Needs |
| --- | --- | --- |
| `recon/static_exe.py <exe>` | DRM wrapper, `.text` readability, static reflection tables | `pip install pefile capstone` |
| `recon/kfc_roundtrip.sh <game> <kfc-parser> <work>` | archive round-trip and a ×10 stack patch, on copies | a built [kfc-parser](https://github.com/Brabb3l/kfc-parser) `cli-legacy` |
| `sudo recon/live_probe.py` | memory access, fixed allocator blocks, `ItemInfo` in memory | game running, world loaded |
