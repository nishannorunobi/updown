#!/usr/bin/env python3
"""test_controls.py — the page's buttons and settings ([2.7]) on a THROWAWAY container. The door: test_controls.sh.

The same image and network as docker-compose.yml, its own ports (page 8077, buttons 8068), temp Downloads, a temp
copy of updown.conf + actions.conf + actions/. Runs next to the real updown-dev-container (never touches it).
C1 page blocks · C2 buttons · C3 a broken action greyed · C4 Check now · C5 Refresh QR · C6 no other web site ·
C7 the switch · C8 a real browser (headless Chrome, when installed).
"""
import os
import re
import shutil
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import test_kit as k
import updown_host as h

if h.starter:
    h.starter.start()          # here, in __main__: start() acts only for the program itself (--help + the mirror log)
T, IMG, PAGE, BTN = "updown-controls-test", h.IMG, "http://localhost:8077", "http://localhost:8068"
if k.docker("image", "inspect", IMG)[0]:
    k.bad(f"image {IMG} missing — bash start.sh --build")
    sys.exit(1)
listening = subprocess.run(["ss", "-ltn"], stdout=subprocess.PIPE, text=True).stdout
for p in (8077, 8068):
    if re.search(rf"127\.0\.0\.1:{p} |:{p} ", listening):
        k.bad(f"port {p} is busy — this test needs it")
        sys.exit(1)
TMP = k.open_tmp("dl", "cache", "agent")
k.warm_cache(h.WS_ROOT, f"{TMP}/cache")
open(f"{TMP}/agent/updown.conf", "w").write(k.conf(open(h.PROJECT / "updown-agent/updown.conf").read(), BIND="auto",
                                                   HOST_PORT="8077", CONTROLS_URL="http://localhost:8068",
                                                   PAGE_PARTS="qr_codes,connection,nope"))
shutil.copytree(h.PROJECT / "updown-agent/actions", f"{TMP}/agent/actions")
shutil.copy(h.PROJECT / "updown-agent/actions.conf", f"{TMP}/agent/actions.conf")
with open(f"{TMP}/agent/actions.conf", "a") as f:
    f.write("broken | ✖ | Broken | broken.py | yes\nhidden | ✖ | Hidden | check_now.py | no\n")
open(f"{TMP}/agent/actions/broken.py", "w").write('raise ImportError("this file is broken on purpose")\n')
k.all_writable(TMP)


def status():
    return k.js(k.http(f"{PAGE}/host/api/status?after=0")[1]) or {}


def actions():
    body = k.http(f"{BTN}/actions", timeout=10)[1]
    return body, (k.js(body) or {}).get("actions", [])


try:
    k.docker("rm", "-f", T)
    if k.docker("run", "-d", "--name", T, "--network", "host", "--user", f"{os.getuid()}:{os.getgid()}",
                "-v", f"{h.PROJECT}:/updown", "-v", f"{TMP}/cache:/gocache", "-v", f"{TMP}/dl:/downloads",
                "-v", f"{TMP}/agent:/agent", "-e", "CONTAINER_WORKDIR=/updown", "-e", "LOG_MIRROR_ROOT=/tmp",
                "-e", "UPDOWN_AGENT_CONF=/agent/updown.conf", "-e", "UPDOWN_AGENT_ACTIONS=/agent/actions.conf",
                "-e", "UPDOWN_AGENT_STATE=/tmp/updown-controls-test", IMG, "python3", "/updown/updown-agent/agent.py")[0]:
        k.bad("throwaway container did not start")
        sys.exit(1)
    if k.docker("exec", T, "python3", "/updown/updown-agent/agent.py", "link", "180")[0]:
        k.bad(f"updown did not start: {' '.join(k.logs(T).splitlines()[-3:])}")
        sys.exit(1)

    # C1 — the page's blocks
    lay = k.http(f"{PAGE}/host/api/page")[1]
    d = k.js(lay) or {}
    got = ",".join(d.get("parts", [])) + " " + str(d.get("controls_url"))
    if got == "qr_codes.html,connection.html http://localhost:8068" \
            and re.search(r'WARN.*PAGE_PARTS: no block "nope"', k.logs(T)):
        k.ok('C1 the page\'s blocks from PAGE_PARTS: qr_codes, connection ("nope" left out with a WARN)')
    else:
        k.bad(f"C1 layout: {lay}")

    # C2 + C3 — the buttons from actions.conf
    body, acts = actions()
    got = " ".join(a["name"] + ("[" + "/".join(o["value"] for o in a["options"]) + "]" if a["options"] else "")
                   for a in acts)
    if got == "network_mode[hotspot/auto] check_now new_link broken":
        k.ok(f"C2 the buttons from actions.conf: {got} (SHOWN=no hidden)")
    else:
        k.bad(f"C2 got [{got}] — {body}")
    got = next(((a["ok"], a["reason"]) for a in acts if a["name"] == "broken"), None)
    if got == (False, "this file is broken on purpose"):
        k.ok(f"C3 a broken action file greys out only its button: {got}")
    else:
        k.bad(f"C3 got [{got}]")

    # C4 — Check now
    r = k.js(k.http(f"{BTN}/action/check_now", "POST", timeout=20)[1]) or {}
    k.ok(f"C4 ⟳ Check now: {r.get('message')}") if r.get("ok") is True else k.bad(f"C4 {r}")

    # C5 — Refresh QR code
    before = status().get("send", "")
    r = k.js(k.http(f"{BTN}/action/new_link", "POST", timeout=20)[1]) or {}
    time.sleep(1)
    after = status().get("send", "")
    if not before:
        if r.get("ok") is False:
            k.ok(f"C5 🔄 Refresh QR code without a phone network → refused: {r.get('message')}")
        else:
            k.bad(f"C5 {r}")
    elif r.get("ok") is True and after and after != before:
        k.ok("C5 🔄 Refresh QR code → a new token, the page's link changed")
    else:
        k.bad(f"C5 {r} (link changed: {'yes' if after != before else 'no'})")

    # C6 — no other web site
    evil = {"Origin": "https://evil.example"}
    a = k.http(f"{BTN}/action/check_now", "POST", evil)[0]
    u = k.http(f"{PAGE}/host/api/check", "POST", evil)[0]
    hh = k.http(f"{BTN}/actions", headers={"Host": "evil.example:8068"})[0]
    p = k.http(f"{BTN}/action/check_now", "POST", {"Origin": "http://localhost:8077"}, timeout=20)[0]
    if (a, u, hh, p) == (403, 403, 403, 200):
        k.ok("C6 another web site: agent 403 · updown 403 · other Host 403 — the page itself 200")
    else:
        k.bad(f"C6 agent={a} updown={u} host={hh} page={p}")

    # C7 — the switch
    r = k.js(k.http(f"{BTN}/action/network_mode?value=hotspot", "POST", timeout=20)[1]) or {}
    s = time.time()
    while "restarting updown with BIND=hotspot" not in k.logs(T) and time.time() - s <= 30:
        time.sleep(1)
    time.sleep(3)
    acts = actions()[1]
    on = [o["value"] for o in acts[0]["options"] if o["on"]] if acts else None
    conf_now = open(f"{TMP}/agent/updown.conf").read().splitlines()
    if "BIND=hotspot" in conf_now and on == ["hotspot"] and r.get("ok") is True \
            and k.docker("exec", T, "python3", "/updown/updown-agent/agent.py", "link", "60")[0] == 0 and status():
        k.ok("C7 the switch → BIND=hotspot in updown.conf, updown restarted, the page answers, the switch shows it")
    else:
        k.bad(f"C7 {r} on={on}")

    # C8 — a real browser (urllib ignores the page's CSP; a browser does not)
    chrome = next((shutil.which(c) for c in ("google-chrome", "chromium", "chromium-browser") if shutil.which(c)), None)
    if not chrome:
        print("[ SKIP ] C8 no Chrome / Chromium on this PC", flush=True)
    else:
        try:
            dom = subprocess.run([chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--virtual-time-budget=8000",
                                  "--dump-dom", f"{PAGE}/"], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                 text=True, timeout=60).stdout
        except subprocess.TimeoutExpired:
            dom = ""
        n = "".join(f"{x} " for x in sorted(set(re.findall(r'data-a="[a-z_]*"', dom))))
        expect = 'data-a="broken" data-a="check_now" data-a="network_mode" data-a="new_link" '
        k.ok(f"C8 a real browser draws the buttons: {n}") if n == expect else k.bad(f"C8 the browser shows [{n}]")

    if k.FAILED:
        print("\n".join(k.logs(T).splitlines()[-15:]), flush=True)
        if "📶" not in body:
            print(f"(icons not UTF-8 in: {body})", flush=True)
finally:
    k.docker("rm", "-f", T)
    shutil.rmtree(TMP, ignore_errors=True)

if k.FAILED:
    sys.exit(1)
k.ok("page button tests passed (throwaway container removed)")
