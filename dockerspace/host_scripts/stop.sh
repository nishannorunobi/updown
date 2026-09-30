#!/bin/bash
# stop.sh — stop updown's dev container. Keeps the container + image, so the next
# start.sh is instant; clean.sh removes the container. Uses the EXPLICIT container name
# (never a compose-wide command), so no other project can be touched.
# Idempotent: not running → says so, exit 0.
set -uo pipefail

# ── Mirror logging ─────────────────────────────────────────────────────────────
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then
    source "$_WS_ROOT/init/create_logging_path.sh"
    setup_logging
fi
# ──────────────────────────────────────────────────────────────────────────────

C=updown-dev-container
if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$C"; then
    echo -e "\033[32m[  OK  ]\033[0m $C already stopped"; exit 0
fi
docker stop "$C" >/dev/null && echo -e "\033[32m[  OK  ]\033[0m $C stopped" && exit 0
echo -e "\033[31m[ERROR]\033[0m  could not stop $C"
exit 1
