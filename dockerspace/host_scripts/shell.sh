#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# shell.sh — an interactive bash inside updown's dev container, in /updown (the project)
#
# How         bash dockerspace/host_scripts/shell.sh
# Errors      exit 1 = the container is not running (bash start.sh)
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/shell.py" "$@"
