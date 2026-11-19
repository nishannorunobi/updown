#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_release.sh — a built release folder in CLEAN Linux containers with networking OFF: it runs on a fresh machine
#
# How         bash dockerspace/host_scripts/test_release.sh --target linux-x64
#             bash dockerspace/host_scripts/test_release.sh --target linux-x64 --image debian:12-slim
#             bash dockerspace/host_scripts/test_release.sh --target linux-x64 --cmd "./start.sh --check"
# Options     --target  which release: the newest release/<name>-<version>-<target>/ folder
#             --image   one image (default: every TEST_IMAGES entry in dockerspace/.env)
#             --cmd     what to run inside the release folder (default "./start.sh --check": the release contract)
# Steps       each image: docker run --rm --network none, the release READ-ONLY at /opt/updown, a non-root user
# Errors      exit 0 passed in every image · 1 a failure (listed at the end) · 2 wrong arguments
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/test_release.py" "$@"
