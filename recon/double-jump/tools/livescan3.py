#!/usr/bin/env python3
"""Read-only: entries holding the sequence GUID, and exact pointers to each copy's head."""
import re, struct, sys
pid = int(sys.argv[1]); CHUNK = 64 << 20
OLDG, NEWG = 0x2000881cfd0, 0x2001cc102d0
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
def read(a, n):
    try:
        mem.seek(a); return mem.read(n)
    except OSError:
        return b""
print("== head of each copy (qwords from GUID-32)")
for nm, g in (("OLD", OLDG), ("NEW", NEWG)):
    b = read(g - 32, 96); print(nm, " ".join(f"{q:016x}" for q in struct.unpack("<12Q", b)))
print("== neighbourhood of runtime GUID hits")
for g in (0x10020dbe178, 0x100210ad488, 0x3004f5329b8, 0x300bd0d4108, 0x2000888c908, 0x2000925a6ec, 0x2001d98849c):
    a = g & ~7; b = read(a - 48, 128)
    print(f"{g:#x}: " + " ".join(f"{q:016x}" for q in struct.unpack("<16Q", b)))
regs = []
for l in open(f"/proc/{pid}/maps"):
    p = l.split(maxsplit=5); lo, hi = (int(x, 16) for x in p[0].split("-")); nm = p[5].strip() if len(p) > 5 else ""
    if p[1][0] == "r" and "p" in p[1] and not nm.startswith("[v") and not nm.endswith((".so", ".dll")) and hi - lo < (8 << 30):
        regs.append((lo, hi))
targets = {}
for nm, g in (("OLD", OLDG), ("NEW", NEWG)):
    for d in range(-0x40, 0x41, 8):
        targets[struct.pack("<Q", g + d)] = (nm, d)
rx = re.compile(b"|".join(re.escape(k) for k in targets))
print("== exact pointers to a copy's head (+-0x40)")
n = 0
for lo, hi in regs:
    a = lo
    while a < hi:
        b = read(a, min(CHUNK + 8, hi - a))
        if not b: break
        for m in rx.finditer(b):
            at = a + m.start()
            if at % 8 == 0 and n < 40:
                n += 1; nm, d = targets[m.group(0)]
                ctx = read(at - 16, 48)
                print(f"  {at:#x} -> {nm}{d:+#x}   " + " ".join(f"{q:016x}" for q in struct.unpack('<6Q', ctx)))
        a += CHUNK
print("total shown", n)
