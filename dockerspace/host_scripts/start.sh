#!/bin/bash
# start.sh — start updown's dev container (docker compose) from its EXISTING image.
#
#     bash dockerspace/host_scripts/start.sh            # daily path: no rebuild
#     bash dockerspace/host_scripts/start.sh --build    # rebuild the image first (Dockerfile / GO_IMAGE changed)
#
# Idempotent: already running → says so, exit 0. Waits until the container runs and Go
# answers inside it, so "OK" means usable. Exit 1 = it did not come up.
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

if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$C"; then
    echo -e "\033[32m[  OK  ]\033[0m $C already running"; exit 0
fi

# Standalone: the shared network the compose joins — created here if missing.
docker network inspect my_docker_network >/dev/null 2>&1 \
    || docker network create --subnet=172.28.0.0/16 my_docker_network >/dev/null

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

for _ in $(seq 1 20); do
    if docker exec "$C" go version >/dev/null 2>&1; then
        echo -e "\033[32m[  OK  ]\033[0m $C running — $(docker exec "$C" go version)"
        exit 0
    fi
    sleep 1
done
echo -e "\033[31m[ERROR]\033[0m  $C did not come up (docker logs $C)"
exit 1
