#!/usr/bin/env python3
"""Read-only: log when jump sequence/item ids appear or vanish in live writable memory. Never writes."""
import re, struct, sys, time
pid, idfile, secs = int(sys.argv[1]), sys.argv[2], float(sys.argv[3])
ids = {}
for l in open(idfile):
    kind, name, val = l.split()
    ids[struct.pack("<I", int(val))] = f"{kind}:{name}"
rx = re.compile(b"|".join(re.escape(k) for k in ids))
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
regs = []
for l in open(f"/proc/{pid}/maps"):
    p = l.split(maxsplit=5); lo, hi = (int(x, 16) for x in p[0].split("-")); nm = p[5].strip() if len(p) > 5 else ""
    if p[1].startswith("r") and (lo >> 40) in (1, 3) and hi - lo < (4 << 30):
        regs.append((lo, hi))
print("regions", len(regs), "MiB", sum(h - l for l, h in regs) >> 20, flush=True)
CHUNK = 64 << 20
def snap():
    out = {}
    for lo, hi in regs:
        a = lo
        while a < hi:
            try:
                mem.seek(a); b = mem.read(min(CHUNK, hi - a))
            except OSError:
                break
            for m in rx.finditer(b):
                if m.start() % 4 == 0: out[a + m.start()] = ids[m.group(0)]
            a += CHUNK
    return out
t0 = time.time(); prev = snap(); base = set(prev)
print(f"baseline {len(prev)} hits, pass {time.time()-t0:.2f}s", flush=True)
for a, n in sorted(prev.items())[:20]: print(f"  base {a:#x} {n}")
passes = 0; ever = {}
while time.time() - t0 < secs:
    t = time.time(); cur = snap(); passes += 1
    for a in set(cur) - set(prev):
        print(f"{t-t0:7.1f}s + {a:#x} {cur[a]}", flush=True); ever[cur[a]] = ever.get(cur[a], 0) + 1
    for a in set(prev) - set(cur):
        print(f"{t-t0:7.1f}s - {a:#x} {prev[a]}", flush=True)
    prev = cur
print(f"done passes={passes} avg={(time.time()-t0)/max(passes,1):.2f}s appearances={ever}", flush=True)
