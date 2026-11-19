#!/usr/bin/env python3
"""build.py — build release packages from the host: container_scripts/build.sh inside updown-dev-container (the host
has no Go). Arguments pass through; --test = afterwards test_release.sh for linux-x64. The door: build.sh."""
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

h.help_only()
args = [a for a in sys.argv[1:] if a != "--test"]
if subprocess.run(["bash", str(h.HERE / "exec.sh"), "bash", "dockerspace/container_scripts/build.sh", *args]).returncode:
    sys.exit(1)
if "--test" not in sys.argv[1:]:
    sys.exit(0)
os.execvp("bash", ["bash", str(h.HERE / "test_release.sh"), "--target", "linux-x64"])
