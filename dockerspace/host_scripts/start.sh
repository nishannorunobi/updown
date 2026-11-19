#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# start.sh — start updown's container (docker compose) from its EXISTING image and show its link
#
# What for    Inside, updown-agent (the entry point, [2.1] UPDOWN-26) builds and runs updown; this only starts the
#             container and shows the link the agent hands out — open it in this PC's browser for the QR codes.
# How         bash dockerspace/host_scripts/start.sh            daily path: no rebuild
#             bash dockerspace/host_scripts/start.sh --build    rebuild the image first (Dockerfile / GO_IMAGE changed)
# Steps       1. bind-mount folders made, owned by you   2. this PC's Downloads → /downloads
#             3. the pod guard (CLAUDEWORK-258)   4. the log volume handed to you   5. docker compose up -d
#             6. the link (≤ 90 s)
# Errors      exit 1 = the container did not come up · the pod is not at its folder · the log volume
# Idempotent  docker compose up: a changed setting recreates the container; otherwise nothing changes
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/start.py" "$@"
