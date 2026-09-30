#!/bin/bash
# test_release.sh — run a built release folder in CLEAN Linux containers with networking
# OFF, to prove it installs and runs on a fresh machine without internet.
#
#     bash dockerspace/host_scripts/test_release.sh --target linux-x64
#     bash dockerspace/host_scripts/test_release.sh --target linux-x64 --image debian:12-slim
#     bash dockerspace/host_scripts/test_release.sh --target linux-x64 --cmd "./start.sh --check"
#
#   --target  which release: the newest release/<name>-<version>-<target>/ folder
#   --image   one image (default: every TEST_IMAGES entry in dockerspace/.env)
#   --cmd     what to run inside the release folder (default "./start.sh --check": the
#             release contract — print name + version, check its files, exit 0)
# Each run: docker run --rm --network none, the release mounted READ-ONLY at /opt/updown,
# as a non-root user. Exit 0 = passed in every image; 1 = a failure (listed at the end).
set -uo pipefail

# ── Mirror logging ─────────────────────────────────────────────────────────────
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then
    source "$_WS_ROOT/init/create_logging_path.sh"
    setup_logging
fi
# ──────────────────────────────────────────────────────────────────────────────

DOCKERSPACE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT="$(dirname "$DOCKERSPACE")"
source "$DOCKERSPACE/.env"

TARGET=""; IMAGES="$TEST_IMAGES"; CMD="./start.sh --check"
while [ $# -gt 0 ]; do
    case "$1" in
        --target) shift; TARGET="${1:-}" ;;
        --image)  shift; IMAGES="${1:-}" ;;
        --cmd)    shift; CMD="${1:-}" ;;
        --help|-h) sed -n '2,15p' "${BASH_SOURCE[0]}"; exit 0 ;;
        *) echo "unknown option: $1 (try --help)"; exit 2 ;;
    esac
    shift
done
[ -n "$TARGET" ] || { echo "usage: test_release.sh --target <os-arch> [--image IMG] [--cmd CMD]"; exit 2; }

REL="$(ls -1d "$PROJECT"/release/*-"$TARGET" 2>/dev/null | sort -V | tail -1)"
[ -n "$REL" ] && [ -d "$REL" ] || { echo -e "\033[31m[ERROR]\033[0m  no release/*-$TARGET folder — build it first"; exit 1; }
echo "==> Release: $REL"

FAILED=()
for img in $IMAGES; do
    docker image inspect "$img" >/dev/null 2>&1 \
        || { echo -e "\033[31m[ERROR]\033[0m  $img not on this machine — run host_scripts/pull_images.sh once (online)"; FAILED+=("$img"); continue; }
    echo "==> $img  (network: none)"
    if docker run --rm --network none --user 65534:65534 \
           -v "$REL":/opt/updown:ro -w /opt/updown "$img" sh -c "$CMD"; then
        echo -e "\033[32m[  OK  ]\033[0m $img — passed"
    else
        echo -e "\033[31m[ERROR]\033[0m  $img — failed"; FAILED+=("$img")
    fi
done
[ ${#FAILED[@]} -eq 0 ] || { echo -e "\033[31m[ERROR]\033[0m  failed in: ${FAILED[*]}"; exit 1; }
echo -e "\033[32m[  OK  ]\033[0m $(basename "$REL") passed offline in: $IMAGES"
exit 0
