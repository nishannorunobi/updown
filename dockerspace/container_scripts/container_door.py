#!/usr/bin/env python3
"""container_door.py — what updown's in-container programs share (build · env_check). ccc is NOT mounted in
the container, so this does here what python_starter does on the host — the same as the old POSIX sh block did:

    help()       -h / --help → the door's header (the same colours as the old awk), exit 0
    say(text)    a line on screen; LOG_MIRROR_ROOT set → stamped [YYYY-MM-DD HH:MM:SS] on screen AND appended to
                 $LOG_MIRROR_ROOT/<the door's folder>/<name>_sh.log (> 10 MB → .1, LOGAN-13) · not set → stdout only
                 (LOGAN-12: Docker keeps it, Fluent Bit ships it — never a hidden file in the container)
Standard library only.
"""
import os
import re
import sys
import time

DOOR = os.path.abspath(os.environ.pop("STARTER", "") or sys.argv[0])
BOLD, CYAN, YELLOW, OFF = "\033[1m", "\033[36m", "\033[33m", "\033[0m"
LOG_FILE = ""


def help():
    if sys.argv[1:2] not in (["-h"], ["--help"]):
        return
    n, titled = 0, False
    for line in open(DOOR, errors="replace").read().splitlines()[1:]:
        if line.startswith("# ─"):
            n += 1
            if n == 2:
                break
            continue
        if n != 1:
            continue
        text = re.sub(r"^# ?", "", line, count=1)
        if not titled:
            print(f"{BOLD}{text}{OFF}")
            titled = True
        elif re.fullmatch(r"[A-Z][A-Za-z ]+", text[:12]):
            print(f"{YELLOW if text.startswith('Errors') else CYAN}{text[:12]}{OFF}{text[12:]}")
        else:
            print(text)
    sys.exit(0)


def _start_log():
    global LOG_FILE
    root = os.environ.get("LOG_MIRROR_ROOT", "")
    if not root:
        return
    workdir = os.environ.get("CONTAINER_WORKDIR", "")
    rel = DOOR[len(workdir) + 1:] if workdir and DOOR.startswith(workdir + "/") else DOOR.lstrip("/")
    rel_dir = os.path.dirname(rel)
    stem, _, ext = os.path.basename(DOOR).rpartition(".")
    LOG_FILE = f"{root}{'/' + rel_dir if rel_dir else ''}/{stem}_{ext}.log"
    try:
        os.makedirs(os.path.dirname(LOG_FILE), exist_ok=True)
        if os.path.getsize(LOG_FILE) > 10485760:
            os.replace(LOG_FILE, LOG_FILE + ".1")
    except OSError:
        pass
    os.environ["LOG_FILE"] = LOG_FILE
    say(f"[logging] → {LOG_FILE}")


def say(text):
    if LOG_FILE:
        text = time.strftime("[%Y-%m-%d %H:%M:%S] ") + text
        try:
            with open(LOG_FILE, "a") as f:
                f.write(text + "\n")
        except OSError:
            pass
    print(text, flush=True)


def run(cmd, input=None, **kw):
    """A child program, every line of its output (stdout + stderr) through say() → its exit code.
    input = text for its stdin (e.g. psql's \\set lines — never a secret on the command line)."""
    import subprocess
    if input is not None:
        kw["stdin"] = subprocess.PIPE
    p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, errors="replace", **kw)
    if input is not None:
        p.stdin.write(input)
        p.stdin.close()
    for line in p.stdout:
        say(line.rstrip("\n"))
    return p.wait()


help()
_start_log()
