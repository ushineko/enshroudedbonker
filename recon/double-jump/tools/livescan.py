#!/usr/bin/env python3
"""Read-only scan of the running game for the edited double-jump sequence. Never writes."""
import re, struct, sys, uuid
pid = int(sys.argv[1])
CHUNK = 64 << 20
names = {b"Ability_Movement_Skill_JumpDoubleSecond": "Second", b"Ability_Movement_Skill_JumpDoubleFirst": "First", b"Ability_Movement_JumpSingle": "Single"}
guid = uuid.UUID("e7b5dbb5-db37-4efa-ba2e-8e94c3f35fba").bytes_le
F5 = struct.pack("<3f", 5, 5, 5); F1 = struct.pack("<3f", 1, 1, 1)
T500 = struct.pack("<q", 500000000); T450 = struct.pack("<q", 450000009); T1500 = struct.pack("<q", 1500000030); T16 = struct.pack("<q", 16666667)
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
regs = []
for l in open(f"/proc/{pid}/maps"):
    p = l.split(maxsplit=5)
    lo, hi = (int(x, 16) for x in p[0].split("-"))
    nm = p[5].strip() if len(p) > 5 else ""
    if p[1][0] == "r" and "p" in p[1] and not nm.startswith("[v") and not nm.endswith((".so", ".dll")) and hi - lo < (8 << 30):
        regs.append((lo, hi, p[1], nm))
def read(a, n):
    try:
        mem.seek(a); return mem.read(n)
    except OSError:
        return b""
rx = re.compile(b"|".join(re.escape(k) for k in names) + b"|" + re.escape(guid))
hits, scanned = [], 0
for lo, hi, perms, nm in regs:
    a = lo
    while a < hi:
        n = min(CHUNK + 64, hi - a); b = read(a, n)
        if not b: break
        scanned += len(b)
        for m in rx.finditer(b):
            hits.append((a + m.start(), m.group(0), nm))
        a += CHUNK
print(f"scanned {scanned/2**30:.2f} GiB, hits {len(hits)}")
seen = set()
for addr, what, nm in sorted(hits):
    if addr in seen: continue
    seen.add(addr)
    label = names.get(what, "GUID")
    lo = max(0, addr - 32768); w = read(lo, 65536)
    def offs(pat):
        return [m.start() + lo - addr for m in re.finditer(re.escape(pat), w)]
    o5, o1, t5, t45, t15, t16 = offs(F5), offs(F1), offs(T500), offs(T450), offs(T1500), offs(T16)
    print(f"{addr:#x} {label:6} {nm[-28:]:28} 5,5,5={o5[:4]} 1,1,1 n={len(o1)} t500ms={t5[:4]} t450={t45[:3]} t1500={t15[:3]} t16={t16[:3]}")
