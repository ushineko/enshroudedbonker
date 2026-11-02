#!/usr/bin/env python3
import os, sys, time
d = sys.argv[1]; out = sys.argv[2]; req = sys.argv[3:]
r = os.path.join(d, "resp.txt")
if os.path.exists(r): os.remove(r)
open(os.path.join(d, "req.txt.tmp"), "w").write("\n".join(req) + "\n"); os.rename(os.path.join(d, "req.txt.tmp"), os.path.join(d, "req.txt"))
for _ in range(600):
    if os.path.exists(r):
        os.replace(r, out); print("ok", os.path.getsize(out)); break
    time.sleep(0.1)
else:
    print("timeout")
