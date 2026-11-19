#!/usr/bin/env python3
"""pull_images.py — the ONE online step: every image updown needs (GO_IMAGE + TEST_IMAGES of dockerspace/.env); those
already here are skipped. The door: pull_images.sh. Exit 1 = an image still missing (listed)."""
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
env = h.env_values()
missing = []
for img in (env.get("GO_IMAGE", "") + " " + env.get("TEST_IMAGES", "")).split():
    if subprocess.run(["docker", "image", "inspect", img], stdout=subprocess.DEVNULL,
                      stderr=subprocess.DEVNULL).returncode == 0:
        h.ok(f"{img} — already here")
    elif subprocess.run(["docker", "pull", "-q", img], stdout=subprocess.DEVNULL).returncode == 0:
        h.ok(f"{img} — pulled")
    else:
        h.err(f"{img} — could not pull")
        missing.append(img)
if missing:
    h.err(f"still missing: {' '.join(missing)}")
    sys.exit(1)
sys.exit(0)
