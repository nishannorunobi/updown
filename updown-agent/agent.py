#!/usr/bin/env python3
# ─────────────────────────────────────────────────────────────────────────────
# agent.py — updown-agent: the updown container's ENTRY POINT (story [2.1], UPDOWN-26; rule CLAUDEWORK-126)
#
# What for    The container runs updown by itself and hands out its link — the host only starts the
#             container (bash start.sh). Nobody needs to know how updown is built or run.
# Steps       1. build updown from the source in /updown (offline: the Go cache is mounted, nothing downloaded)
#             2. start it as the agent's CHILD with updown-agent/updown.conf (STOP_MODE=idle by default)
#             3. read its "[updown] open this page …" line → print the link + keep it (for start.sh / status.sh)
#             4. watch it: updown stopped (idle) or crashed → start it again → a NEW link;
#                docker stop → passed on to updown → clean exit
#             5. follow the PC's network ([2.4], UPDOWN-35): every 5 s it asks `updown --address` which address
#                the settings allow (BIND=hotspot: ONLY the phone's hotspot). Changed → updown restarted on the new
#                address (new link). None → updown stopped, "waiting for the phone's hotspot", started when it is back
# How         command in docker-compose.yml:  python3 /updown/updown-agent/agent.py
#             python3 /updown/updown-agent/agent.py link [SECONDS]   → prints the current link (waits up to
#                                                                     SECONDS, default 60), exit 1 if none
# Output      [updown-agent] lines + updown's own [updown] lines, in docker logs and the mirror log
# Note        Python standard library only (the dev image has python3, nothing else). The phone token is
#             never printed: the link is the HOST page (this PC's browser), protected by its own key.
# ─────────────────────────────────────────────────────────────────────────────
import os, re, signal, subprocess, sys, threading, time

PROJECT = os.environ.get("CONTAINER_WORKDIR", "/updown")
HERE = os.path.dirname(os.path.abspath(__file__))
CONF = os.environ.get("UPDOWN_AGENT_CONF", os.path.join(HERE, "updown.conf"))
STATE = os.environ.get("UPDOWN_AGENT_STATE", "/tmp/updown-agent")      # inside the container only
LINK_FILE = os.path.join(STATE, "link")
BINARY = os.path.join(STATE, "updown")
LOG_DIR = os.path.join(os.environ.get("LOG_MIRROR_ROOT", "/tmp"), "updown-agent")
LINK_RE = re.compile(r"open this page on this PC to show the QR codes: (\S+)")
LISTEN_RE = re.compile(r"listening on (\d+\.\d+\.\d+\.\d+):\d+")
WATCH_SECONDS = 5
ADDRESS_CMD = os.environ.get("UPDOWN_AGENT_ADDRESS_CMD")              # tests only: a shell command instead of updown --address

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


def current_address():
    """The address the settings allow right now — asked from updown itself (one source of truth)."""
    cmd = ADDRESS_CMD if ADDRESS_CMD else [BINARY, "--config", CONF, "--address"]
    r = subprocess.run(cmd, shell=bool(ADDRESS_CMD), cwd=PROJECT, capture_output=True, text=True)
    if r.returncode == 0 and r.stdout.strip():
        return r.stdout.strip().splitlines()[-1], ""
    return None, (r.stderr.strip() or r.stdout.strip() or "no address").replace("[updown] ", "")


class Agent:
    def __init__(self):
        self.child = None
        self.stopping = False
        self.listening = None          # the address updown listens on (from its own output)
        self.restart_reason = ""

    def stop(self, *_):
        """docker stop (SIGTERM) / Ctrl+C: pass it on to updown, then leave."""
        self.stopping = True
        if self.child and self.child.poll() is None:
            self.child.send_signal(signal.SIGTERM)

    def watch(self, child):
        """While updown runs: the PC's network changed (or the hotspot is gone) → stop it; main() restarts it."""
        while child.poll() is None and not self.stopping:
            time.sleep(WATCH_SECONDS)
            if child.poll() is not None or self.listening is None:
                continue
            addr, why = current_address()
            if addr != self.listening:
                self.restart_reason = (f"the network changed: {self.listening} → {addr}" if addr
                                       else f"{self.listening} is gone — {why}")
                child.send_signal(signal.SIGTERM)
                return

    def run_once(self):
        """Start updown, relay its output, keep the link; returns updown's exit code."""
        set_link("")
        self.listening, self.restart_reason = None, ""
        self.child = subprocess.Popen([BINARY, "--config", CONF], cwd=PROJECT, stdin=subprocess.DEVNULL,
                                      stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)
        threading.Thread(target=self.watch, args=(self.child,), daemon=True).start()
        for line in self.child.stdout:
            line = line.rstrip("\n")
            print(line, flush=True)                                  # updown's own [updown] lines
            m = LISTEN_RE.search(line)
            if m:
                self.listening = m.group(1)
            m = LINK_RE.search(line)
            if m:
                set_link(m.group(1))
                say("link — open it in this PC's browser for the QR codes: " + m.group(1))
        return self.child.wait()

    def main(self):
        signal.signal(signal.SIGTERM, self.stop)
        signal.signal(signal.SIGINT, self.stop)
        say(f"starting — settings: {CONF}")
        delay = 1
        built = False
        waiting = ""
        while not self.stopping:
            if not built:
                if not build():
                    time.sleep(min(delay, 60)); delay *= 2; continue
                built = True
            addr, why = current_address()
            if not addr:                                        # BIND=hotspot and no hotspot: wait, say it once
                if why != waiting:
                    say(f"waiting — {why}")
                    waiting = why
                time.sleep(WATCH_SECONDS)
                continue
            waiting = ""
            started = time.time()
            rc = self.run_once()
            set_link("")
            if self.stopping:
                break
            if self.restart_reason:
                say(self.restart_reason + " — starting updown again")
                built = False                                    # a rebuild is quick and picks up source changes
                continue
            built = False
            if rc == 0:
                say("updown stopped by itself (idle / after a transfer) — starting it again with a new link")
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
