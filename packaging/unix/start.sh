#!/bin/sh
# start.sh — start updown from its release package (Linux, macOS). Copied into every
# release by the build. It only reads release.conf and runs BUILD — no OS detection:
# the package already is for one OS (docs/contract.md §1). Every argument passes through.
#   ./start.sh [--config FILE] [--check] [--version] [FILE ...]
set -eu
HERE="$(cd "$(dirname "$0")" && pwd)"
. "$HERE/release.conf"
exec "$HERE/$BUILD" "$@"
