#!/usr/bin/env python3
"""shell.py — an interactive bash inside updown's dev container, in /updown. The door: shell.sh."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

h.help_only()
if not h.running():
    h.err(f"{h.C} is not running — start it: bash start.sh", file=sys.stderr)
    sys.exit(1)
os.execvp("docker", ["docker", "exec", "-it", "-w", "/updown", h.C, "bash"])
