#!/usr/bin/env bash
#
# Builds emm from upstream's source at the tag enshroudedbonker pins, for
# developers. The program never runs this: players get upstream's released
# emm.exe with `enshroudedbonker tools get-emm` (spec 003).
#
#   scripts/build-emm.sh [work-dir]
#
# Needs git and cargo. The checkout and build go in work-dir (default
# ~/.cache/enshroudedbonker/emm-src). The last line printed is the binary,
# ready for `enshroudedbonker config set emm_path <it>`.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PIN_FILE="${REPO_DIR}/internal/emm/emm.go"

# The pin lives in one place, the Go source; read it from there.
TAG="$(sed -n 's/^\tTag  *= "\(v[^"]*\)"$/\1/p' "${PIN_FILE}")"
REPO="$(sed -n 's/^\tRepo *= "\([^"]*\)"$/\1/p' "${PIN_FILE}")"
if [[ -z "${TAG}" || -z "${REPO}" ]]; then
	echo "build-emm: could not read Tag and Repo from ${PIN_FILE}" >&2
	exit 1
fi

for tool in git cargo; do
	if ! command -v "${tool}" >/dev/null; then
		echo "build-emm: ${tool} is not on PATH" >&2
		exit 1
	fi
done

WORK="${1:-${XDG_CACHE_HOME:-${HOME}/.cache}/enshroudedbonker/emm-src}"
SRC="${WORK}/kfc-parser-${TAG}"

if [[ ! -f "${SRC}/Cargo.toml" ]]; then
	mkdir -p "${WORK}"
	rm -rf "${SRC}"
	git clone --depth 1 --branch "${TAG}" "${REPO}" "${SRC}" >&2
fi

# --locked: the dependency set upstream committed, not whatever resolves today.
(cd "${SRC}" && cargo build --release --locked -p mod-manager-cli >&2)

BIN="${SRC}/target/release/emm"
"${BIN}" --version >&2
echo "${BIN}"
