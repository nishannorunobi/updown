#!/bin/bash
# build.sh — build release packages. Runs INSIDE updown-dev-container (the host has no Go):
#     bash dockerspace/host_scripts/build.sh --target linux-x64
#     bash dockerspace/host_scripts/build.sh --all
#     … --skip-tests       (skip go vet + go test before packaging)
#
# For each target it makes, from the project root:
#   release/<RELEASE>/            only the files that OS needs (see files_for below)
#   release/<RELEASE>.zip         the package to hand out
#   release/<RELEASE>.zip.sha256  its checksum
# RELEASE = PRODUCT_NAME-VERSION-TARGET, read from release.conf — the only place for name
# and version. The package's own release.conf adds TARGET, RELEASE and BUILD. Offline:
# pure-Go static binaries (CGO_ENABLED=0), no module downloads (GOPROXY=off).
# Exit 0 = every requested target built; 1 = a failure (the old package is kept).
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

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT" || exit 1
ALL_TARGETS="linux-x64 linux-arm64 windows-x64 macos-arm64 macos-x64"

ok()  { echo -e "\033[32m[  OK  ]\033[0m $*"; }
err() { echo -e "\033[31m[ERROR]\033[0m  $*"; }

TARGETS=""; TESTS=true
while [ $# -gt 0 ]; do
    case "$1" in
        --target) shift; TARGETS="$TARGETS ${1:-}" ;;
        --all) TARGETS="$ALL_TARGETS" ;;
        --skip-tests) TESTS=false ;;
        --help|-h) sed -n '2,15p' "${BASH_SOURCE[0]}"; exit 0 ;;
        *) err "unknown option: $1 (try --help)"; exit 2 ;;
    esac
    shift
done
[ -n "${TARGETS// /}" ] || { err "usage: build.sh --target <$(echo $ALL_TARGETS | tr ' ' '|')> | --all"; exit 2; }

# The source release.conf: PRODUCT_NAME + VERSION.
PRODUCT_NAME=""; VERSION=""
. "$ROOT/release.conf"
[ -n "$PRODUCT_NAME" ] && [ -n "$VERSION" ] || { err "release.conf needs PRODUCT_NAME and VERSION"; exit 1; }

# goos_goarch TARGET → "GOOS GOARCH"
goos_goarch() {
    case "$1" in
        linux-x64) echo "linux amd64" ;;     linux-arm64) echo "linux arm64" ;;
        windows-x64) echo "windows amd64" ;;
        macos-arm64) echo "darwin arm64" ;;  macos-x64) echo "darwin amd64" ;;
        *) return 1 ;;
    esac
}
# files_for TARGET → "source destination" lines: ONLY what that OS needs.
files_for() {
    case "$1" in
        windows-*) echo "packaging/windows/start.cmd start.cmd" ;;
        *)         echo "packaging/unix/start.sh start.sh" ;;
    esac
    echo "packaging/updown.conf updown.conf"
}
crlf() { sed -i 's/\r*$/\r/' "$1"; }   # Windows text files

if $TESTS; then
    echo "==> go vet + go test"
    go vet ./... && go test ./... || { err "tests failed — nothing built"; exit 1; }
fi

FAILED=()
mkdir -p release
for t in $TARGETS; do
    GG="$(goos_goarch "$t")" || { err "unknown target: $t"; FAILED+=("$t"); continue; }
    read -r GOOS_ GOARCH_ <<<"$GG"
    REL="$PRODUCT_NAME-$VERSION-$t"
    BIN="bin/$PRODUCT_NAME"; [ "$GOOS_" = windows ] && BIN="$BIN.exe"
    STAGE="release/.$REL.tmp"
    echo "==> $REL  (GOOS=$GOOS_ GOARCH=$GOARCH_)"
    rm -rf "$STAGE"; mkdir -p "$STAGE/bin"

    if ! GOOS=$GOOS_ GOARCH=$GOARCH_ go build -trimpath -ldflags "-s -w" -o "$STAGE/$BIN" ./cmd/updown; then
        err "$t: go build failed"; rm -rf "$STAGE"; FAILED+=("$t"); continue
    fi
    while read -r src dst; do
        cp "$src" "$STAGE/$dst" || { err "$t: missing $src"; FAILED+=("$t"); continue 2; }
    done < <(files_for "$t")
    { grep -v '^#' release.conf | grep .
      echo "TARGET=$t"; echo "RELEASE=$REL"; echo "BUILD=$BIN"; } > "$STAGE/release.conf"
    if [ "$GOOS_" = windows ]; then
        for f in "$STAGE"/*.cmd "$STAGE"/*.conf; do crlf "$f"; done
    else
        chmod +x "$STAGE/start.sh" "$STAGE/$BIN"
    fi

    # Swap in only when complete: a failed build never leaves a half package.
    rm -rf "release/$REL" && mv "$STAGE" "release/$REL"
    rm -f "release/$REL.zip" "release/$REL.zip.sha256"
    if go run ./tools/mkzip "release/$REL" "release/$REL.zip" \
       && (cd release && sha256sum "$REL.zip" > "$REL.zip.sha256"); then
        ok "$REL — $(du -h "release/$REL/$BIN" | cut -f1) program · $(du -h "release/$REL.zip" | cut -f1) zip · files: $(cd "release/$REL" && find . -type f | sed 's|^\./||' | sort | tr '\n' ' ')"
    else
        err "$t: zip failed"; FAILED+=("$t")
    fi
done

[ ${#FAILED[@]} -eq 0 ] || { err "failed: ${FAILED[*]}"; exit 1; }
ok "built:$TARGETS → release/"
exit 0
