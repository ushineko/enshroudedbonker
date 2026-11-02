#!/usr/bin/env python3
"""Read-only: absolute pointers to named strings across the image and engine data; show neighbours."""
import re, struct, sys
pid = int(sys.argv[1]); BASE = 0x140000000
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
def rd(a, n):
    mem.seek(a); return mem.read(n)
rdata = rd(BASE + 0x13d0000, 0xc0943e)
tg = {}
for w in sys.argv[2:]:
    for m in re.finditer(b"\0" + re.escape(w.encode()) + b"\0", rdata):
        tg[struct.pack("<Q", BASE + 0x13d0000 + m.start() + 1)] = w
rx = re.compile(b"|".join(re.escape(k) for k in tg))
def scan(label, lo, n):
    b = rd(lo, n); c = 0
    for m in rx.finditer(b):
        o = m.start()
        if o % 8: continue
        c += 1
        qs = struct.unpack_from("<8Q", b, max(o - 24, 0))
        print(f"{label} {lo+o:#x} {tg[m.group(0)]:32} " + " ".join(f"{q:x}" for q in qs))
    print(label, "hits", c)
scan("rdata", BASE + 0x13d0000, 0xc09400)
scan("data", BASE + 0x1fda000, 0x8802f0)
