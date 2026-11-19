#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# build.sh — build release packages. Runs INSIDE updown-dev-container (the host has no Go)
#
# How         bash dockerspace/host_scripts/build.sh --target linux-x64      (from the host)
#             bash dockerspace/host_scripts/build.sh --all
#             … --skip-tests       skip go vet + go test before packaging
# Output      release/<RELEASE>/ (only the files that OS needs) · release/<RELEASE>.zip · …zip.sha256
#             RELEASE = PRODUCT_NAME-VERSION-TARGET from release.conf — the only place for name and version
# Offline     pure-Go static binaries (CGO_ENABLED=0), no module downloads (GOPROXY=off)
# Errors      exit 0 every requested target built · 1 a failure (the old package is kept) · 2 wrong arguments
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/build.py" "$@"
