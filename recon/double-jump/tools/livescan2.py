#!/usr/bin/env python3
"""Read-only: which copy of the sequence does live data point to? Never writes."""
import re, struct, sys
pid = int(sys.argv[1]); CHUNK = 64 << 20
OLD, NEW = 0x2000881e3f8, 0x2001cc116f8
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
def read(a, n):
    try:
        mem.seek(a); return mem.read(n)
    except OSError:
        return b""
regs = []
for l in open(f"/proc/{pid}/maps"):
    p = l.split(maxsplit=5); lo, hi = (int(x, 16) for x in p[0].split("-")); nm = p[5].strip() if len(p) > 5 else ""
    if p[1][0] == "r" and "p" in p[1] and not nm.startswith("[v") and not nm.endswith((".so", ".dll")) and hi - lo < (8 << 30):
        regs.append((lo, hi, p[1], nm))
# window: 0x40000 either side, by the middle bytes of the pointer
def pat(c):
    mids = {((c + d) >> 16) & 0xffff for d in range(-0x40000, 0x40001, 0x10000)}
    alt = b"|".join(re.escape(struct.pack("<H", m)) for m in sorted(mids))
    return b"(?s)..(?:" + alt + b")" + re.escape(struct.pack("<H", (c >> 32) & 0xffff)) + b"\x00\x00"
rx = {"OLD": (re.compile(pat(OLD)), OLD), "NEW": (re.compile(pat(NEW)), NEW)}
res = {"OLD": [], "NEW": []}
for lo, hi, perms, nm in regs:
    a = lo
    while a < hi:
        b = read(a, min(CHUNK + 8, hi - a))
        if not b: break
        for k, (r, c) in rx.items():
            for m in r.finditer(b):
                at = a + m.start()
                if at % 8: continue
                v = struct.unpack("<Q", m.group(0))[0]
                if abs(v - c) <= 0x40000: res[k].append((at, v - c))
        a += CHUNK
def block(a):
    return {1: "GF_Main", 2: "GF_Resource", 3: "GF_App", 4: "GF_Fallback"}.get(a >> 40, "other")
for k in res:
    hits = sorted(set(res[k])); by = {}
    for at, d in hits: by.setdefault(block(at), []).append((at, d))
    print(f"== {k}: {len(hits)} pointers", {b: len(v) for b, v in by.items()})
    for b, v in by.items():
        ext = [x for x in v if abs(x[0] - (OLD if k == 'OLD' else NEW)) > 0x40000]
        print(f"  {b}: from outside the copy's own window: {len(ext)}")
        for at, d in ext[:14]: print(f"    {at:#x} -> name{d:+#x}")
