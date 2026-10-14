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
case "${1:-}" in -h|--help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;; esac
set -uo pipefail
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then source "$_WS_ROOT/init/create_logging_path.sh"; setup_logging; fi

PROJECT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
T=updown-controls-test; IMG=updown-dev-image:1.0; FAIL=0; PAGE=http://localhost:8077; BTN=http://localhost:8068
ok()  { echo -e "\033[32m[  OK  ]\033[0m $*"; }
bad() { echo -e "\033[31m[ERROR]\033[0m  $*"; FAIL=1; }
docker image inspect "$IMG" >/dev/null 2>&1 || { bad "image $IMG missing — bash start.sh --build"; exit 1; }
for p in 8077 8068; do ss -ltn | grep -q "127.0.0.1:$p \|:$p " && { bad "port $p is busy — this test needs it"; exit 1; }; done
TMP="$(mktemp -d)"; mkdir -p "$TMP/dl" "$TMP/cache" "$TMP/agent"
cp -r "$_WS_ROOT/mountspace/data/myworkspace/projectspace/updown/gocache/." "$TMP/cache/" 2>/dev/null   # a warm cache: offline build
sed 's/^BIND=.*/BIND=auto/; s/^HOST_PORT=.*/HOST_PORT=8077/; s#^CONTROLS_URL=.*#CONTROLS_URL=http://localhost:8068#; s/^PAGE_PARTS=.*/PAGE_PARTS=qr_codes,connection,nope/' \
    "$PROJECT/updown-agent/updown.conf" > "$TMP/agent/updown.conf"
cp -r "$PROJECT/updown-agent/actions" "$PROJECT/updown-agent/actions.conf" "$TMP/agent/"
printf 'broken | ✖ | Broken | broken.py | yes\nhidden | ✖ | Hidden | check_now.py | no\n' >> "$TMP/agent/actions.conf"
echo 'raise ImportError("this file is broken on purpose")' > "$TMP/agent/actions/broken.py"
chmod -R a+rwX "$TMP"
cleanup() { docker rm -f "$T" >/dev/null 2>&1; rm -rf "$TMP"; }
trap cleanup EXIT
js() { python3 -c "import sys,json; d=json.loads(sys.argv[2]); print($1)" "$1" "$2" 2>/dev/null; }   # js EXPR JSON
status() { curl -s --max-time 5 "$PAGE/host/api/status?after=0"; }

docker rm -f "$T" >/dev/null 2>&1
docker run -d --name "$T" --network host --user "$(id -u):$(id -g)" \
    -v "$PROJECT:/updown" -v "$TMP/cache:/gocache" -v "$TMP/dl:/downloads" -v "$TMP/agent:/agent" \
    -e CONTAINER_WORKDIR=/updown -e LOG_MIRROR_ROOT=/tmp -e UPDOWN_AGENT_CONF=/agent/updown.conf -e UPDOWN_AGENT_ACTIONS=/agent/actions.conf \
    -e UPDOWN_AGENT_STATE=/tmp/updown-controls-test \
    "$IMG" python3 /updown/updown-agent/agent.py >/dev/null || { bad "throwaway container did not start"; exit 1; }
docker exec "$T" python3 /updown/updown-agent/agent.py link 180 >/dev/null || { bad "updown did not start: $(docker logs "$T" 2>&1 | tail -3 | tr '\n' ' ')"; exit 1; }

# C1 — the page's blocks
lay="$(curl -s --max-time 5 "$PAGE/host/api/page")"
[ "$(js "','.join(d['parts'])+' '+d['controls_url']" "$lay")" = "qr_codes.html,connection.html http://localhost:8068" ] \
  && docker logs "$T" 2>&1 | grep -q 'WARN.*PAGE_PARTS: no block "nope"' \
  && ok "C1 the page's blocks from PAGE_PARTS: qr_codes, connection (\"nope\" left out with a WARN)" || bad "C1 layout: $lay"

# C2 + C3 — the buttons from actions.conf
acts="$(curl -s --max-time 10 "$BTN/actions")"
got="$(js "' '.join(a['name']+('['+'/'.join(o['value'] for o in a['options'])+']' if a['options'] else '') for a in d['actions'])" "$acts")"
[ "$got" = "network_mode[hotspot/auto] check_now new_link broken" ] && ok "C2 the buttons from actions.conf: $got (SHOWN=no hidden)" || bad "C2 got [$got] — $acts"
got="$(js "[(a['ok'],a['reason']) for a in d['actions'] if a['name']=='broken'][0]" "$acts")"
case "$got" in "(False, 'this file is broken on purpose')") ok "C3 a broken action file greys out only its button: $got" ;; *) bad "C3 got [$got]" ;; esac

# C4 — Check now
r="$(curl -s --max-time 20 -X POST "$BTN/action/check_now")"
[ "$(js "d['ok']" "$r")" = True ] && ok "C4 ⟳ Check now: $(js "d['message']" "$r")" || bad "C4 $r"

# C5 — New link
before="$(js "d['send']" "$(status)")"
r="$(curl -s --max-time 20 -X POST "$BTN/action/new_link")"; sleep 1
after="$(js "d['send']" "$(status)")"
if [ -z "$before" ]; then
  [ "$(js "d['ok']" "$r")" = False ] && ok "C5 🔄 Refresh QR code without a phone network → refused: $(js "d['message']" "$r")" || bad "C5 $r"
else
  [ "$(js "d['ok']" "$r")" = True ] && [ -n "$after" ] && [ "$after" != "$before" ] && ok "C5 🔄 Refresh QR code → a new token, the page's link changed" || bad "C5 $r (link changed: $([ "$after" != "$before" ] && echo yes || echo no))"
fi

# C6 — no other web site
a="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 -X POST -H 'Origin: https://evil.example' "$BTN/action/check_now")"
u="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 -X POST -H 'Origin: https://evil.example' "$PAGE/host/api/check")"
h="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 -H 'Host: evil.example:8068' "$BTN/actions")"
p="$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST -H 'Origin: http://localhost:8077' "$BTN/action/check_now")"
[ "$a$u$h$p" = 403403403200 ] && ok "C6 another web site: agent 403 · updown 403 · other Host 403 — the page itself 200" || bad "C6 agent=$a updown=$u host=$h page=$p"

# C7 — the switch
r="$(curl -s --max-time 20 -X POST "$BTN/action/network_mode?value=hotspot")"
s=$(date +%s); until docker logs "$T" 2>&1 | grep -q 'restarting updown with BIND=hotspot'; do sleep 1; [ $(( $(date +%s) - s )) -gt 30 ] && break; done
sleep 3
on="$(js "[o['value'] for o in d['actions'][0]['options'] if o['on']]" "$(curl -s --max-time 10 "$BTN/actions")")"
grep -qx 'BIND=hotspot' "$TMP/agent/updown.conf" && [ "$on" = "['hotspot']" ] && [ "$(js "d['ok']" "$r")" = True ] \
  && docker exec "$T" python3 /updown/updown-agent/agent.py link 60 >/dev/null && [ -n "$(status)" ] \
  && ok "C7 the switch → BIND=hotspot in updown.conf, updown restarted, the page answers, the switch shows it" || bad "C7 $r on=$on"

# C8 — a real browser (curl ignores the page's CSP; a browser does not)
CH="$(command -v google-chrome chromium chromium-browser 2>/dev/null | head -1)"
if [ -z "$CH" ]; then echo "[ SKIP ] C8 no Chrome / Chromium on this PC"
else
  n="$(timeout 60 "$CH" --headless=new --disable-gpu --no-sandbox --virtual-time-budget=8000 --dump-dom "$PAGE/" 2>/dev/null | grep -o 'data-a="[a-z_]*"' | sort -u | tr '\n' ' ')"
  [ "$n" = 'data-a="broken" data-a="check_now" data-a="network_mode" data-a="new_link" ' ] && ok "C8 a real browser draws the buttons: $n" || bad "C8 the browser shows [$n]"
fi

[ "$FAIL" = 0 ] && ok "page button tests passed (throwaway container removed)" && exit 0
docker logs "$T" 2>&1 | tail -15
grep -q '📶' <<< "$acts" || echo "(icons not UTF-8 in: $acts)"
exit 1
