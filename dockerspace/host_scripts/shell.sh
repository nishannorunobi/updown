#!/bin/bash
# shell.sh — an interactive bash inside updown's dev container, in /updown (the project).
#     bash dockerspace/host_scripts/shell.sh
set -uo pipefail
C=updown-dev-container
if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$C"; then
    echo -e "\033[31m[ERROR]\033[0m  $C is not running — start it: bash start.sh" >&2
    exit 1
fi
exec docker exec -it -w /updown "$C" bash
