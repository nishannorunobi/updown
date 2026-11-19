#!/usr/bin/env python3
"""updown_host.py — what updown's host programs share (start · stop · status · clean · exec · shell · build · tests).

    HERE · DOCKERSPACE · PROJECT · WS_ROOT   this folder · dockerspace/ (compose) · the project · the workspace
                                             ("/" when none, as the bash loop)
    C · IMG                                  updown-dev-container · updown-dev-image:1.0
    starter                                  ccc's python_starter (--help + the mirror log) — None in a temp copy
    help_only()                              --help for the programs that keep no log (the others call
                                             starter.start() themselves, in __main__ — it acts only there)
    running()                                is updown-dev-container running?
    env_values()                             dockerspace/.env as KEY → value (quotes off), what `source .env` gave
    link(seconds)                            the link updown-agent hands out inside the container ('' = none yet)
    ok / warn / err                          the coloured lines the bash printed
Standard library only.
"""
import os
import pathlib
import re
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
DOCKERSPACE = HERE.parent
PROJECT = DOCKERSPACE.parent
WS_ROOT = next((str(p) for p in HERE.parents if (p / "mountspace").is_dir()), "/")
C = "updown-dev-container"
IMG = "updown-dev-image:1.0"
sys.path[:0] = [str(pathlib.Path(WS_ROOT) / "ccc/scripts/lib/python_starter")]
try:
    import starter
except ImportError:                       # outside a workspace (a temp copy): no --help, no mirror log — as the bash
    starter = None


def help_only():
    if starter and sys.argv[1:2] in (["-h"], ["--help"]):
        print(starter.header_help(os.environ.get("STARTER", "")))
        sys.exit(0)


def ok(text):
    print(f"\033[32m[  OK  ]\033[0m {text}", flush=True)


def warn(text):
    print(f"\033[33m[WARN]\033[0m   {text}", flush=True)


def err(text, file=None):
    print(f"\033[31m[ERROR]\033[0m  {text}", file=file or sys.stdout, flush=True)


def names(*ps_args):
    r = subprocess.run(["docker", "ps", *ps_args, "--format", "{{.Names}}"], stdout=subprocess.PIPE,
                       stderr=subprocess.DEVNULL, text=True)
    return r.stdout.split()


def running():
    return C in names("--filter", "status=running")


def env_values():
    values = {}
    for line in (DOCKERSPACE / ".env").read_text(errors="replace").splitlines():
        m = re.match(r"^([A-Za-z_][A-Za-z0-9_]*)=(.*)$", line)
        if m:
            values[m.group(1)] = m.group(2).strip().strip('"').strip("'")
    return values


def link(seconds):
    """The link from updown-agent inside the container (it builds updown first: up to ~1 minute on a cold cache)."""
    r = subprocess.run(["docker", "exec", C, "python3", "/updown/updown-agent/agent.py", "link", str(seconds)],
                       stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    return r.stdout.rstrip("\n") if r.returncode == 0 else None
