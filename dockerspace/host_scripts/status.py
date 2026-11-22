#!/usr/bin/env python3
"""status.py — is updown running, and its link? The door: status.sh beside it. --state → one word (running / stopped)."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

h.help_only()
state = "running" if h.running() else "stopped"
if sys.argv[1:2] == ["--state"]:
    print(state)
    sys.exit(0)
print(f"{h.C}: {state}")
if state == "running":
    link = h.link(5)
    if link is not None:
        print(f"link (open in this PC's browser for the QR codes): {link}")
    else:
        print("no link right now — updown is (re)starting; try again in a moment")
