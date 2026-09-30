#!/bin/bash
# pull_images.sh — the ONE online step: download every image updown needs (GO_IMAGE and
# TEST_IMAGES from dockerspace/.env) so everything after works offline. Images already on
# this machine are skipped — no network used for them.
#     bash dockerspace/host_scripts/pull_images.sh
# Exit 1 = an image is still missing (offline? wrong tag?), listed at the end.
set -uo pipefail

# ── Mirror logging ─────────────────────────────────────────────────────────────
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then
    source "$_WS_ROOT/init/create_logging_path.sh"
    setup_logging
fi
# ──────────────────────────────────────────────────────────────────────────────

DOCKERSPACE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$DOCKERSPACE/.env"
MISSING=()
for img in $GO_IMAGE $TEST_IMAGES; do
    if docker image inspect "$img" >/dev/null 2>&1; then
        echo -e "\033[32m[  OK  ]\033[0m $img — already here"
    elif docker pull -q "$img" >/dev/null; then
        echo -e "\033[32m[  OK  ]\033[0m $img — pulled"
    else
        echo -e "\033[31m[ERROR]\033[0m  $img — could not pull"; MISSING+=("$img")
    fi
done
[ ${#MISSING[@]} -eq 0 ] || { echo -e "\033[31m[ERROR]\033[0m  still missing: ${MISSING[*]}"; exit 1; }
exit 0
