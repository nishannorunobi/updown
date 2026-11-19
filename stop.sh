#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# stop.sh — stop updown's dev container (idempotent; container + image kept)
#
# What for    This project's own entry point; the work is dockerspace/host_scripts/stop.py.
# How         bash stop.sh
# Output      [  OK  ] updown-dev-container stopped   ·   … already stopped
# Errors      exit 1 = docker could not stop it
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/dockerspace/host_scripts/stop.py" "$@"
