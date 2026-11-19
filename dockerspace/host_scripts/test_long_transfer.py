#!/usr/bin/env python3
"""test_long_transfer.py — long transfers are never cut off; STOP_MODE=never keeps running ([2.8.1], UPDOWN-86).
The door: test_long_transfer.sh (its --help).

Two THROWAWAY containers on their own ports 8075 (STOP_MODE=idle) / 8074 (never), IDLE_MINUTES=1 — beside the real
updown, never touching it. A rate-limited client (300 KB/s, as curl --limit-rate 300k) moves 40 MB ≈ 2¼ minutes.
L1 a long download completes, no stop during it · L2 idle afterwards → it still stops · L3 never: no stop, same link
· L4 a long upload saved complete, no stop during it · L5 a 4 KB file both ways.
"""
import filecmp
import http.client
import os
import re
import shutil
import subprocess
import sys
import threading
import time
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import test_kit as k
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
IMG, RATE, CHUNK = h.IMG, 300 * 1024, 16384
# the real workspace (also from a test worktree): the git common dir's folder, 4 levels up
common = subprocess.run(["git", "-C", str(h.PROJECT), "rev-parse", "--path-format=absolute", "--git-common-dir"],
                        stdout=subprocess.PIPE, text=True).stdout.strip()
WS = os.path.realpath(os.path.join(common, "../../../.."))
if k.docker("image", "inspect", IMG)[0]:
    k.bad(f"image {IMG} missing")
    sys.exit(1)
listening = subprocess.run(["ss", "-ltn"], stdout=subprocess.PIPE, text=True).stdout
for p in (8075, 8074):
    if f":{p} " in listening:
        k.bad(f"port {p} is busy")
        sys.exit(1)
TMP = k.open_tmp("cache", "idle", "never", "dl")
k.warm_cache(WS, f"{TMP}/cache")
with open(f"{TMP}/dl/big.bin", "wb") as f:                        # 40 MB: far more than the network buffers hold
    f.write(os.urandom(40000000))
base_conf = open(h.PROJECT / "updown-agent/updown.conf").read()
for mode, port in (("idle", 8075), ("never", 8074)):
    open(f"{TMP}/{mode}/updown.conf", "w").write(k.conf(base_conf, STOP_MODE=mode, IDLE_MINUTES="1", BIND="auto",
                                                        HOST_PORT=str(port), CONTROLS_URL=""))
k.all_writable(TMP)


def run(name, conf_dir):
    k.docker("run", "-d", "--name", name, "--network", "host", "--user", f"{os.getuid()}:{os.getgid()}",
             "-v", f"{h.PROJECT}:/updown", "-v", f"{TMP}/cache:/gocache", "-v", f"{TMP}/dl:/downloads",
             "-v", f"{conf_dir}:/agent", "-e", "CONTAINER_WORKDIR=/updown", "-e", "LOG_MIRROR_ROOT=/tmp",
             "-e", "UPDOWN_AGENT_CONF=/agent/updown.conf", "-e", "UPDOWN_AGENT_STATE=/tmp/s", IMG,
             "python3", "/updown/updown-agent/agent.py")


def link(port):
    return str((k.js(k.http(f"http://localhost:{port}/host/api/status?after=0")[1]) or {}).get("receive") or "")


def wait_link(port):
    for _ in range(180):
        l = link(port)
        if l:
            return l
        time.sleep(1)
    return ""


def base_token(l):
    """The receive link → (http://host:port, token) — as ${LI%%/receive*} and ${LI##*t=}."""
    return l.split("/receive")[0], l.rsplit("t=", 1)[-1]


def paced(stream_read, write, rate):
    """Copy at most `rate` bytes a second (curl --limit-rate)."""
    start, sent = time.time(), 0
    while True:
        chunk = stream_read(CHUNK)
        if not chunk:
            return
        write(chunk)
        sent += len(chunk)
        ahead = sent / rate - (time.time() - start)
        if ahead > 0:
            time.sleep(ahead)


def download(url, path, rate=None):
    try:
        with urllib.request.urlopen(url, timeout=600) as r, open(path, "wb") as f:
            paced(r.read, f.write, rate or 10 ** 12)
        return 0
    except OSError:
        return 1


def upload(url, path, rate=None):
    """PUT the file (curl -T) → (exit code, the answer)."""
    u = urllib.parse.urlsplit(url)
    try:
        conn = http.client.HTTPConnection(u.hostname, u.port, timeout=600)
        conn.putrequest("PUT", u.path + ("?" + u.query if u.query else ""))
        conn.putheader("Content-Length", str(os.path.getsize(path)))
        conn.endheaders()
        with open(path, "rb") as f:
            paced(f.read, conn.send, rate or 10 ** 12)
        r = conn.getresponse()
        return (0 if r.status < 400 else 22), r.read().decode(errors="replace")
    except OSError as e:
        return 1, str(e)


try:
    run("updown-lt-idle", f"{TMP}/idle")
    LI = wait_link(8075)
    if not LI:
        k.bad(f"the idle updown has no link: {' '.join(k.logs('updown-lt-idle').splitlines()[-2:])}")
        sys.exit(1)

    # L1 — a download twice as long as the idle time (40 MB at 300 KB/s ≈ 2¼ min), started the moment updown is up
    s = int(time.time())
    base, token = base_token(LI)
    result = {}
    t = threading.Thread(target=lambda: result.update(rc=download(f"{base}/api/files/big.bin?t={token}",
                                                                  f"{TMP}/got.bin", RATE)))
    t.start()
    run("updown-lt-never", f"{TMP}/never")                       # L3's updown idles meanwhile
    t.join()
    until = int(time.time())
    took = until - s
    during = k.logs("updown-lt-idle", since=s, until=until).count("stopped: idle")
    LN = wait_link(8074)
    if not LN:
        k.bad("the never updown has no link")
        sys.exit(1)
    same = os.path.exists(f"{TMP}/got.bin") and filecmp.cmp(f"{TMP}/got.bin", f"{TMP}/dl/big.bin", shallow=False)
    if result.get("rc") == 0 and same and took >= 90 and during == 0 \
            and "download finished: big.bin" in k.logs("updown-lt-idle"):
        k.ok(f"L1 a {took}s download (idle limit 60s) completed byte for byte — updown did not stop during it")
    else:
        lines = [l for l in k.logs("updown-lt-idle").splitlines() if re.search("stopped|download", l)][-3:]
        k.bad(f"L1 stops during the download: {during} · rc={result.get('rc')} took={took}s "
              f"same={'yes' if same else 'no'} — {' '.join(lines)}")

    # L2 — afterwards, really idle → it still stops by itself (idle mode not broken)
    for _ in range(100):
        if "stopped: idle" in k.logs("updown-lt-idle"):
            break
        time.sleep(2)
    if "stopped: idle" in k.logs("updown-lt-idle"):
        k.ok("L2 once really idle, STOP_MODE=idle still stops by itself")
    else:
        k.bad("L2 no idle stop after the transfer")

    # L3 — STOP_MODE=never: idle the whole time (now > 3 min) → no stop, the same link
    L3 = link(8074)
    never_logs = k.logs("updown-lt-never")
    if L3 == LN and not re.search("stopped|starting it again", never_logs):
        k.ok(f"L3 STOP_MODE=never: idle for {int(time.time()) - s}s+ — no stop, the same link (same QR codes)")
    else:
        lines = [l for l in never_logs.splitlines() if re.search("stopped|again", l)][-2:]
        k.bad(f"L3 link same={'yes' if L3 == LN else 'no'} — {' '.join(lines)}")

    # L4 — a long UPLOAD on the idle updown (it restarted in L2: use its new link)
    LI = wait_link(8075)
    base, token = base_token(LI)
    s = time.time()
    rc, answer = upload(f"{base}/api/upload/up.bin?t={token}", f"{TMP}/dl/big.bin", RATE)
    took = int(time.time() - s)
    # stops BETWEEN "upload started" and "upload finished" in the log (order, not time: no overlap with L2's stop)
    during, on = 0, False
    for line in k.logs("updown-lt-idle").splitlines():
        if "upload started: up.bin" in line:
            on = True
        if on and "stopped: idle" in line:
            during += 1
        if "upload finished: up.bin" in line:
            on = False
    same = os.path.exists(f"{TMP}/dl/up.bin") and filecmp.cmp(f"{TMP}/dl/up.bin", f"{TMP}/dl/big.bin", shallow=False)
    if rc == 0 and same and took >= 90 and during == 0:
        k.ok(f"L4 a {took}s upload (idle limit 60s) was saved complete — updown did not stop during it")
    else:
        k.bad(f"L4 stops during the upload: {during} · rc={rc} took={took}s same={'yes' if same else 'no'} {answer[:120]}")

    # L5 — small files (a few KB) both ways, on the never updown
    with open(f"{TMP}/small.txt", "wb") as f:
        f.write(os.urandom(4096))
    base, token = base_token(LN)
    if upload(f"{base}/api/upload/small.txt?t={token}", f"{TMP}/small.txt")[0] == 0 \
            and download(f"{base}/api/files/small.txt?t={token}", f"{TMP}/small_back.txt") == 0 \
            and filecmp.cmp(f"{TMP}/small.txt", f"{TMP}/small_back.txt", shallow=False):
        k.ok("L5 a 4 KB file up and back down: unchanged")
    else:
        k.bad("L5 small file round trip failed")
finally:
    k.docker("rm", "-f", "updown-lt-idle", "updown-lt-never")
    shutil.rmtree(TMP, ignore_errors=True)

if k.FAILED:
    sys.exit(1)
k.ok("long-transfer tests passed (throwaway containers removed)")
