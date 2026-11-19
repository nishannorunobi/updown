#!/usr/bin/env python3
"""test_release.py — a built release folder in CLEAN Linux containers with networking OFF: it installs and runs on a
fresh machine without internet. The door: test_release.sh (its --help).

    --target  the newest release/<name>-<version>-<target>/ folder (version order)
    --image   one image (default: every TEST_IMAGES entry of dockerspace/.env)
    --cmd     what to run inside the release folder (default "./start.sh --check")
Each run: docker run --rm --network none, the release READ-ONLY at /opt/updown, user 65534. Exit 0 = passed in every
image · 1 = a failure (listed) · 2 = wrong arguments.
"""
import os
import re
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
target, images, cmd = "", h.env_values().get("TEST_IMAGES", ""), "./start.sh --check"
args = sys.argv[1:]
while args:
    a = args.pop(0)
    if a == "--target":
        target = args.pop(0) if args else ""
    elif a == "--image":
        images = args.pop(0) if args else ""
    elif a == "--cmd":
        cmd = args.pop(0) if args else ""
    else:
        print(f"unknown option: {a} (try --help)", flush=True)
        sys.exit(2)
if not target:
    print("usage: test_release.sh --target <os-arch> [--image IMG] [--cmd CMD]", flush=True)
    sys.exit(2)


def version_key(path):                     # sort -V: numbers as numbers
    return [int(p) if p.isdigit() else p for p in re.split(r"(\d+)", path.name)]


found = sorted((p for p in (h.PROJECT / "release").glob(f"*-{target}")), key=version_key)
rel = found[-1] if found else None
if not rel or not rel.is_dir():
    h.err(f"no release/*-{target} folder — build it first")
    sys.exit(1)
print(f"==> Release: {rel}", flush=True)

failed = []
for img in images.split():
    if subprocess.run(["docker", "image", "inspect", img], stdout=subprocess.DEVNULL,
                      stderr=subprocess.DEVNULL).returncode:
        h.err(f"{img} not on this machine — run host_scripts/pull_images.sh once (online)")
        failed.append(img)
        continue
    print(f"==> {img}  (network: none)", flush=True)
    if subprocess.run(["docker", "run", "--rm", "--network", "none", "--user", "65534:65534", "-v", f"{rel}:/opt/updown:ro",
                       "-w", "/opt/updown", img, "sh", "-c", cmd]).returncode == 0:
        h.ok(f"{img} — passed")
    else:
        h.err(f"{img} — failed")
        failed.append(img)
if failed:
    h.err(f"failed in: {' '.join(failed)}")
    sys.exit(1)
h.ok(f"{rel.name} passed offline in: {images}")
sys.exit(0)
