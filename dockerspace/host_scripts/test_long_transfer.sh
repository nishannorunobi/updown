#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_long_transfer.sh — long transfers are never cut off; STOP_MODE=never keeps running ([2.8.1], UPDOWN-86)
#
# What for    A big file can take hours. Before this fix updown counted only the START of a request as activity,
#             so after IDLE_MINUTES it stopped in the middle of a download (cut file, new QR code).
# Checks      L1 STOP_MODE=idle, IDLE_MINUTES=1: a 40 MB download slowed to ~2¼ minutes (twice the idle time) completes,
#                byte for byte, and updown did not stop during it
#             L2 the same updown still stops by itself once it is really idle afterwards (idle mode still works)
#             L3 STOP_MODE=never, IDLE_MINUTES=1: idle for 2+ minutes → no stop, the same link (same QR codes)
#             L4 the same for an UPLOAD (phone → PC, 40 MB slowed to ~2¼ min): saved complete, no stop during it
#             L5 a small file (a few KB) both ways: unchanged, nothing special needed
# How         bash dockerspace/host_scripts/test_long_transfer.sh        (~7 minutes; two THROWAWAY containers on
#             their own ports 8075 / 8074 — runs beside the real updown, never touches it)
# Exit        0 = all passed
# ─────────────────────────────────────────────────────────────────────────────
case "${1:-}" in -h|--help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;; esac
set -uo pipefail
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then source "$_WS_ROOT/init/create_logging_path.sh"; setup_logging; fi

PROJECT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WS="$(cd "$(git -C "$PROJECT" rev-parse --path-format=absolute --git-common-dir)/../../.." && pwd)"   # the real workspace (also from a test worktree)
IMG=updown-dev-image:1.0; FAIL=0
ok()  { echo -e "\033[32m[  OK  ]\033[0m $*"; }
bad() { echo -e "\033[31m[ERROR]\033[0m  $*"; FAIL=1; }
docker image inspect "$IMG" >/dev/null 2>&1 || { bad "image $IMG missing"; exit 1; }
for p in 8075 8074; do ss -ltn | grep -q ":$p " && { bad "port $p is busy"; exit 1; }; done
TMP="$(mktemp -d)"; mkdir -p "$TMP/cache" "$TMP/idle" "$TMP/never" "$TMP/dl"
cp -r "$WS/mountspace/data/myworkspace/projectspace/updown/gocache/." "$TMP/cache/" 2>/dev/null
head -c 40000000 /dev/urandom > "$TMP/dl/big.bin"   # 40 MB: far more than the network buffers hold
conf() { sed "s/^STOP_MODE=.*/STOP_MODE=$1/; s/^IDLE_MINUTES=.*/IDLE_MINUTES=1/; s/^BIND=.*/BIND=auto/; s/^HOST_PORT=.*/HOST_PORT=$2/; s#^CONTROLS_URL=.*#CONTROLS_URL=#" "$PROJECT/updown-agent/updown.conf"; }
conf idle 8075 > "$TMP/idle/updown.conf"; conf never 8074 > "$TMP/never/updown.conf"
chmod -R a+rwX "$TMP"
cleanup() { docker rm -f updown-lt-idle updown-lt-never >/dev/null 2>&1; rm -rf "$TMP"; }
trap cleanup EXIT
run() {  # NAME CONFDIR
    docker run -d --name "$1" --network host --user "$(id -u):$(id -g)" -v "$PROJECT:/updown" -v "$TMP/cache:/gocache" \
        -v "$TMP/dl:/downloads" -v "$2:/agent" -e CONTAINER_WORKDIR=/updown -e LOG_MIRROR_ROOT=/tmp \
        -e UPDOWN_AGENT_CONF=/agent/updown.conf -e UPDOWN_AGENT_STATE=/tmp/s "$IMG" python3 /updown/updown-agent/agent.py >/dev/null
}
link() { curl -s --max-time 5 "http://localhost:$1/host/api/status?after=0" | python3 -c 'import sys,json; print(json.load(sys.stdin)["receive"])' 2>/dev/null; }
wait_link() { for _ in $(seq 1 180); do l="$(link "$1")"; [ -n "$l" ] && { echo "$l"; return 0; }; sleep 1; done; return 1; }
run updown-lt-idle "$TMP/idle"
LI="$(wait_link 8075)" || { bad "the idle updown has no link: $(docker logs updown-lt-idle 2>&1 | tail -2 | tr '\n' ' ')"; exit 1; }

# L1 — a download twice as long as the idle time (40 MB at 300 KB/s ≈ 2¼ min), started the moment updown is up
s=$(date +%s); since=$s
curl -s --limit-rate 300k -o "$TMP/got.bin" "${LI%%/receive*}/api/files/big.bin?t=${LI##*t=}" & CURL=$!
run updown-lt-never "$TMP/never"                                   # L3's updown idles meanwhile
wait $CURL; rc=$?; until_=$(date +%s)
took=$(( $(date +%s) - s ))
during="$(docker logs --since "$since" --until "$until_" updown-lt-idle 2>&1 | grep -c "stopped: idle")"
LN="$(wait_link 8074)" || { bad "the never updown has no link"; exit 1; }
if [ "$rc" = 0 ] && cmp -s "$TMP/got.bin" "$TMP/dl/big.bin" && [ "$took" -ge 90 ] && [ "$during" = 0 ] \
   && docker logs updown-lt-idle 2>&1 | grep -q "download finished: big.bin"; then
    ok "L1 a ${took}s download (idle limit 60s) completed byte for byte — updown did not stop during it"
else bad "L1 stops during the download: $during · curl=$rc took=${took}s same=$(cmp -s "$TMP/got.bin" "$TMP/dl/big.bin" && echo yes || echo no) — $(docker logs updown-lt-idle 2>&1 | grep -E 'stopped|download' | tail -3 | tr '\n' ' ')"; fi

# L2 — afterwards, really idle → it still stops by itself (idle mode not broken)
for _ in $(seq 1 100); do docker logs updown-lt-idle 2>&1 | grep -q "stopped: idle" && break; sleep 2; done
docker logs updown-lt-idle 2>&1 | grep -q "stopped: idle" && ok "L2 once really idle, STOP_MODE=idle still stops by itself" || bad "L2 no idle stop after the transfer"

# L3 — STOP_MODE=never: idle the whole time (now > 3 min) → no stop, the same link
L3="$(link 8074)"
if [ "$L3" = "$LN" ] && ! docker logs updown-lt-never 2>&1 | grep -qE "stopped|starting it again"; then
    ok "L3 STOP_MODE=never: idle for $(( $(date +%s) - s ))s+ — no stop, the same link (same QR codes)"
else bad "L3 link same=$([ "$L3" = "$LN" ] && echo yes || echo no) — $(docker logs updown-lt-never 2>&1 | grep -E 'stopped|again' | tail -2 | tr '\n' ' ')"; fi

# L4 — a long UPLOAD on the idle updown (it restarted in L2: use its new link)
LI="$(wait_link 8075)"; s=$(date +%s)
curl -s --limit-rate 300k -T "$TMP/dl/big.bin" "${LI%%/receive*}/api/upload/up.bin?t=${LI##*t=}" -o "$TMP/up.json"; rc=$?
took=$(( $(date +%s) - s ))
# stops BETWEEN "upload started" and "upload finished" in the log (order, not time: no 1-second overlap with L2's stop)
during="$(docker logs updown-lt-idle 2>&1 | awk '/upload started: up.bin/{on=1} on && /stopped: idle/{n++} /upload finished: up.bin/{on=0} END{print n+0}')"
if [ "$rc" = 0 ] && cmp -s "$TMP/dl/up.bin" "$TMP/dl/big.bin" && [ "$took" -ge 90 ] && [ "$during" = 0 ]; then
    ok "L4 a ${took}s upload (idle limit 60s) was saved complete — updown did not stop during it"
else bad "L4 stops during the upload: $during · curl=$rc took=${took}s same=$(cmp -s "$TMP/dl/up.bin" "$TMP/dl/big.bin" && echo yes || echo no) $(cat "$TMP/up.json" 2>/dev/null | cut -c1-120)"; fi

# L5 — small files (a few KB) both ways, on the never updown
head -c 4096 /dev/urandom > "$TMP/small.txt"; B="${LN%%/receive*}"; T="${LN##*t=}"
curl -s -T "$TMP/small.txt" "$B/api/upload/small.txt?t=$T" -o /dev/null && curl -s -o "$TMP/small_back.txt" "$B/api/files/small.txt?t=$T" \
  && cmp -s "$TMP/small.txt" "$TMP/small_back.txt" && ok "L5 a 4 KB file up and back down: unchanged" || bad "L5 small file round trip failed"

[ "$FAIL" = 0 ] && ok "long-transfer tests passed (throwaway containers removed)" && exit 0
exit 1
