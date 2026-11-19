#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# clean.sh — remove updown's dev CONTAINER (the next start.sh recreates it from the image)
#
# How         bash dockerspace/host_scripts/clean.sh            container only
#             bash dockerspace/host_scripts/clean.sh --image    also the dev image (a rebuild needs GO_IMAGE, kept offline)
# Careful     offline-first: never the Go base image, the test images or the Go cache in mountspace — those would
#             need the internet to get back
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/clean.py" "$@"
