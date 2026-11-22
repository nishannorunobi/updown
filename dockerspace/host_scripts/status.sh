#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# status.sh — is updown running, and what is its current link?
#
# How         bash dockerspace/host_scripts/status.sh            human: running / stopped + the link (asked from updown-agent)
#             bash dockerspace/host_scripts/status.sh --state    one word: running / stopped   (svcmgt)
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/status.py" "$@"
