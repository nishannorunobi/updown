#!/usr/bin/env python3
"""stop.py — stop updown's dev container by its EXPLICIT name (never compose-wide). Container + image kept.
The doors: ../../stop.sh and stop.sh here. Not running → says so, exit 0."""
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
if h.C not in h.names():
    h.ok(f"{h.C} already stopped")
    sys.exit(0)
if subprocess.run(["docker", "stop", h.C], stdout=subprocess.DEVNULL).returncode == 0:
    h.ok(f"{h.C} stopped")
    sys.exit(0)
h.err(f"could not stop {h.C}")
sys.exit(1)
