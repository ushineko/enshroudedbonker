#!/usr/bin/env python3
"""Read-only live probe of a running Enshrouded (Proton) process.

Answers three questions with the game running and a world loaded:
  1. Can we read the process's memory at all (ptrace_scope / root)?
  2. Are the engine's fixed allocator blocks at the addresses the log reports?
  3. Do decoded ItemInfo records sit in memory, and in which block?

It never writes. Run as root (sudo) when kernel.yama.ptrace_scope >= 1:

    sudo python3 recon/live_probe.py --probes /path/to/probes.json

Probe pattern per item: u32 itemId | 16-byte objectId GUID (Microsoft
mixed-endian, i.e. uuid.bytes_le) | u16 maxStackSize. That is the start of
keen::ItemInfo per the build's reflection data (offsets 0, 4, 20).
"""
import argparse
import json
import os
import re
import struct
import sys
import uuid

# From enshrouded.log, "[memory] Alloc block ..." lines (1.0, build 1269579).
FIXED_BLOCKS = {
    "GF_Main": 0x0000010000000000,
    "GF_Resource": 0x0000020000000000,
    "GF_App": 0x0000030000000000,
    "GF_Fallback": 0x0000040000000000,
}
CHUNK = 64 << 20


def find_pid():
    hits = []
    for d in os.listdir("/proc"):
        if not d.isdigit():
            continue
        try:
            with open(f"/proc/{d}/comm") as f:
                comm = f.read().strip()
        except OSError:
            continue
        # Wine names the process after the PE image; match that, not any
        # command line that merely mentions the file.
        if comm.lower() == "enshrouded.exe":
            hits.append((int(d), comm))
    return hits


def regions(pid):
    with open(f"/proc/{pid}/maps") as f:
        for line in f:
            parts = line.split(maxsplit=5)
            lo, hi = (int(x, 16) for x in parts[0].split("-"))
            perms = parts[1]
            name = parts[5].strip() if len(parts) > 5 else ""
            yield lo, hi, perms, name


def block_of(addr):
    for name, base in FIXED_BLOCKS.items():
        if base <= addr < base + (1 << 40):
            return name
    return "-"


def main():
    ap = argparse.ArgumentParser()
    # The probe list is game-derived (item ids and GUIDs), so it is never
    # committed: generate it from an unpacked ItemInfo set and pass it here.
    ap.add_argument("--probes", required=True,
                    help='JSON list of {"itemId", "objectId", "maxStackSize"} taken from unpacked ItemInfo')
    ap.add_argument("--max-hits", type=int, default=8)
    args = ap.parse_args()

    pids = find_pid()
    if not pids:
        sys.exit("enshrouded.exe is not running")
    pid = sorted(pids)[-1][0]
    print(f"pid {pid}  ({len(pids)} candidate(s): {[p for p, _ in pids]})")
    try:
        scope = open("/proc/sys/kernel/yama/ptrace_scope").read().strip()
    except OSError:
        scope = "n/a (no Yama)"
    print(f"ptrace_scope = {scope}  euid = {os.geteuid()}")

    regs = list(regions(pid))
    print(f"\n[2] fixed allocator blocks ({len(regs)} mappings total)")
    for name, base in FIXED_BLOCKS.items():
        maps = [(lo, hi, p) for lo, hi, p, _ in regs if base <= lo < base + (1 << 40)]
        size = sum(hi - lo for lo, hi, _ in maps)
        first = f"{maps[0][0]:#x} {maps[0][2]}" if maps else "absent"
        print(f"  {name:12} base {base:#016x}: {len(maps)} mapping(s), {size / 2**20:,.0f} MiB, first {first}")

    probes = json.load(open(args.probes))
    pats = {}
    for p in probes:
        g = uuid.UUID(p["objectId"]).bytes_le
        pats[struct.pack("<I", p["itemId"]) + g + struct.pack("<H", p["maxStackSize"])] = p

    print(f"\n[1+3] scanning readable private memory for {len(pats)} ItemInfo probes ...")
    try:
        mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
    except PermissionError as e:
        sys.exit(f"cannot open /proc/{pid}/mem: {e} (run with sudo)")

    rx = re.compile(b"|".join(re.escape(k) for k in pats))
    hits, scanned, unreadable = [], 0, 0
    for lo, hi, perms, name in regs:
        if perms[0] != "r" or name.startswith("[v") or (name and "/" in name and name.endswith((".so", ".dll", ".exe"))):
            continue
        addr = lo
        while addr < hi:
            n = min(CHUNK + 32, hi - addr)
            try:
                mem.seek(addr)
                buf = mem.read(n)
            except OSError:
                unreadable += 1
                break
            for m in rx.finditer(buf):
                hits.append((addr + m.start(), pats[m.group(0)], perms, name))
            scanned += len(buf)
            addr += CHUNK
    print(f"  scanned {scanned / 2**30:.2f} GiB, {unreadable} unreadable region(s)")
    if not scanned:
        sys.exit("  read nothing: memory access is blocked")

    print(f"  {len(hits)} hit(s)")
    for addr, p, perms, name in hits[: args.max_hits]:
        print(f"    {addr:#016x} {perms} block={block_of(addr):11} item {p['itemId']} max {p['maxStackSize']} {name}")
    found = {p["itemId"] for _, p, _, _ in hits}
    print(f"  probes found: {len(found)}/{len(pats)}")
    print("\nverdict:", "LIVE ItemInfo records located" if found else "no ItemInfo records found (load a world and retry)")


if __name__ == "__main__":
    main()
