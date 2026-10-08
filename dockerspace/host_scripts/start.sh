#!/bin/bash
# start.sh — start updown's container (docker compose) from its EXISTING image. Inside, updown-agent (the
# entry point, [2.1] UPDOWN-26) builds and runs updown; this script only starts the container and shows
# the link the agent hands out — open it in this PC's browser for the QR codes.
#
#     bash dockerspace/host_scripts/start.sh            # daily path: no rebuild
#     bash dockerspace/host_scripts/start.sh --build    # rebuild the image first (Dockerfile / GO_IMAGE changed)
#
# Idempotent: docker compose up (a changed setting recreates the container; otherwise nothing changes).
# Exit 1 = the container did not come up.
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
C=updown-dev-container

# Bind-mount sources must exist and belong to the host user BEFORE compose runs,
# otherwise Docker creates them root-owned and the container (running as this user)
# cannot write its Go cache or logs.
mkdir -p "$PROJECT/../../mountspace/data/myworkspace/projectspace/updown/gocache" \
         "$PROJECT/../../mountspace/logs/myworkspace/projectspace/updown" 2>/dev/null || true

export HOST_UID="$(id -u)" HOST_GID="$(id -g)"
# This PC's Downloads folder (XDG on Linux, else ~/Downloads) — mounted at /downloads.
HOST_DOWNLOADS="$(sed -n 's/^XDG_DOWNLOAD_DIR="\(.*\)"/\1/p' "$HOME/.config/user-dirs.dirs" 2>/dev/null | sed "s|\$HOME|$HOME|")"
export HOST_DOWNLOADS="${HOST_DOWNLOADS:-$HOME/Downloads}"
mkdir -p "$HOST_DOWNLOADS"
cd "$DOCKERSPACE" || exit 1
if [ "${1:-}" = "--build" ]; then
    echo "==> Rebuilding updown-dev-image, then starting…"
    docker compose up -d --build || exit 1
else
    docker compose up -d || exit 1
fi

# the link — asked from updown-agent inside (it builds updown first: up to ~1 minute on a cold Go cache)
if link="$(docker exec "$C" python3 /updown/updown-agent/agent.py link 90 2>/dev/null)"; then
    echo -e "\033[32m[  OK  ]\033[0m $C running — updown is ready"
    echo "         open this link in this PC's browser for the QR codes (send / receive files):"
    echo -e "         \033[1m$link\033[0m"
    echo "         a new link comes after 15 idle minutes — the current one: bash status.sh"
    exit 0
fi
if docker ps --format '{{.Names}}' | grep -qx "$C"; then
    echo -e "\033[33m[WARN]\033[0m   $C running, but updown gave no link yet — see: docker logs $C"; exit 0
fi
echo -e "\033[31m[ERROR]\033[0m  $C did not come up (docker logs $C)"
exit 1
