#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# start.sh — start updown's dev container (Go toolchain, builds, test runs — nothing on the host PC)
#
# What for    This project's own entry point, so it is manageable on its own; the work is the host start
#             (dockerspace/host_scripts/start.py) — arguments pass through.
# How         bash start.sh            daily: no rebuild
#             bash start.sh --build    rebuild the image first (Dockerfile / GO_IMAGE changed)
# Output      [  OK  ] updown-dev-container running — updown is ready + the link for this PC's browser (QR codes)
# Errors      exit 1 = the container did not come up (docker logs updown-dev-container)
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/dockerspace/host_scripts/start.py" "$@"
