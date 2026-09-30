#!/bin/bash
# build.sh — build release packages from the host: runs container_scripts/build.sh inside
# updown-dev-container (the host has no Go). Arguments pass through.
#     bash dockerspace/host_scripts/build.sh --target linux-x64
#     bash dockerspace/host_scripts/build.sh --all
#     bash dockerspace/host_scripts/build.sh --all --test    # then the offline Linux test
# --test: after building, runs test_release.sh for linux-x64 (the images here are x86-64).
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARGS=(); TEST=false
for a in "$@"; do [ "$a" = --test ] && TEST=true || ARGS+=("$a"); done
bash "$HERE/exec.sh" bash dockerspace/container_scripts/build.sh "${ARGS[@]}" || exit 1
$TEST || exit 0
exec bash "$HERE/test_release.sh" --target linux-x64
