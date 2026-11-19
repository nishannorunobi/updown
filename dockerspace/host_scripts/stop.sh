#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# stop.sh — stop updown's dev container by its EXPLICIT name (never compose-wide: no other project is touched)
#
# What for    Keeps the container + image, so the next start.sh is instant; clean.sh removes the container.
# How         bash dockerspace/host_scripts/stop.sh
# Idempotent  not running → says so, exit 0
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/stop.py" "$@"
