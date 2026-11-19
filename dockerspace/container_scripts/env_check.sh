#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# env_check.sh — runs INSIDE updown-dev-container: proves the dev environment is ready
#
# How         bash dockerspace/host_scripts/exec.sh bash dockerspace/container_scripts/env_check.sh
# Checks      Go present and pinned offline (GOTOOLCHAIN=local, GOPROXY=off), caches writable, a tiny pure-Go
#             program cross-compiles for every product target with no network
# Errors      exit 0 ready · 1 something is wrong (listed)
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/env_check.py" "$@"
