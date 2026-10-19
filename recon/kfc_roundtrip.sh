#!/usr/bin/env bash
# Round-trip and patch test of the game's .kfc archive with kfc-parser's
# cli-legacy, on Linux, against a COPY of the install. Never touches the game.
#
#   recon/kfc_roundtrip.sh <game-dir> <kfc-parser-binary> <work-dir>
#
# <game-dir> needs enshrouded.exe, enshrouded.kfc, enshrouded.kfc_resources
# (the .dat files are not needed for reflected resources such as ItemInfo).
set -euo pipefail
GAME=$1 K=$(realpath "$2") W=$3
mkdir -p "$W" && cd "$W"
fresh() { rm -rf "$1" && mkdir "$1" && cp "$GAME"/enshrouded.{exe,kfc,kfc_resources} "$1"/ && chmod u+w "$1"/*; }

fresh orig
(cd orig && "$K" extract-types -g .)              # writes reflection_data.json next to the binary on some builds
[ -f orig/reflection_data.json ] || mv "$(dirname "$K")/reflection_data.json" orig/
"$K" unpack -g orig -o unpacked -f tkeen::ItemInfo

echo "== unchanged repack"
fresh rt && cp orig/reflection_data.json rt/
"$K" repack -g rt -i unpacked/ItemInfo
cmp -n "$(stat -c%s orig/enshrouded.kfc_resources)" orig/enshrouded.kfc_resources rt/enshrouded.kfc_resources \
  && echo "original resource bytes untouched; appended $(( $(stat -c%s rt/enshrouded.kfc_resources) - $(stat -c%s orig/enshrouded.kfc_resources) )) bytes"
"$K" unpack -g rt -o unpacked_rt -f tkeen::ItemInfo
diff -rq unpacked/ItemInfo unpacked_rt/ItemInfo && echo "ItemInfo semantic round-trip: identical"

echo "== x10 stack patch"
rm -rf patched_in && cp -r unpacked/ItemInfo patched_in
python3 -I - patched_in <<'EOF'
import glob, json, sys
for f in glob.glob(sys.argv[1] + "/*.json"):
    d = json.load(open(f))
    if d["maxStackSize"] > 1:
        d["maxStackSize"] = min(d["maxStackSize"] * 10, 65535)
        json.dump(d, open(f, "w"))
EOF
fresh pt && cp orig/reflection_data.json pt/
"$K" repack -g pt -i patched_in
"$K" unpack -g pt -o unpacked_pt -f tkeen::ItemInfo
python3 -I - unpacked/ItemInfo unpacked_pt/ItemInfo <<'EOF'
import glob, json, os, sys
ok = other = 0
for f in glob.glob(sys.argv[1] + "/*.json"):
    a = json.load(open(f)); b = json.load(open(os.path.join(sys.argv[2], os.path.basename(f))))
    want = min(a["maxStackSize"] * 10, 65535) if a["maxStackSize"] > 1 else a["maxStackSize"]
    ok += b.pop("maxStackSize") == want; a.pop("maxStackSize"); other += a != b
print(f"stack values correct: {ok}; records with any other field changed: {other}")
EOF
