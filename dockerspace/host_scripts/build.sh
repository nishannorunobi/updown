#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# build.sh — build release packages from the host: container_scripts/build.sh inside updown-dev-container
#
# What for    The host has no Go. Arguments pass through.
# How         bash dockerspace/host_scripts/build.sh --target linux-x64
#             bash dockerspace/host_scripts/build.sh --all
#             bash dockerspace/host_scripts/build.sh --all --test    then the offline Linux test (linux-x64)
# Errors      exit 1 = the build failed (its lines say why) · with --test: test_release.sh's exit code
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/build.py" "$@"
