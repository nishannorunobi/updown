#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# exec.sh — run ONE command inside updown's dev container, in /updown (the project)
#
# What for    This is how anything is built or run: the host only has Docker.
# How         bash dockerspace/host_scripts/exec.sh go version
#             bash dockerspace/host_scripts/exec.sh bash dockerspace/container_scripts/env_check.sh
# Errors      exit = the command's exit code · 1 the container is not running · 2 no command given
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/exec_in.py" "$@"
