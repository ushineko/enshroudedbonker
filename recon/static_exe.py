#!/usr/bin/env python3
"""Static recon of enshrouded.exe: Steam DRM wrapper and reflection tables.

    python3 -I recon/static_exe.py <path/to/enshrouded.exe>

Needs: pip install pefile capstone
Answers:
  - Is the entry point in .bind (SteamStub), and is .text encrypted on disk?
  - Are reflection field descriptors static .rdata tables (readable without
    running the game)?
"""
import collections
import math
import random
import re
import struct
import sys

import capstone
import pefile


def entropy(b):
    if not b:
        return 0.0
    c, n = collections.Counter(b), len(b)
    return -sum(v / n * math.log2(v / n) for v in c.values())


def main(path):
    pe = pefile.PE(path)
    ib = pe.OPTIONAL_HEADER.ImageBase
    ep = pe.OPTIONAL_HEADER.AddressOfEntryPoint
    img = pe.get_memory_mapped_image()
    secs = {}
    print(f"image base {ib:#x}, entry point rva {ep:#x}")
    for s in pe.sections:
        name = s.Name.rstrip(b"\0").decode()
        lo, hi = s.VirtualAddress, s.VirtualAddress + s.Misc_VirtualSize
        secs[name] = (lo, hi)
        mark = "  <- entry point" if lo <= ep < hi else ""
        print(f"  {name:8} rva {lo:#09x} vsize {hi - lo:#09x} entropy {entropy(s.get_data()[:4 << 20]):.2f}{mark}")

    # .text decryption check: disassemble functions listed in .pdata.
    lo, hi = secs[".pdata"]
    pd = img[lo:hi]
    funcs = [struct.unpack_from("<III", pd, i) for i in range(0, len(pd) - 12, 12)]
    funcs = [f for f in funcs if f[0]]
    md = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64)
    random.seed(1)
    sample = random.sample(funcs, 300)
    bad = 0
    for b, e, _ in sample:
        code = img[b:e]
        if sum(i.size for i in md.disasm(code, ib + b)) < 0.9 * len(code):
            bad += 1
    print(f"\n.pdata functions {len(funcs)}; {bad}/{len(sample)} sampled fail to disassemble "
          f"({'.text is plaintext on disk' if bad < 10 else '.text looks ENCRYPTED'})")

    # Reflection: who points at the field-name string?
    print()
    for word in (b"maxStackSize\0", b"keen::ItemInfo\0"):
        for m in re.finditer(re.escape(word), img):
            ptr = struct.pack("<Q", ib + m.start())
            refs = [r.start() for r in re.finditer(re.escape(ptr), img)]
            where = collections.Counter(n for r in refs for n, (a, z) in secs.items() if a <= r < z)
            print(f"  {word[:-1].decode():16} @ rva {m.start():#x}: {len(refs)} absolute pointer(s) in {dict(where)}")


if __name__ == "__main__":
    main(sys.argv[1])
