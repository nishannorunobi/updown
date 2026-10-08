#!/usr/bin/env python3
# ─────────────────────────────────────────────────────────────────────────────
# agent.py — updown-agent: the updown container's ENTRY POINT (story [2.1], UPDOWN-26; rule CLAUDEWORK-126)
#
# What for    The container runs updown by itself and hands out its link — the host only starts the
#             container (bash start.sh). Nobody needs to know how updown is built or run.
# Steps       1. build updown from the source in /updown (offline: the Go cache is mounted, nothing downloaded)
#             2. start it as the agent's CHILD with updown-agent/updown.conf (STOP_MODE=idle by default)
#             3. read its "[updown] open this page …" line → print the link + keep it (for start.sh / status.sh)
#             4. watch it: updown stopped (idle) or crashed → start it again (the page link stays the same);
#                docker stop → passed on to updown → clean exit
#             The phone's network is followed by updown ITSELF ([2.6], UPDOWN-52): its page stays at
#             http://localhost:8070 and it opens / moves / closes the phone's door as the network changes.
#             5. the page's BUTTONS ([2.7]): a small API on CONTROLS_URL (updown.conf, http://localhost:8069) —
#                actions.conf lists the buttons, each runs its own file in actions/ (check_now.py, …).
#                A new button = a line in actions.conf + a file in actions/. This PC only, this page only.
# How         command in docker-compose.yml:  python3 /updown/updown-agent/agent.py
#             python3 /updown/updown-agent/agent.py link [SECONDS]   → prints the current link (waits up to
#                                                                     SECONDS, default 60), exit 1 if none
# Output      [updown-agent] lines + updown's own [updown] lines, in docker logs and the mirror log
# Note        Python standard library only (the dev image has python3, nothing else). The phone token is
#             never printed: the link is the HOST page (this PC's browser), protected by its own key.
# ─────────────────────────────────────────────────────────────────────────────
import importlib.util, json, os, re, signal, subprocess, sys, threading, time, urllib.error, urllib.parse, urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PROJECT = os.environ.get("CONTAINER_WORKDIR", "/updown")
HERE = os.path.dirname(os.path.abspath(__file__))
CONF = os.environ.get("UPDOWN_AGENT_CONF", os.path.join(HERE, "updown.conf"))
STATE = os.environ.get("UPDOWN_AGENT_STATE", "/tmp/updown-agent")      # inside the container only
LINK_FILE = os.path.join(STATE, "link")
BINARY = os.path.join(STATE, "updown")
LOG_DIR = os.path.join(os.environ.get("LOG_MIRROR_ROOT", "/tmp"), "updown-agent")
LINK_RE = re.compile(r"open this page on this PC to show the QR codes: (\S+)")
ACTIONS_CONF = os.environ.get("UPDOWN_AGENT_ACTIONS", os.path.join(HERE, "actions.conf"))
ACTIONS_DIR = os.path.join(os.path.dirname(ACTIONS_CONF), "actions")
# actions.conf missing → these buttons (the shipped file's lines)
DEFAULT_ACTIONS = """network_mode | 📶 | Network | network_mode.py | yes
check_now | ⟳ | Check now | check_now.py | yes
new_link | 🔑 | New link | new_link.py | yes"""
NAME_RE, FILE_RE = re.compile(r"^[a-z0-9_]+$"), re.compile(r"^[a-z0-9_]+\.py$")


_log_lock = threading.Lock()


def say(msg):
    """One line to the console (docker logs) and to the mirror log."""
    line = f"[updown-agent] {msg}"
    with _log_lock:
        print(line, flush=True)
        try:
            os.makedirs(LOG_DIR, exist_ok=True)
            with open(os.path.join(LOG_DIR, "agent_py.log"), "a") as f:
                f.write(time.strftime("[%Y-%m-%d %H:%M:%S] ") + line + "\n")
        except OSError:
            pass


def set_link(url):
    os.makedirs(STATE, exist_ok=True)
    tmp = LINK_FILE + ".part"
    with open(tmp, "w") as f:
        f.write((url or "") + "\n")
    os.replace(tmp, LINK_FILE)


def build():
    """Build updown from the source — offline (GOPROXY=off, GOTOOLCHAIN=local in the image)."""
    os.makedirs(STATE, exist_ok=True)
    r = subprocess.run(["go", "build", "-o", BINARY, "./cmd/updown"], cwd=PROJECT,
                       capture_output=True, text=True)
    if r.returncode != 0:
        say("build failed: " + (r.stderr.strip().splitlines() or ["?"])[-1])
        return False
    return True


_warned = set()


def warn_once(msg):
    """A WARN about a file is said once, not at every page load."""
    if msg not in _warned:
        _warned.add(msg)
        say("WARN " + msg)


def settings(path=None):
    """updown.conf as a dict (KEY=VALUE lines, # comments) — read fresh each time."""
    out = {}
    try:
        for line in open(path or CONF, encoding="utf-8"):
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, v = line.split("=", 1)
                out[k.strip()] = v.strip().strip("'\"")
    except OSError:
        pass
    return out


def set_setting(key, value, path=None):
    """Change one KEY=VALUE line of updown.conf in place (comments stay); a missing key is added at the end."""
    path = path or CONF
    lines = open(path, encoding="utf-8").read().splitlines(keepends=True)
    for i, line in enumerate(lines):
        if re.match(rf"\s*{re.escape(key)}\s*=", line):
            lines[i] = f"{key}={value}\n"
            break
    else:
        lines.append(f"{key}={value}\n")
    with open(path, "w", encoding="utf-8") as f:        # in place: the file may be a single bind-mounted file
        f.writelines(lines)


class Context:
    """What an action file gets: updown's settings, updown's control points, a restart."""

    def __init__(self, agent):
        self._agent = agent

    def setting(self, key, default=""):
        return settings().get(key) or default

    def set_setting(self, key, value):
        set_setting(key, value)
        say(f"page button: {key}={value} saved in {CONF}")

    def restart_updown(self, why):
        self._agent.restart(why)

    def updown(self, path):
        """POST to updown's own page (this PC only) → its JSON answer; an error → an exception with its words."""
        url = f"http://127.0.0.1:{self.setting('HOST_PORT', '8070')}{path}"
        try:
            with urllib.request.urlopen(urllib.request.Request(url, data=b"", method="POST"), timeout=15) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            try:
                msg = json.load(e).get("error")
            except ValueError:
                msg = None
            raise RuntimeError(msg or f"updown answered {e.code}") from None
        except OSError:
            raise RuntimeError("updown is not reachable (restarting?) — try again in a few seconds") from None


def actions():
    """actions.conf → [{name, icon, label, file, ok, reason}] — only SHOWN ones, in order. Read at every call."""
    try:
        text = open(ACTIONS_CONF, encoding="utf-8").read()
    except OSError:
        warn_once(f"{ACTIONS_CONF} not found — using the default buttons")
        text = DEFAULT_ACTIONS
    out = []
    for n, line in enumerate(text.splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        f = [x.strip() for x in line.split("|")]
        if len(f) < 4 or not NAME_RE.match(f[0]) or not FILE_RE.match(f[3]):
            warn_once(f"{ACTIONS_CONF} line {n}: expected NAME | ICON | LABEL | FILE | SHOWN — left out: {line}")
            continue
        shown = (f[4] if len(f) > 4 and f[4] else "yes").lower()
        if shown not in ("yes", "no"):
            warn_once(f"{ACTIONS_CONF} line {n}: SHOWN={shown} is not yes/no — using yes")
            shown = "yes"
        if shown == "yes":
            out.append({"name": f[0], "icon": f[1], "label": f[2], "file": f[3]})
    return out


def load_action(a):
    """The action's module, loaded fresh from actions/FILE (an edit counts at once)."""
    path = os.path.join(ACTIONS_DIR, a["file"])
    if not os.path.isfile(path):
        raise RuntimeError(f"actions/{a['file']} not found")
    spec = importlib.util.spec_from_file_location("updown_action_" + a["name"], path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    if not callable(getattr(mod, "run", None)):
        raise RuntimeError(f"actions/{a['file']} has no run(ctx, value)")
    return mod


class Controls(BaseHTTPRequestHandler):
    """The page's buttons: GET /actions · POST /action/NAME[?value=V]. Only this PC (127.0.0.1, Host check), and a
    press only from updown's own page (Origin) or a program on this PC (no Origin) — never another web site."""
    agent = None

    def log_message(self, *_):
        pass

    def allowed_origins(self):
        port = settings().get("HOST_PORT") or "8070"
        return {f"http://localhost:{port}", f"http://127.0.0.1:{port}"}

    def answer(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Cache-Control", "no-store")
        origin = self.headers.get("Origin")
        if origin in self.allowed_origins():
            self.send_header("Access-Control-Allow-Origin", origin)
            self.send_header("Vary", "Origin")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def guard(self, press):
        host = (self.headers.get("Host") or "").rsplit(":", 1)[0]
        if host not in ("localhost", "127.0.0.1"):
            self.answer(403, {"error": "this PC only"})
            return False
        origin = self.headers.get("Origin")
        if press and origin is not None and origin not in self.allowed_origins():
            self.answer(403, {"error": "only updown's page may press its buttons"})
            return False
        return True

    def do_GET(self):
        if not self.guard(False):
            return
        if urllib.parse.urlsplit(self.path).path != "/actions":
            return self.answer(404, {"error": "not found"})
        ctx, out = Context(self.agent), []
        for a in actions():
            item = {"name": a["name"], "icon": a["icon"], "label": a["label"], "ok": True, "reason": "", "options": None}
            try:
                mod = load_action(a)
                if callable(getattr(mod, "options", None)):
                    item["options"] = mod.options(ctx)
            except Exception as e:                             # a broken file greys out its button, nothing else
                item["ok"], item["reason"] = False, str(e) or type(e).__name__
            out.append(item)
        self.answer(200, {"actions": out})

    def do_POST(self):
        if not self.guard(True):
            return
        u = urllib.parse.urlsplit(self.path)
        name = u.path[len("/action/"):] if u.path.startswith("/action/") else ""
        a = next((a for a in actions() if a["name"] == name), None)
        if not a:
            return self.answer(404, {"ok": False, "message": f"no button {name!r} in actions.conf"})
        value = urllib.parse.parse_qs(u.query).get("value", [None])[0]
        try:
            msg = load_action(a).run(Context(self.agent), value)
            say(f"page button {name}{'=' + value if value else ''}: {msg}")
            self.answer(200, {"ok": True, "message": msg})
        except Exception as e:
            say(f"page button {name}: failed — {e}")
            self.answer(200, {"ok": False, "message": str(e) or type(e).__name__})


def serve_controls(agent):
    """Start the buttons' API on CONTROLS_URL's port (updown.conf) — not set → no buttons, said once."""
    url = settings().get("CONTROLS_URL", "")
    u = urllib.parse.urlsplit(url)
    if not url or u.hostname not in ("localhost", "127.0.0.1") or not u.port:
        say(f"WARN no buttons on the page: CONTROLS_URL in {CONF} must be http://localhost:PORT (is {url!r})")
        return None
    Controls.agent = agent
    try:
        srv = ThreadingHTTPServer(("127.0.0.1", u.port), Controls)
    except OSError as e:
        say(f"WARN no buttons on the page: cannot listen on 127.0.0.1:{u.port} ({e.strerror})")
        return None
    srv.daemon_threads = True
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    say(f"page buttons ({ACTIONS_CONF}) on {url}")
    return srv


class Agent:
    def __init__(self):
        self.child = None
        self.stopping = False
        self.restarting = ""

    def restart(self, why):
        """A page button changed updown's settings: stop updown, the loop starts it again with them."""
        self.restarting = why
        if self.child and self.child.poll() is None:
            self.child.send_signal(signal.SIGTERM)


    def stop(self, *_):
        """docker stop (SIGTERM) / Ctrl+C: pass it on to updown, then leave."""
        self.stopping = True
        if self.child and self.child.poll() is None:
            self.child.send_signal(signal.SIGTERM)

    def run_once(self):
        """Start updown, relay its output, keep the link; returns updown's exit code."""
        set_link("")
        self.child = subprocess.Popen([BINARY, "--config", CONF], cwd=PROJECT, stdin=subprocess.DEVNULL,
                                      stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)
        for line in self.child.stdout:
            line = line.rstrip("\n")
            print(line, flush=True)                                  # updown's own [updown] lines
            m = LINK_RE.search(line)
            if m:
                set_link(m.group(1))
                say("link — open it in this PC's browser for the QR codes: " + m.group(1))
        return self.child.wait()

    def main(self):
        signal.signal(signal.SIGTERM, self.stop)
        signal.signal(signal.SIGINT, self.stop)
        say(f"starting — settings: {CONF}")
        serve_controls(self)
        delay = 1
        while not self.stopping:
            if not build():
                time.sleep(min(delay, 60)); delay *= 2; continue
            started = time.time()
            rc = self.run_once()
            set_link("")
            if self.stopping:
                break
            if self.restarting:
                say(f"restarting updown with {self.restarting} (new QR codes)")
                self.restarting, delay = "", 1
            elif rc == 0:
                say("updown stopped by itself (idle / after a transfer) — starting it again (same page, new QR codes)")
                delay = 1
            else:
                say(f"updown ended with exit {rc} — starting it again in {delay}s")
                time.sleep(delay)
                delay = 1 if time.time() - started > 60 else min(delay * 2, 60)  # a long good run resets the backoff
        say("stopped")
        return 0


def show_link(wait):
    """`agent.py link [SECONDS]` — for start.sh / status.sh: the current link, waiting for it if needed."""
    end = time.time() + wait
    while True:
        try:
            url = open(LINK_FILE).read().strip()
        except OSError:
            url = ""
        if url:
            print(url)
            return 0
        if time.time() >= end:
            return 1
        time.sleep(1)


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] in ("-h", "--help"):
        n = 0
        for l in open(__file__).read().splitlines()[1:]:           # the header between the two rule lines
            if l.startswith("# ─"):
                n += 1
                if n == 2:
                    break
                continue
            if n == 1:
                print(l[2:] if l.startswith("# ") else l.lstrip("#"))
        sys.exit(0)
    if len(sys.argv) > 1 and sys.argv[1] == "link":
        sys.exit(show_link(int(sys.argv[2]) if len(sys.argv) > 2 else 60))
    sys.exit(Agent().main())
