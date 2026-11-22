#!/usr/bin/env python3
"""start.py — start updown's container (docker compose) from its EXISTING image and show the link updown-agent hands
out. The door: start.sh beside it (--build = rebuild the image first).

Steps
  1. the bind-mount folders exist and belong to you (else Docker makes them root-owned)
  2. this PC's Downloads folder (XDG, else ~/Downloads) → mounted at /downloads
  3. the pod guard: ../../../../mountspace from dockerspace/ must be THIS workspace's (CLAUDEWORK-258)
  4. the mirror-log volume handed to you (chown, once per start, idempotent)
  5. docker compose up -d [--build] — fails → exit 1
  6. the link (≤ 90 s) → OK + the link · no link but running → WARN · not running → ERROR exit 1
"""
import os
import pathlib
import re
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
C = h.C
ws = h.PROJECT / "../../../mountspace"
for folder in (ws / "data/myworkspace/projectspace/updown/gocache", ws / "logs/myworkspace/projectspace/apps/updown"):
    try:
        folder.mkdir(parents=True, exist_ok=True)
    except OSError:
        pass

uid, gid = str(os.getuid()), str(os.getgid())
home = os.environ.get("HOME", "")
downloads = ""
try:
    dirs = pathlib.Path(home, ".config/user-dirs.dirs").read_text(errors="replace")
    found = re.findall(r'^XDG_DOWNLOAD_DIR="(.*)"', dirs, re.M)
    downloads = found[0].replace("$HOME", home, 1) if found else ""
except OSError:
    pass
downloads = downloads or f"{home}/Downloads"
os.environ.update({"HOST_UID": uid, "HOST_GID": gid, "HOST_DOWNLOADS": downloads})
subprocess.run(["mkdir", "-p", downloads])
os.chdir(h.DOCKERSPACE)
# this pod sits at its folder of ccc/scripts/gitmgt/repos.conf: its relative mounts reach THIS workspace's mountspace/
# — else Docker would quietly make EMPTY data folders elsewhere (CLAUDEWORK-258)
if os.path.realpath("../../../../mountspace") != os.path.realpath(f"{h.WS_ROOT}/mountspace") \
        or not os.path.isdir("../../../../mountspace"):
    h.err(f"../../../../mountspace from {h.DOCKERSPACE} is not this workspace's mountspace/ — this pod is not at its "
          "folder (ccc/scripts/gitmgt/repos.conf): bash ccc/scripts/gitmgt/move_repos_to_new_folders.sh")
    sys.exit(1)
# the mirror-log volume (LOGAN-13) is created root-owned — hand it to the user the container runs as (once per start,
# idempotent; compose creates the volume with its labels and builds the image when missing)
if subprocess.run(["docker", "compose", "run", "--rm", "--no-deps", "--user", "0", "--entrypoint", "chown", "updown-dev",
                   f"{uid}:{gid}", "/mountspace/logs"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
    h.err(f"the log volume (/mountspace/logs) could not be handed to {uid}:{gid} — docker compose run updown-dev")
    sys.exit(1)
if sys.argv[1:2] == ["--build"]:
    print("==> Rebuilding updown-dev-image, then starting…", flush=True)
    up = ["docker", "compose", "up", "-d", "--build"]
else:
    up = ["docker", "compose", "up", "-d"]
if subprocess.run(up).returncode:
    sys.exit(1)

link = h.link(90)
if link is not None:
    h.ok(f"{C} running — updown is ready")
    print("         open this link in this PC's browser for the QR codes (send / receive files):")
    print(f"         \033[1m{link}\033[0m")
    print("         this link never changes — bookmark it; the QR codes on it follow the phone's network", flush=True)
    sys.exit(0)
if h.running():
    h.warn(f"{C} running, but updown gave no link yet — see: docker logs {C}")
    sys.exit(0)
h.err(f"{C} did not come up (docker logs {C})")
sys.exit(1)
