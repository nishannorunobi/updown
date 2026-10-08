#!/bin/bash
# status.sh — is updown running, and what is its current link?
#     bash status.sh            # human: running/stopped + the link (asked from updown-agent)
#     bash status.sh --state    # one word: running / stopped
set -uo pipefail
docker ps --format '{{.Names}}' 2>/dev/null | grep -qx updown-dev-container && s=running || s=stopped
[ "${1:-}" = --state ] && { echo "$s"; exit 0; }
echo "updown-dev-container: $s"
if [ "$s" = running ]; then
    link="$(docker exec updown-dev-container python3 /updown/updown-agent/agent.py link 5 2>/dev/null)" \
        && echo "link (open in this PC's browser for the QR codes): $link" \
        || echo "no link right now — updown is (re)starting; try again in a moment"
fi
