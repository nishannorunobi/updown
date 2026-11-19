#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_agent.sh — updown-agent ([2.1], UPDOWN-26) on a THROWAWAY container: the same image, command and
# network as docker-compose.yml, but temp folders for Downloads + Go cache and a 1-minute idle stop.
# The real updown-dev-container is never touched (it must be stopped: both use ports 8070-8079).
#
# Checks      A1 the agent builds + starts updown and hands out a link (agent.py link)
#             A2 the link opens the host page from this PC (200)
#             A3 updown killed → started again, the SAME link (http://localhost:HOST_PORT), the page answers again
#             A4 1 idle minute → updown stops by itself → started again, the same link
#             A5 docker stop → updown stopped, the agent exits 0
#             A6 the phone token never in the agent's output (only the host-page link)
#             A7 the page reports the connection (status.connection: state + via + this PC)          ([2.6] UPDOWN-52)
#             A8 no phone door possible (BIND = an address this PC does not have) → updown still runs, the page
#                answers on localhost with connection.state "none" + the reason (the red state)
# How         bash dockerspace/host_scripts/test_agent.sh        (~2 minutes)
# Exit        0 = all passed
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/test_agent.py" "$@"
