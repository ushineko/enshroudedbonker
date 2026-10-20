# Synthetic stand-in for upstream emm, for tests only. Mimics the slice of
# `emm run -g <dir> --patch` the pipeline relies on: appends to the resource
# file, rewrites the archive index keeping its original bytes (and so its build
# string), registers nine empty containers, and prints the log lines the
# parser reads. Behaviour knobs (environment):
#   FAKE_EMM_MODE=ok|fail|rewrite   rewrite clobbers the first resource byte
#   FAKE_EMM_FAIL_WITH=<mod id>     fail whenever that mod is staged
#   FAKE_EMM_SILENT=<mod id>        that mod prints nothing of its own
set -eu
if [ "${1:-}" = "--version" ]; then echo "emm 0.0.0-fake"; exit 0; fi
if [ "${1:-}" != "run" ]; then echo "Error: unexpected command ${1:-}"; exit 2; fi
shift
game=""
patch=0
while [ $# -gt 0 ]; do
  case "$1" in
    -g) game="$2"; shift 2 ;;
    --patch) patch=1; shift ;;
    *) shift ;;
  esac
done
if [ -z "$game" ]; then echo "Error: no game directory"; exit 2; fi
mode="${FAKE_EMM_MODE:-ok}"
printf '\033[2m2026-01-01T00:00:00Z\033[0m \033[32m INFO\033[0m Loading %s\n' "$game"
n=0
for d in "$game"/mods/*/; do
  [ -d "$d" ] || continue
  id="$(basename "$d")"
  printf '\033[32m INFO\033[0m Running mod mod_id="%s"\n' "$id"
  if [ "$id" = "${FAKE_EMM_FAIL_WITH:-}" ]; then
    echo "Error: mod $id failed: synthetic failure"
    exit 1
  fi
  if [ "$id" != "${FAKE_EMM_SILENT:-}" ]; then
    printf '\033[32m INFO\033[0m \033[1mlua\033[0m\033[1m{\033[0mmod_id="%s"\033[1m}\033[0m: eml::lua: %s: changed 5 things\n' "$id" "$id"
  fi
  n=$((n + 1))
done
if [ "$mode" = "fail" ]; then echo "Error: synthetic failure"; exit 1; fi
if [ "$patch" = 1 ]; then
  if [ "$mode" = "rewrite" ]; then
    printf 'X' | dd of="$game/enshrouded.kfc_resources" bs=1 count=1 conv=notrunc 2>/dev/null
  fi
  printf 'APPENDED-RECORDS' >> "$game/enshrouded.kfc_resources"
  cp "$game/enshrouded.kfc" "$game/.kfc.fake-tmp"
  printf 'INDEX-TRAILER' >> "$game/.kfc.fake-tmp"
  mv "$game/.kfc.fake-tmp" "$game/enshrouded.kfc"
  for i in 055 056 057 058 059 060 061 062 063; do
    : > "$game/enshrouded_$i.dat"
  done
fi
echo "Applied $((n * 3)) patches"
