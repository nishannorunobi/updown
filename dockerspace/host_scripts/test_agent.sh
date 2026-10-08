#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_agent.sh — updown-agent ([2.1], UPDOWN-26) on a THROWAWAY container: the same image, command and
# network as docker-compose.yml, but temp folders for Downloads + Go cache and a 1-minute idle stop.
# The real updown-dev-container is never touched (it must be stopped: both use ports 8070-8079).
#
# Checks      A1 the agent builds + starts updown and hands out a link (agent.py link)
#             A2 the link opens the host page from this PC (200)
#             A3 updown killed → started again, a NEW link
#             A4 1 idle minute → updown stops by itself → started again, a NEW link
#             A5 docker stop → updown stopped, the agent exits 0
#             A6 the phone token never in the agent's output (only the host-page link)
#             A7 the PC's network changes → updown restarted on the new address, a NEW link   ([2.4] UPDOWN-35)
#             A8 no phone hotspot → updown stopped, "waiting …", no link; hotspot back → started, a link
#             (A7/A8 feed the agent a fake answer for "which address" — the real choice is tested in Go: TestPick)
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
sed 's/^IDLE_MINUTES=.*/IDLE_MINUTES=1/; s/^BIND=.*/BIND=auto/' "$PROJECT/updown-agent/updown.conf" > "$TMP/test.conf"   # any network: the test must not depend on a hotspot
cleanup() { docker rm -f "$T" >/dev/null 2>&1; rm -rf "$TMP"; }
trap cleanup EXIT
link() { docker exec "$T" python3 /updown/updown-agent/agent.py link "${1:-60}" 2>/dev/null; }

# the address updown's own choice gives here (BIND=auto) — the fake "which address" answer starts with it
REAL="$(docker run --rm --network host --user "$(id -u):$(id -g)" -v "$PROJECT:/updown" -v "$TMP/cache:/gocache" -v "$TMP/test.conf:/tmp/test.conf:ro" -w /updown \
    "$IMG" sh -c 'go build -o /tmp/u ./cmd/updown && /tmp/u --config /tmp/test.conf --address' 2>/dev/null | tail -1)"
[ -n "$REAL" ] || { bad "this PC has no phone-reachable network at all — the test needs one (any Wi-Fi / LAN)"; exit 1; }
mkdir -p "$TMP/fake"; echo "$REAL" > "$TMP/fake/addr"

docker rm -f "$T" >/dev/null 2>&1
docker run -d --name "$T" --network host --user "$(id -u):$(id -g)" \
    -v "$PROJECT:/updown" -v "$TMP/cache:/gocache" -v "$TMP/dl:/downloads" -v "$TMP/test.conf:/tmp/test.conf:ro" -v "$TMP/fake:/fake" \
    -e CONTAINER_WORKDIR=/updown -e LOG_MIRROR_ROOT=/tmp -e UPDOWN_AGENT_CONF=/tmp/test.conf -e UPDOWN_AGENT_ADDRESS_CMD="cat /fake/addr" \
    "$IMG" python3 /updown/updown-agent/agent.py >/dev/null || { bad "throwaway container did not start"; exit 1; }

# A1 + A2
L1="$(link 180)"
[[ "$L1" == http://*:807[0-9]/host\?k=* ]] && ok "A1 the agent built + started updown, link: ${L1%%\?*}?k=…" || bad "A1 no link (docker logs: $(docker logs "$T" 2>&1 | tail -2 | tr '\n' ' '))"
[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$L1")" = 200 ] && ok "A2 the link opens the host page (200)" || bad "A2 host page not 200"

# A3 — a crash
docker exec "$T" sh -c 'for p in /proc/[0-9]*; do case "$(tr "\0" " " < $p/cmdline 2>/dev/null)" in /tmp/updown-agent/updown*) kill -9 ${p#/proc/} ;; esac; done'
sleep 4; L2="$(link 60)"
[ -n "$L2" ] && [ "$L2" != "$L1" ] && ok "A3 updown killed → started again with a new link" || bad "A3 L2=[$L2]"

# A4 — idle stop
s=$(date +%s); until docker logs "$T" 2>&1 | grep -q 'stopped by itself'; do sleep 5; [ $(( $(date +%s) - s )) -gt 150 ] && break; done
sleep 4; L3="$(link 60)"
[ -n "$L3" ] && [ "$L3" != "$L2" ] && ok "A4 1 idle minute → updown stopped by itself, started again with a new link ($(( $(date +%s) - s ))s)" || bad "A4 L3=[$L3]"

# A7 — the network changes: the fake answer moves to another address, then back (updown itself binds REAL)
L4="$(link 60)"; echo "192.0.2.77" > "$TMP/fake/addr"
s=$(date +%s); until docker logs "$T" 2>&1 | grep -q 'the network changed'; do sleep 2; [ $(( $(date +%s) - s )) -gt 40 ] && break; done
echo "$REAL" > "$TMP/fake/addr"; sleep 3; L5="$(link 60)"
docker logs "$T" 2>&1 | grep -q "the network changed: $REAL → 192.0.2.77" && [ -n "$L5" ] && [ "$L5" != "$L4" ] \
    && ok "A7 network changed → updown restarted, a new link ($(( $(date +%s) - s ))s)" || bad "A7 L4=[$L4] L5=[$L5]"

# A8 — no hotspot (the answer is empty): updown stopped, waiting, no link; back → a link
: > "$TMP/fake/addr"
s=$(date +%s); until docker logs "$T" 2>&1 | grep -q 'waiting —'; do sleep 2; [ $(( $(date +%s) - s )) -gt 40 ] && break; done
sleep 2; nolink="$(link 2)"
echo "$REAL" > "$TMP/fake/addr"; L6="$(link 60)"
docker logs "$T" 2>&1 | grep -q 'is gone' && [ -z "$nolink" ] && [ -n "$L6" ] && [ "$L6" != "$L5" ] \
    && ok "A8 no hotspot → updown stopped, 'waiting', no link · back → started with a new link" || bad "A8 nolink=[$nolink] L6=[$L6]"

# A5 — docker stop
docker stop -t 15 "$T" >/dev/null
[ "$(docker inspect -f '{{.State.ExitCode}}' "$T")" = 0 ] && docker logs "$T" 2>&1 | tail -1 | grep -q 'stopped' \
    && ok "A5 docker stop → updown stopped, the agent exited 0" || bad "A5 exit $(docker inspect -f '{{.State.ExitCode}}' "$T")"

# A6 — no phone token in the output (the phone links carry ?t=)
docker logs "$T" 2>&1 | grep -q '[?&]t=' && bad "A6 a phone token appeared in the output" || ok "A6 no phone token in the output — only host-page links"

[ "$FAIL" = 0 ] && ok "updown-agent tests passed (throwaway container removed)" && exit 0
exit 1
