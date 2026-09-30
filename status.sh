#!/bin/bash
# status.sh — is updown's dev container running?
#     bash status.sh            # human
#     bash status.sh --state    # one word: running / stopped
set -uo pipefail
docker ps --format '{{.Names}}' 2>/dev/null | grep -qx updown-dev-container && s=running || s=stopped
[ "${1:-}" = --state ] && { echo "$s"; exit 0; }
echo "updown-dev-container: $s"
