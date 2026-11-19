#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# updown_container_login.sh — a shell INSIDE updown's running container, to look around (UPDOWN)
#
# What for    One command instead of remembering: docker exec -it <container> bash|sh. A container has no SSH
#             and no login prompt — this is the way in. Type exit to come back; the container keeps running.
# Steps       1. the container: updown-dev-container (the default)
#             2. checks it runs (else: how to start it)
#             3. a short reminder, then the shell — bash if the image has it, else sh
# How         bash ~/myworkspace/projectspace/apps/updown/dockerspace/host_scripts/updown_container_login.sh
# Careful     the shared folder and settings are REAL — look, do not change.
# See also    a single command without a shell: docker exec updown-dev-container <command>
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/container_login.py" "$@"
