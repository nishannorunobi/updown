#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# pull_images.sh — the ONE online step: download every image updown needs, so everything after works offline
#
# What for    GO_IMAGE and TEST_IMAGES from dockerspace/.env; images already here are skipped (no network for them).
# How         bash dockerspace/host_scripts/pull_images.sh
# Errors      exit 1 = an image is still missing (offline? wrong tag?), listed at the end
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/pull_images.py" "$@"
