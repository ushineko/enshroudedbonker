#!/usr/bin/env python3
"""Read-only: RIP-relative references to named strings in the running game's code. Stores nothing."""
import re, struct, sys
pid = int(sys.argv[1]); BASE = 0x140000000
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
def rd(a, n):
    mem.seek(a); return mem.read(n)
text = rd(BASE + 0x1000, 0x13ce4a0); rdata = rd(BASE + 0x13d0000, 0xc0943e)
print("text head", text[:16].hex(), "density of 0xE8", text.count(b"\xe8") / len(text))
tg = {}
for w in sys.argv[2:]:
    for m in re.finditer(b"\0" + re.escape(w.encode()) + b"\0", rdata):
        tg[0x13d0000 + m.start() + 1] = w
print("targets", len(tg))
out = {}
for m in re.finditer(rb"[\x48\x4c]\x8d[\x05\x0d\x15\x1d\x25\x2d\x35\x3d]", text):
    o = m.start(); dest = 0x1000 + o + 7 + struct.unpack_from("<i", text, o + 3)[0]
    if dest in tg: out.setdefault(tg[dest], []).append(BASE + 0x1000 + o)
for w, a in sorted(out.items()): print(f"{w:36}", " ".join(hex(x) for x in a[:8]))
print("no refs:", sorted(set(tg.values()) - set(out)))
