#!/bin/bash
# exec.sh — run ONE command inside updown's dev container, in /updown (the project).
# This is how anything is built or run: the host only has Docker.
#
#     bash dockerspace/host_scripts/exec.sh go version
#     bash dockerspace/host_scripts/exec.sh bash dockerspace/container_scripts/env_check.sh
#
# Exit code = the command's exit code (1 if the container is not running).
set -uo pipefail
C=updown-dev-container
if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$C"; then
    echo -e "\033[31m[ERROR]\033[0m  $C is not running — start it: bash start.sh" >&2
    exit 1
fi
[ $# -gt 0 ] || { echo "usage: exec.sh COMMAND [ARGS…]" >&2; exit 2; }
exec docker exec -w /updown "$C" "$@"
