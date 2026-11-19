#!/usr/bin/env python3
"""test_agent.py — updown-agent ([2.1], UPDOWN-26) on a THROWAWAY container. The door: test_agent.sh (its --help).

The same image, command and network as docker-compose.yml, but temp folders for Downloads + Go cache and a 1-minute
idle stop. The real updown-dev-container is never touched — it must be stopped (both use ports 8070-8079).
A1 link · A2 page 200 · A3 crash → same link · A4 idle stop → same link · A7 the connection · A8 no phone door (red
state) · A5 docker stop → exit 0 · A6 no phone token in the output.
"""
import os
import shutil
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import test_kit as k
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
T, IMG = "updown-agent-test", h.IMG
if h.C in h.names():
    k.bad("stop updown first (bash stop.sh) — the test needs ports 8070-8079")
    sys.exit(1)
if k.docker("image", "inspect", IMG)[0]:
    k.bad(f"image {IMG} missing — bash start.sh --build")
    sys.exit(1)
TMP = k.open_tmp("dl", "cache")
k.warm_cache(h.WS_ROOT, f"{TMP}/cache")
# any network (no hotspot needed); its own page port
open(f"{TMP}/test.conf", "w").write(k.conf(open(h.PROJECT / "updown-agent/updown.conf").read(), STOP_MODE="idle",
                                           IDLE_MINUTES="1", BIND="auto", HOST_PORT="8079",
                                           CONTROLS_URL="http://localhost:8067"))
UID = f"{os.getuid()}:{os.getgid()}"


def link(seconds=60):
    r = subprocess.run(["docker", "exec", T, "python3", "/updown/updown-agent/agent.py", "link", str(seconds)],
                       stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    return r.stdout.strip()


def connection(body):
    return (k.js(body) or {}).get("connection") or {}


try:
    k.docker("rm", "-f", T)
    if k.docker("run", "-d", "--name", T, "--network", "host", "--user", UID, "-v", f"{h.PROJECT}:/updown",
                "-v", f"{TMP}/cache:/gocache", "-v", f"{TMP}/dl:/downloads", "-v", f"{TMP}/test.conf:/tmp/test.conf:ro",
                "-e", "CONTAINER_WORKDIR=/updown", "-e", "LOG_MIRROR_ROOT=/tmp", "-e", "UPDOWN_AGENT_CONF=/tmp/test.conf",
                IMG, "python3", "/updown/updown-agent/agent.py")[0]:
        k.bad("throwaway container did not start")
        sys.exit(1)

    # A1 + A2
    L1 = link(180)
    if L1 == "http://localhost:8079":
        k.ok(f"A1 the agent built + started updown, link: {L1}")
    else:
        k.bad(f"A1 link [{L1}] (docker logs: {' '.join(k.logs(T).splitlines()[-2:])})")
    k.ok("A2 the link opens the host page (200)") if k.http(L1)[0] == 200 else k.bad("A2 host page not 200")

    # A3 — a crash
    k.docker("exec", T, "sh", "-c", 'for p in /proc/[0-9]*; do case "$(tr "\\0" " " < $p/cmdline 2>/dev/null)" in '
             '/tmp/updown-agent/updown*) kill -9 ${p#/proc/} ;; esac; done')
    time.sleep(4)
    L2 = link(60)
    time.sleep(3)
    if L2 == L1 and k.http(L1)[0] == 200:
        k.ok("A3 updown killed → started again, the same link answers")
    else:
        k.bad(f"A3 L2=[{L2}]")

    # A4 — idle stop
    s = time.time()
    while "stopped by itself" not in k.logs(T):
        time.sleep(5)
        if time.time() - s > 150:
            break
    time.sleep(4)
    L3 = link(60)
    if L3 == L1:
        k.ok(f"A4 1 idle minute → updown stopped by itself, started again, the same link ({int(time.time() - s)}s)")
    else:
        k.bad(f"A4 L3=[{L3}]")

    # A7 — the page reports the connection
    st = k.http(f"{L1}/host/api/status?after=0")[1]
    c = connection(st)
    if c.get("state") in ("hotspot", "lan") and c.get("via") and c.get("pc"):
        k.ok(f"A7 the page reports the connection: {c['state']} over {c['via']} · {c['pc']}")
    else:
        k.bad(f"A7 status: {st[:200]}")

    # A8 — no phone door possible: updown still runs, the page shows state "none" + why (the red state)
    open(f"{TMP}/none.conf", "w").write("BIND=192.0.2.1\nHOST_PORT=8078\nSTOP_MODE=idle\nRECEIVE_DIR=/downloads\n"
                                        "SHARE_DIR=/downloads\n")
    k.docker("run", "-d", "--name", f"{T}-none", "--network", "host", "--user", UID, "-v", f"{h.PROJECT}:/updown",
             "-v", f"{TMP}/cache:/gocache", "-v", f"{TMP}/dl:/downloads", "-v", f"{TMP}/none.conf:/tmp/none.conf:ro",
             "-w", "/updown", IMG, "sh", "-c", "go build -o /tmp/u ./cmd/updown && exec /tmp/u --config /tmp/none.conf")
    st = ""
    for _ in range(90):
        st = k.http("http://localhost:8078/host/api/status?after=0", timeout=2)[1]
        if st:
            break
        time.sleep(1)
    c = connection(st)
    if c.get("state") == "none" and c.get("reason") and k.http("http://localhost:8078/", timeout=3)[0] == 200:
        k.ok(f"A8 no phone door → updown still runs, the page answers, state none: {c['reason'][:70]}")
    else:
        k.bad(f"A8 status: {st[:200]}")
    k.docker("rm", "-f", f"{T}-none")

    # A5 — docker stop
    k.docker("stop", "-t", "15", T)
    code = k.docker("inspect", "-f", "{{.State.ExitCode}}", T)[1].strip()
    last = (k.logs(T).splitlines() or [""])[-1]
    k.ok("A5 docker stop → updown stopped, the agent exited 0") if code == "0" and "stopped" in last \
        else k.bad(f"A5 exit {code}")

    # A6 — no phone token in the output (the phone links carry ?t=)
    out = k.logs(T)
    if "?t=" in out or "&t=" in out:
        k.bad("A6 a phone token appeared in the output")
    else:
        k.ok("A6 no phone token in the output — only host-page links")
finally:
    k.docker("rm", "-f", T, f"{T}-none")
    shutil.rmtree(TMP, ignore_errors=True)

if k.FAILED:
    sys.exit(1)
k.ok("updown-agent tests passed (throwaway container removed)")
