#!/usr/bin/env python3
"""exec_in.py — run ONE command inside updown's dev container, in /updown. The door: exec.sh.
Exit = the command's exit code · 1 the container is not running · 2 no command given."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

h.help_only()
if not h.running():
    h.err(f"{h.C} is not running — start it: bash start.sh", file=sys.stderr)
    sys.exit(1)
if not sys.argv[1:]:
    print("usage: exec.sh COMMAND [ARGS…]", file=sys.stderr)
    sys.exit(2)
os.execvp("docker", ["docker", "exec", "-w", "/updown", h.C, *sys.argv[1:]])
