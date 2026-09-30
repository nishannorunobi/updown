#!/bin/bash
# env_check.sh — runs INSIDE updown-dev-container: proves the dev environment is ready.
#     bash dockerspace/host_scripts/exec.sh bash dockerspace/container_scripts/env_check.sh
# Checks: Go present and pinned offline (GOTOOLCHAIN=local, GOPROXY=off), caches writable,
# and a tiny pure-Go program cross-compiles for every product target with no network.
# Exit 0 = ready; 1 = something is wrong (listed).
set -uo pipefail

# ── Mirror logging (container) ────────────────────────────────────────────────
_SELF_ABS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
_BASE="$(basename "$_SELF_ABS")"; _EXT="${_BASE##*.}"; _STEM="${_BASE%.*}"
_REL_DIR="$(dirname "${_SELF_ABS#${CONTAINER_WORKDIR:-}/}")"
[ "$_REL_DIR" = "." ] && _REL_DIR="" || _REL_DIR="/$_REL_DIR"
LOG_FILE="${LOG_MIRROR_ROOT:-/tmp/logs}${_REL_DIR}/${_STEM}_${_EXT}.log"
mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true; touch "$LOG_FILE" 2>/dev/null || true; export LOG_FILE
exec > >(awk '{ print strftime("[%Y-%m-%d %H:%M:%S]"), $0; fflush() }' | tee -a "$LOG_FILE") 2>&1
echo "[logging] → $LOG_FILE"
# ──────────────────────────────────────────────────────────────────────────────

FAIL=()
ok()  { echo -e "\033[32m[  OK  ]\033[0m $*"; }
bad() { echo -e "\033[31m[ERROR]\033[0m  $*"; FAIL+=("$1"); }

command -v go >/dev/null && ok "$(go version)" || bad "go not found"
[ "$(go env GOTOOLCHAIN)" = local ] && ok "GOTOOLCHAIN=local (never downloads a toolchain)" || bad "GOTOOLCHAIN is $(go env GOTOOLCHAIN)"
[ "$(go env GOPROXY)" = off ] && ok "GOPROXY=off (never fetches modules)" || bad "GOPROXY is $(go env GOPROXY)"
[ "$(go env CGO_ENABLED)" = 0 ] && ok "CGO_ENABLED=0 (static binaries)" || bad "CGO_ENABLED is $(go env CGO_ENABLED)"
for d in "$(go env GOCACHE)" "$(go env GOPATH)"; do
    mkdir -p "$d" 2>/dev/null && [ -w "$d" ] && ok "writable: $d" || bad "not writable: $d"
done

# Cross-compile a throwaway program for every product target (no network needed).
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
printf 'module check\n\ngo 1.25\n' > "$T/go.mod"
printf 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("ok") }\n' > "$T/main.go"
for tgt in linux/amd64 linux/arm64 windows/amd64 darwin/arm64 darwin/amd64; do
    if (cd "$T" && GOOS="${tgt%/*}" GOARCH="${tgt#*/}" go build -o "$T/out-${tgt/\//-}" . 2>&1); then
        ok "cross-compile $tgt"
    else
        bad "cross-compile $tgt"
    fi
done
[ "$(cd "$T" && go run .)" = ok ] && ok "go run works" || bad "go run"

[ ${#FAIL[@]} -eq 0 ] || { echo -e "\033[31m[ERROR]\033[0m  not ready: ${FAIL[*]}"; exit 1; }
echo -e "\033[32m[  OK  ]\033[0m dev environment ready — Go builds for Linux, Windows and macOS, offline"
exit 0
