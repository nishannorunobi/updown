#!/usr/bin/env python3
"""container_login.py — a shell INSIDE updown's running container. The door: updown_container_login.sh.
The session itself is NOT mirror-logged: the shell gets the real terminal back."""
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)


def die(text):
    h.err(text)
    sys.exit(1)


containers = [h.C]
c = sys.argv[1] if len(sys.argv) > 1 and sys.argv[1] else h.C
if c not in containers:
    die(f"'{c}' is not one of updown's containers: {' '.join(containers)}")
if subprocess.run(["docker", "info"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
    die("Docker is not running (or this login is not in the docker group yet)")
if not subprocess.run(["docker", "ps", "-q", "-f", f"name=^{c}$"], stdout=subprocess.PIPE, text=True).stdout.strip():
    die(f"{c} is not running — start it: bash {h.PROJECT}/start.sh")
try:
    tty = os.open("/dev/tty", os.O_RDWR)
except OSError:
    die("this needs a terminal (run it yourself, not from another script)")
shell = "bash" if subprocess.run(["docker", "exec", c, "sh", "-c", "command -v bash"], stdout=subprocess.DEVNULL,
                                 stderr=subprocess.DEVNULL).returncode == 0 else "sh"
h.ok(f"entering {c} ({shell}) — exit = back here, the container keeps running")
print(f"\033[36m[ INFO ]\033[0m careful: the shared folder and settings are REAL", flush=True)
# The session itself is NOT mirror-logged: the log pipe is not a terminal (no prompt, no keys) and would record
# everything typed — the shell gets the real terminal back.
for fd in (0, 1, 2):
    os.dup2(tty, fd)
os.execvp("docker", ["docker", "exec", "-it", c, shell])
