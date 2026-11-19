#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# test_long_transfer.sh — long transfers are never cut off; STOP_MODE=never keeps running ([2.8.1], UPDOWN-86)
#
# What for    A big file can take hours. Before this fix updown counted only the START of a request as activity,
#             so after IDLE_MINUTES it stopped in the middle of a download (cut file, new QR code).
# Checks      L1 STOP_MODE=idle, IDLE_MINUTES=1: a 40 MB download slowed to ~2¼ minutes (twice the idle time) completes,
#                byte for byte, and updown did not stop during it
#             L2 the same updown still stops by itself once it is really idle afterwards (idle mode still works)
#             L3 STOP_MODE=never, IDLE_MINUTES=1: idle for 2+ minutes → no stop, the same link (same QR codes)
#             L4 the same for an UPLOAD (phone → PC, 40 MB slowed to ~2¼ min): saved complete, no stop during it
#             L5 a small file (a few KB) both ways: unchanged, nothing special needed
# How         bash dockerspace/host_scripts/test_long_transfer.sh        (~7 minutes; two THROWAWAY containers on
#             their own ports 8075 / 8074 — runs beside the real updown, never touches it)
# Exit        0 = all passed
# ─────────────────────────────────────────────────────────────────────────────
STARTER="$0" exec python3 -u "$(dirname "$0")/test_long_transfer.py" "$@"
