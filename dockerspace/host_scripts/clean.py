#!/usr/bin/env python3
"""clean.py — remove updown's dev CONTAINER (--image: the dev image too). The door: clean.sh. Never the Go base image,
the test images or the Go cache (offline-first). Always exit 0."""
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)


def quiet(*args):
    return subprocess.run(["docker", *args], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0


h.ok(f"{h.C} removed" if quiet("rm", "-f", h.C) else f"{h.C} did not exist")
if sys.argv[1:2] == ["--image"]:
    h.ok(f"{h.IMG} removed" if quiet("rmi", h.IMG) else f"{h.IMG} did not exist")
sys.exit(0)
