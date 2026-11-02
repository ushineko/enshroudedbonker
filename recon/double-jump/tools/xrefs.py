#!/usr/bin/env python3
"""Static, read-only: PE sections and RIP-relative references to chosen strings in the exe file."""
import re, struct, sys
d = open(sys.argv[1], "rb").read()
pe = struct.unpack_from("<I", d, 0x3c)[0]; n = struct.unpack_from("<H", d, pe + 6)[0]; opt = struct.unpack_from("<H", d, pe + 20)[0]
base = struct.unpack_from("<Q", d, pe + 24 + 24)[0]; secs = []
for i in range(n):
    o = pe + 24 + opt + i * 40
    nm = d[o:o + 8].rstrip(b"\0").decode("latin1"); vs, va, rs, rp = struct.unpack_from("<IIII", d, o + 8)
    secs.append((nm, va, vs, rp, rs)); print(f"{nm:10} va={va:#x} vsize={vs:#x} raw={rp:#x} rawsize={rs:#x}")
print("imagebase", hex(base))
def rva(off):
    for nm, va, vs, rp, rs in secs:
        if rp <= off < rp + rs: return va + off - rp
text = [s for s in secs if s[0] == ".text"][0]
want = sys.argv[2:]
tg = {}
for w in want:
    b = w.encode() + b"\0"
    for m in re.finditer(b"\0" + re.escape(b), d):
        r = rva(m.start() + 1)
        if r: tg[r] = w
print("targets", {hex(k): v for k, v in tg.items()})
t = d[text[3]:text[3] + text[4]]; va = text[1]
for m in re.finditer(rb"[\x48\x4c]\x8d[\x05\x0d\x15\x1d\x25\x2d\x35\x3d]", t):
    o = m.start(); disp = struct.unpack_from("<i", t, o + 3)[0]; dest = va + o + 7 + disp
    if dest in tg: print(f"xref {tg[dest]:34} code_rva={va+o:#x}")
