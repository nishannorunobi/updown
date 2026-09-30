#!/bin/bash
# start.sh — start updown's dev container (Go toolchain, builds, test runs — nothing on the
# host PC). This project's own entry point, so it is manageable on its own; the real work
# is in dockerspace/host_scripts/start.sh (arguments pass through, e.g. start.sh --build).
set -uo pipefail

# ── Mirror logging ─────────────────────────────────────────────────────────────
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then
    source "$_WS_ROOT/init/create_logging_path.sh"
    setup_logging
fi
# ──────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec bash "$SCRIPT_DIR/dockerspace/host_scripts/start.sh" "$@"
