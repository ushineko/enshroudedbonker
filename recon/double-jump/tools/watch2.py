#!/usr/bin/env python3
"""Read-only: find active memory blocks, then fast-watch them for jump sequence ids. Never writes."""
import re, struct, sys, time, zlib
pid, idfile, secs = int(sys.argv[1]), sys.argv[2], float(sys.argv[3])
BLOCK = 256 << 10
mem = open(f"/proc/{pid}/mem", "rb", buffering=0)
def read(a, n):
    try:
        mem.seek(a); return mem.read(n)
    except OSError:
        return b""
regs = []
for l in open(f"/proc/{pid}/maps"):
    p = l.split(maxsplit=5); lo, hi = (int(x, 16) for x in p[0].split("-"))
    if p[1].startswith("rw") and (lo >> 40) in (1, 3) and hi - lo < (4 << 30):
        regs.append((lo, hi))
seqs, items = {}, {}
for l in open(idfile):
    k, n, v = l.split()
    (seqs if k == "seq" else items)[int(v)] = n
# runtime ids: the sequence table holds "rid, ptr, addr, 1<<32, rid, hash"
hrx = re.compile(b"|".join(re.escape(struct.pack("<I", v)) for v in seqs))
rids = {}
for lo, hi in regs:
    for a in range(lo, hi, 64 << 20):
        b = read(a, min(64 << 20, hi - a))
        for m in hrx.finditer(b):
            o = m.start()
            if o % 8 == 0 and o >= 8:
                q = struct.unpack_from("<Q", b, o - 8)[0]
                r = struct.unpack_from("<Q", b, o - 32)[0] if o >= 32 else None
                if q and q == r and q < 1 << 24:
                    rids[q] = seqs[struct.unpack_from("<I", b, o)[0]]
print("runtime ids", {hex(k): v for k, v in rids.items()}, flush=True)
want = {struct.pack("<I", v): "seq:" + n for v, n in seqs.items()}
want.update({struct.pack("<I", v): "item:" + n for v, n in items.items()})
want.update({struct.pack("<I", v): "rid:" + n for v, n in rids.items()})
rx = re.compile(b"|".join(re.escape(k) for k in want))
crcs = {}; active = set()
t0 = time.time()
for i in range(3):
    for lo, hi in regs:
        for a in range(lo, hi, BLOCK):
            c = zlib.crc32(read(a, BLOCK))
            if a in crcs and crcs[a] != c: active.add(a)
            crcs[a] = c
    print(f"pass {i} t={time.time()-t0:.1f}s active={len(active)*BLOCK>>20} MiB", flush=True)
def refresh():
    for lo, hi in regs:
        for a in range(lo, hi, BLOCK):
            c = zlib.crc32(read(a, BLOCK))
            if crcs.get(a) != c: active.add(a)
            crcs[a] = c
def snap():
    out = {}
    for a in sorted(active):
        b = read(a, BLOCK)
        for m in rx.finditer(b):
            o = m.start()
            if o % 4 == 0:
                out[a + o] = (want[m.group(0)], b[o + 4:o + 24])
    return out
prev = snap(); print(f"watch baseline {len(prev)} hits", flush=True)
for a, (nm, tail) in sorted(prev.items()): print(f"  base {a:#x} {nm} {tail.hex()}", flush=True)
passes = 0; n = {}; per = {}; tw = time.time()
def log(t, sign, a, nm, tail):
    n[nm] = n.get(nm, 0) + 1; per[a] = per.get(a, 0) + 1
    if per[a] <= 12: print(f"{t:7.2f}s {sign} {a:#x} {nm} {tail.hex()}", flush=True)
while time.time() - t0 < secs:
    t = time.time() - t0
    if passes % 15 == 14:
        refresh(); print(f"{t:7.2f}s active={len(active)*BLOCK>>20} MiB", flush=True)
    cur = snap(); passes += 1
    for a, (nm, tail) in cur.items():
        if a not in prev: log(t, "+", a, nm, tail)
        elif prev[a][1] != tail: log(t, "~", a, nm, tail)
    for a in set(prev) - set(cur):
        log(t, "-", a, prev[a][0], b"")
    prev = cur
print(f"done passes={passes} avg={(time.time()-tw)/max(passes,1):.3f}s events={n} addrs={len(per)}", flush=True)
