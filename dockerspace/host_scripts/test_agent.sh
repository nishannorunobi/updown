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
case "${1:-}" in -h|--help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;; esac
set -uo pipefail
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then source "$_WS_ROOT/init/create_logging_path.sh"; setup_logging; fi

PROJECT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
T=updown-agent-test; IMG=updown-dev-image:1.0; FAIL=0
ok()  { echo -e "\033[32m[  OK  ]\033[0m $*"; }
bad() { echo -e "\033[31m[ERROR]\033[0m  $*"; FAIL=1; }
docker ps --format '{{.Names}}' | grep -qx updown-dev-container && { bad "stop updown first (bash stop.sh) — the test needs ports 8070-8079"; exit 1; }
docker image inspect "$IMG" >/dev/null 2>&1 || { bad "image $IMG missing — bash start.sh --build"; exit 1; }
TMP="$(mktemp -d)"; mkdir -p "$TMP/dl" "$TMP/cache"
cp -r "$_WS_ROOT/mountspace/data/myworkspace/projectspace/updown/gocache/." "$TMP/cache/" 2>/dev/null   # a warm cache: offline build
sed 's/^IDLE_MINUTES=.*/IDLE_MINUTES=1/; s/^BIND=.*/BIND=auto/; s/^HOST_PORT=.*/HOST_PORT=8079/' "$PROJECT/updown-agent/updown.conf" > "$TMP/test.conf"   # any network (no hotspot needed); its own page port
cleanup() { docker rm -f "$T" "$T-none" >/dev/null 2>&1; rm -rf "$TMP"; }
trap cleanup EXIT
link() { docker exec "$T" python3 /updown/updown-agent/agent.py link "${1:-60}" 2>/dev/null; }

docker rm -f "$T" >/dev/null 2>&1
docker run -d --name "$T" --network host --user "$(id -u):$(id -g)" \
    -v "$PROJECT:/updown" -v "$TMP/cache:/gocache" -v "$TMP/dl:/downloads" -v "$TMP/test.conf:/tmp/test.conf:ro" \
    -e CONTAINER_WORKDIR=/updown -e LOG_MIRROR_ROOT=/tmp -e UPDOWN_AGENT_CONF=/tmp/test.conf \
    "$IMG" python3 /updown/updown-agent/agent.py >/dev/null || { bad "throwaway container did not start"; exit 1; }

# A1 + A2
L1="$(link 180)"
[ "$L1" = "http://localhost:8079" ] && ok "A1 the agent built + started updown, link: $L1" || bad "A1 link [$L1] (docker logs: $(docker logs "$T" 2>&1 | tail -2 | tr '\n' ' '))"
[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$L1")" = 200 ] && ok "A2 the link opens the host page (200)" || bad "A2 host page not 200"

# A3 — a crash
docker exec "$T" sh -c 'for p in /proc/[0-9]*; do case "$(tr "\0" " " < $p/cmdline 2>/dev/null)" in /tmp/updown-agent/updown*) kill -9 ${p#/proc/} ;; esac; done'
sleep 4; L2="$(link 60)"
sleep 3; [ "$L2" = "$L1" ] && [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$L1")" = 200 ] && ok "A3 updown killed → started again, the same link answers" || bad "A3 L2=[$L2]"

# A4 — idle stop
s=$(date +%s); until docker logs "$T" 2>&1 | grep -q 'stopped by itself'; do sleep 5; [ $(( $(date +%s) - s )) -gt 150 ] && break; done
sleep 4; L3="$(link 60)"
[ "$L3" = "$L1" ] && ok "A4 1 idle minute → updown stopped by itself, started again, the same link ($(( $(date +%s) - s ))s)" || bad "A4 L3=[$L3]"

# A7 — the page reports the connection
st="$(curl -s --max-time 5 "$L1/host/api/status?after=0")"
python3 -c 'import sys,json; c=json.loads(sys.argv[1])["connection"]; sys.exit(0 if c["state"] in ("hotspot","lan") and c["via"] and c["pc"] else 1)' "$st" 2>/dev/null \
    && ok "A7 the page reports the connection: $(python3 -c 'import sys,json; c=json.loads(sys.argv[1])["connection"]; print(c["state"], "over", c["via"], "·", c["pc"])' "$st")" || bad "A7 status: $(echo "$st" | cut -c1-200)"

# A8 — no phone door possible: updown still runs, the page shows state "none" + why (the red state)
printf 'BIND=192.0.2.1\nHOST_PORT=8078\nSTOP_MODE=idle\nRECEIVE_DIR=/downloads\nSHARE_DIR=/downloads\n' > "$TMP/none.conf"
docker run -d --name "$T-none" --network host --user "$(id -u):$(id -g)" -v "$PROJECT:/updown" -v "$TMP/cache:/gocache" -v "$TMP/dl:/downloads" \
    -v "$TMP/none.conf:/tmp/none.conf:ro" -w /updown "$IMG" sh -c 'go build -o /tmp/u ./cmd/updown && exec /tmp/u --config /tmp/none.conf' >/dev/null
for _ in $(seq 1 90); do st="$(curl -s --max-time 2 http://localhost:8078/host/api/status?after=0)"; [ -n "$st" ] && break; sleep 1; done
python3 -c 'import sys,json; c=json.loads(sys.argv[1])["connection"]; sys.exit(0 if c["state"]=="none" and c["reason"] else 1)' "$st" 2>/dev/null \
    && [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 http://localhost:8078/)" = 200 ] \
    && ok "A8 no phone door → updown still runs, the page answers, state none: $(python3 -c 'import sys,json; print(json.loads(sys.argv[1])["connection"]["reason"][:70])' "$st")" || bad "A8 status: $(echo "$st" | cut -c1-200)"
docker rm -f "$T-none" >/dev/null 2>&1

# A5 — docker stop
docker stop -t 15 "$T" >/dev/null
[ "$(docker inspect -f '{{.State.ExitCode}}' "$T")" = 0 ] && docker logs "$T" 2>&1 | tail -1 | grep -q 'stopped' \
    && ok "A5 docker stop → updown stopped, the agent exited 0" || bad "A5 exit $(docker inspect -f '{{.State.ExitCode}}' "$T")"

# A6 — no phone token in the output (the phone links carry ?t=)
docker logs "$T" 2>&1 | grep -q '[?&]t=' && bad "A6 a phone token appeared in the output" || ok "A6 no phone token in the output — only host-page links"

[ "$FAIL" = 0 ] && ok "updown-agent tests passed (throwaway container removed)" && exit 0
exit 1
