#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_host_doors.sh — automatic tests of updown's doors: the old bash = the new Python, on a FAKE docker
#
# What for    Prove the sh → py conversion (CLAUDEWORK-514) changed nothing — the same lines, exit codes and docker
#             calls — and that every --help only shows the guide. Never the real updown container.
# Who         Claude (auto test in the branch, then on main) · the owner by hand.
# How         bash projectspace/apps/updown/dockerspace/host_scripts/test_host_doors.sh      (~30 s)
# Cases       H0 doors · H1 status · H2 stop / clean · H3 start · H4 exec / shell / build · H5 pull_images /
#             test_release · H6 container login
# Output      [ PASS ] / [ FAIL ] per check · last line: ALL PASS — old = new   or   N FAILED (exit 1)
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/test_host_doors.py" "$@"
