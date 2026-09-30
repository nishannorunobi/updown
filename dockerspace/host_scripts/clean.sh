#!/bin/bash
# clean.sh — remove updown's dev CONTAINER (the next start.sh recreates it from the image).
#
#     bash dockerspace/host_scripts/clean.sh            # container only
#     bash dockerspace/host_scripts/clean.sh --image    # also the dev image (a rebuild needs
#                                                       # the GO_IMAGE base, kept offline)
# Offline-first: never removes the Go base image, the test images or the Go cache in
# mountspace — those would need the internet to get back.
set -uo pipefail

# ── Mirror logging ─────────────────────────────────────────────────────────────
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then
    source "$_WS_ROOT/init/create_logging_path.sh"
    setup_logging
fi
# ──────────────────────────────────────────────────────────────────────────────

C=updown-dev-container
docker rm -f "$C" >/dev/null 2>&1 && echo -e "\033[32m[  OK  ]\033[0m $C removed" \
    || echo -e "\033[32m[  OK  ]\033[0m $C did not exist"
if [ "${1:-}" = "--image" ]; then
    docker rmi updown-dev-image:1.0 >/dev/null 2>&1 && echo -e "\033[32m[  OK  ]\033[0m updown-dev-image:1.0 removed" \
        || echo -e "\033[32m[  OK  ]\033[0m updown-dev-image:1.0 did not exist"
fi
exit 0
