#!/bin/bash
# run.sh — run the updown server INSIDE the dev container for a real phone test. Nothing
# runs on the host: the server listens on all of the container's interfaces, Docker's
# published ports 8070-8079 carry the phone's requests in, and the QR codes show THIS PC's
# phone-reachable address (ADVERTISE). The console prints the host page link + the event
# log; open that link in this PC's browser to see the two QR codes — "Send files" (phone's
# upload screen) and "Receive files" (phone's download screen) — never drawn in the console.
#
#     bash dockerspace/host_scripts/run.sh                      # STOP_MODE=enter
#     bash dockerspace/host_scripts/run.sh --stop after-transfer
#     bash dockerspace/host_scripts/run.sh --stop idle --idle 5
#     bash dockerspace/host_scripts/run.sh --advertise 192.168.1.23
#
# Files: this PC's Downloads folder (mounted at /downloads) — uploads from the phone land
#        there, and the Receive screen lists its files.
# Address order (same as the program's BIND=auto): phone hotspot 172.20.10.x / USB tether
# (enx*, usb*, rndis*) / Android 192.168.43.x → Wi-Fi (wl*) → other private LAN.
set -uo pipefail

# ── Mirror logging ─────────────────────────────────────────────────────────────
_WS_ROOT="$(d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; while [ ! -d "$d/mountspace" ] && [ "$d" != "/" ]; do d="$(dirname "$d")"; done; echo "$d")"
if [ -f "$_WS_ROOT/init/create_logging_path.sh" ]; then
    source "$_WS_ROOT/init/create_logging_path.sh"
    setup_logging
fi
# ──────────────────────────────────────────────────────────────────────────────

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
C=updown-dev-container
CONF=/tmp/updown-dev.conf   # inside the container
STOP=enter; IDLE=15; AD=""
while [ $# -gt 0 ]; do
    case "$1" in
        --stop) shift; STOP="${1:-enter}" ;;
        --idle) shift; IDLE="${1:-15}" ;;
        --advertise) shift; AD="${1:-}" ;;
        --help|-h) sed -n '2,17p' "${BASH_SOURCE[0]}"; exit 0 ;;
        *) echo "unknown option: $1 (try --help)"; exit 2 ;;
    esac
    shift
done

# This PC's phone-reachable address.
if [ -z "$AD" ]; then
    best=""; rank=9
    while read -r ifc ip; do
        case "$ifc" in lo|docker*|br-*|veth*|virbr*|tun*|tap*|wg*) continue ;; esac
        r=9
        if [[ "$ip" =~ ^172\.20\.10\. || "$ip" =~ ^192\.168\.43\. || "$ifc" =~ ^(enx|usb|rndis) ]]; then r=0
        elif [[ "$ifc" == wl* ]]; then r=1
        elif [[ "$ip" =~ ^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.) ]]; then r=2
        fi
        [ "$r" -lt "$rank" ] && { rank=$r; best="$ip"; }
    done < <(ip -4 -o addr show 2>/dev/null | awk '{split($4, a, "/"); print $2, a[1]}')
    AD="$best"
fi
[ -n "$AD" ] || { echo -e "\033[31m[ERROR]\033[0m  no phone-reachable network — connect this PC to the phone hotspot (USB or Wi-Fi) or the same Wi-Fi"; exit 1; }

# The container must run and have the /downloads mount (added after the first start).
bash "$HERE/start.sh" >/dev/null || exit 1
if ! docker exec "$C" test -d /downloads; then
    echo "==> recreating $C to add the /downloads mount"
    docker rm -f "$C" >/dev/null && bash "$HERE/start.sh" >/dev/null || exit 1
fi
printf '%s\n' "# written by dockerspace/host_scripts/run.sh — dev run inside the container" \
    "RECEIVE_DIR=/downloads" "SHARE_DIR=/downloads" "STOP_MODE=$STOP" "IDLE_MINUTES=$IDLE" \
    "PORT=0" "BIND=0.0.0.0" "ADVERTISE=$AD" "MAX_FILE_GB=10" \
    | docker exec -i "$C" sh -c "cat > $CONF" || exit 1

HOST_DL="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/downloads"}}{{.Source}}{{end}}{{end}}' "$C")"
echo "==> phone address: $AD · uploads + Receive screen: $HOST_DL"
# A terminal on stdin → -t: Enter and Ctrl+C reach the server inside the container
# (stdout may be the mirror-log pipe; -t only needs stdin to be a terminal).
TTY=(-i); [ -t 0 ] && TTY=(-it)
# stop_server — end a dev server still running in the container (a closed terminal or a
# killed run.sh does not reach it). Used before starting and on every exit.
stop_server() {
    docker exec "$C" sh -c 'for p in /proc/[0-9]*; do
        case "$(tr "\0" " " < "$p/cmdline" 2>/dev/null)" in "/tmp/updown --config /tmp/updown-dev.conf"*) kill "${p#/proc/}" ;; esac
    done' 2>/dev/null || true
}
stop_server
trap stop_server EXIT
# Build once inside the container, then run that binary.
docker exec -w /updown "$C" go build -o /tmp/updown ./cmd/updown || exit 1
docker exec "${TTY[@]}" -w /updown "$C" /tmp/updown --config "$CONF"
