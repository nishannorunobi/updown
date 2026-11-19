#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_controls.sh — the page's buttons and settings ([2.7]) on a THROWAWAY container: the same image and
# network as docker-compose.yml, its own ports (page 8077, buttons 8068), temp Downloads, a temp copy of
# updown.conf + actions.conf + actions/. Runs next to the real updown-dev-container (never touches it).
#
# Checks      C1 the page's blocks come from PAGE_PARTS (order kept, an unknown block left out with a WARN)
#             C2 the buttons come from actions.conf: the switch with its choices, the buttons, SHOWN=no hidden
#             C3 a broken action file → only its button is greyed, with the reason
#             C4 ⟳ Check now (check_now.py) answers with the connection
#             C5 🔄 Refresh QR code (new_link.py) → a new token: the page's link changes
#             C6 another web site can press no button (agent and updown both answer 403); other Host → 403
#             C7 the switch (network_mode.py) → BIND written into updown.conf, updown restarted, the page back
#             C8 in a real browser (headless Chrome, when installed) the page draws the buttons — CSP lets it reach them
# How         bash dockerspace/host_scripts/test_controls.sh        (~1 minute)
# Exit        0 = all passed
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/test_controls.py" "$@"
