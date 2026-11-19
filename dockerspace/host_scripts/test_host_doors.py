#!/usr/bin/env python3
"""test_host_doors.py — updown's own doors (root start · stop · status + host_scripts) against a FAKE docker, the old
bash and the new Python side by side. The door: test_host_doors.sh (its --help).

    H0  every door: invoking lines only · --help shows the guide WITHOUT running it · every .py compiles
    H1  status: --state running / stopped · the link · no link yet
    H2  stop: already stopped · stopped · docker fails → exit 1 · clean: container / --image, there or not
    H3  start: the link → OK · --build · no link but running → WARN · not up → ERROR · compose fails · the log
        volume refused · the pod guard (one level too high) · Downloads from XDG / ~/Downloads (the compose env)
    H4  exec: not running → exit 1 · no command → exit 2 · the command in /updown · shell · build (+ --test)
    H5  pull_images: here / pulled / missing → exit 1 · test_release: arguments · no release · offline run per image
    H6  container login: unknown name · Docker down · not running · no terminal
Each case runs on the OLD bash (git OLD_REF) and the NEW doors in two throwaway workspaces → the same lines, exit
codes and docker calls. Nothing real: a fake docker, a fake HOME.
"""
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

sys.path[:0] = [str(p / "ccc/scripts/lib/python_starter") for p in pathlib.Path(__file__).resolve().parents
                if (p / "mountspace").is_dir()][:1]
try:
    import starter
    starter.start()
except ImportError:
    starter = None

HERE = pathlib.Path(__file__).resolve().parent
PROJECT = HERE.parent.parent
OLD_REF = os.environ.get("OLD_REF", "d8bc609")          # main (v2026.11.03) before the conversion (CLAUDEWORK-514)
H = "dockerspace/host_scripts"
DOORS = ["start.sh", "stop.sh", "status.sh", *(f"{H}/{d}.sh" for d in (
    "start", "stop", "clean", "exec", "shell", "build", "pull_images", "test_release", "updown_container_login",
    "test_agent", "test_controls", "test_long_transfer")), "dockerspace/container_scripts/build.sh",
    "dockerspace/container_scripts/env_check.sh"]
C = "updown-dev-container"
FAILED = []

FAKE_DOCKER = r'''#!/usr/bin/env python3
import os, sys
a = sys.argv[1:]
env = os.environ.get
open(env("FAKE_LOG"), "a").write(" ".join(a) + "\n")
st = env("FAKE_STATE")
names = open(st).read().split() if os.path.exists(st) else []
C = "updown-dev-container"
if a[:1] == ["info"]:
    sys.exit(1 if env("FAKE_DOCKER_DOWN") else 0)
if a[:1] == ["ps"]:
    if "-q" in a:
        w = a[a.index("-f") + 1].split("=", 1)[1].strip("^$")
        if w in names: print("c0ffee")
    else:
        print("\n".join(names))
    sys.exit(0)
if a[:1] == ["compose"]:
    if not os.path.isfile("docker-compose.yml"):
        print("no configuration file provided: not found", file=sys.stderr); sys.exit(1)
    print("fake compose " + " ".join(a[1:]) + " · HOST_DOWNLOADS=" + env("HOST_DOWNLOADS", "-"))
    if a[1:2] == ["run"]:
        sys.exit(int(env("FAKE_CHOWN_RC", "0")))
    if a[1:2] == ["up"]:
        rc = int(env("FAKE_UP_RC", "0"))
        if not rc and not env("FAKE_UP_DIES"): open(st, "w").write(C + "\n")
        sys.exit(rc)
    sys.exit(0)
if a[:1] == ["exec"]:
    if "link" in a:
        if env("FAKE_LINK"): print(env("FAKE_LINK")); sys.exit(0)
        print("agent: no link", file=sys.stderr); sys.exit(1)
    if a[-3:] == ["sh", "-c", "command -v bash"]:
        sys.exit(0)
    print("fake exec " + " ".join(a[1:])); sys.exit(int(env("FAKE_EXEC_RC", "0")))
if a[:1] == ["stop"]:
    rc = int(env("FAKE_STOP_RC", "0"))
    if rc: print("Error: cannot stop", file=sys.stderr); sys.exit(rc)
    print(a[-1]); open(st, "w").write(""); sys.exit(0)
if a[:2] == ["rm", "-f"]:
    sys.exit(0 if env("FAKE_EXISTS") else 1)
if a[:1] == ["rmi"]:
    sys.exit(0 if env("FAKE_EXISTS") else 1)
if a[:2] == ["image", "inspect"]:
    sys.exit(0 if a[2] in env("FAKE_IMAGES", "").split() else 1)
if a[:1] == ["pull"]:
    sys.exit(0 if a[-1] in env("FAKE_PULLABLE", "").split() else 1)
if a[:1] == ["run"]:
    print("fake run in " + a[-1 - 3]); sys.exit(int(env("FAKE_RUN_RC", "0")))
sys.exit(0)
'''


def check(name, good, detail=""):
    print(f"\033[32m[ PASS ]\033[0m {name}" if good else f"\033[31m[ FAIL ]\033[0m {name}\n{detail}", flush=True)
    if not good:
        FAILED.append(name)


def make_ws(ws, source):
    pod = ws / "projectspace/apps/updown"
    (ws / "mountspace").mkdir(parents=True)
    if source == "new":
        shutil.copytree(PROJECT, pod, ignore=shutil.ignore_patterns("__pycache__", ".git", "release", "*.zip"))
    else:
        pod.mkdir(parents=True)
        arch = subprocess.run(["git", "-C", str(PROJECT), "archive", source], stdout=subprocess.PIPE)
        if arch.returncode:
            return None
        subprocess.run(["tar", "-x", "-C", str(pod)], input=arch.stdout, check=True)
    for v in ("0.1.2", "0.1.10"):
        (pod / f"release/updown-{v}-linux-x64").mkdir(parents=True)
    return pod


def run(pod, door, args=(), env=None, session=False, cwd=None):
    ws = pod.parents[2]
    log, state = ws / "fake_docker.log", ws / "fake_state"
    log.write_text("")
    home = ws / "home"
    e = {**os.environ, "PATH": f"{BIN}:{os.environ['PATH']}", "FAKE_LOG": str(log), "FAKE_STATE": str(state),
         "HOME": str(home), **(env or {})}
    e.pop("LOG_FILE", None)
    r = subprocess.run(["bash", str(pod / door), *args], cwd=cwd or pod, env=e, text=True, stdout=subprocess.PIPE,
                       stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL, start_new_session=session, timeout=60)
    return r.stdout.replace(str(ws), "<WS>"), r.returncode, log.read_text().replace(str(ws), "<WS>")


def same(name, door, args=(), env=None, state=None, expect=None, prepare=None, session=False):
    res = {}
    for side, pod in PODS.items():
        ws = pod.parents[2]
        (ws / "fake_state").write_text("\n".join(state or []) + "\n")
        shutil.rmtree(ws / "home", ignore_errors=True)
        (ws / "home").mkdir()
        if prepare:
            prepare(ws)
        res[side] = run(pod, door, args, env, session=session)
    new = res["new"]
    good = expect(*new) if expect else True
    if "old" in res:
        good = good and res["old"] == new
    check(name, good, "\n".join(f"  {s} rc={r[1]}:\n{r[0]}--- docker calls:\n{r[2]}" for s, r in res.items()))


TMP = pathlib.Path(tempfile.mkdtemp(prefix="updown-doors-"))
try:
    BIN = TMP / "bin"
    BIN.mkdir()
    (BIN / "docker").write_text(FAKE_DOCKER)
    (BIN / "docker").chmod(0o755)

    # H0 — the doors, with the fake docker first on PATH: a --help that is not caught would show docker calls
    bad = []
    for d in DOORS:
        if starter and starter.complex_logic(PROJECT / d):
            bad.append(f"{d}: {starter.complex_logic(PROJECT / d)}")
        log = TMP / "help.log"
        log.write_text("")
        h = subprocess.run(["bash", str(PROJECT / d), "--help"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                           text=True, timeout=30, env={**os.environ, "PATH": f"{BIN}:{os.environ['PATH']}",
                                                       "FAKE_LOG": str(log), "FAKE_STATE": str(TMP / "none")}).stdout
        if not h or os.path.basename(d) not in h.splitlines()[0] or log.read_text():
            bad.append(f"{d} --help: {h[:80]!r} docker: {log.read_text()[:80]!r}")
    for f in [*HERE.glob("*.py"), *(PROJECT / "dockerspace/container_scripts").glob("*.py")]:
        try:
            compile(f.read_text(), str(f), "exec")
        except SyntaxError as e:
            bad.append(f"{f.name}: {e}")
    check("H0 the doors: invoking lines only · --help shows the guide, runs nothing · every .py compiles", not bad,
          "\n".join(bad))

    PODS = {"new": make_ws(TMP / "new/ws", "new")}
    old = make_ws(TMP / "old/ws", OLD_REF)
    if old:
        PODS["old"] = old
    else:
        print(f"\033[33m[ WARN ]\033[0m OLD_REF {OLD_REF} not in git — only the new side runs (no old = new)")
    LINK = {"FAKE_LINK": "http://localhost:8070"}

    # H1 — status
    same("H1 status --state: running", "status.sh", ["--state"], state=[C], expect=lambda o, rc, d: o == "running\n")
    same("H1 status --state: stopped", "status.sh", ["--state"], expect=lambda o, rc, d: o == "stopped\n")
    same("H1 status: running + the link", "status.sh", state=[C], env=LINK,
         expect=lambda o, rc, d: "link (open in this PC's browser" in o and "8070" in o)
    same("H1 status: running, no link yet", "status.sh", state=[C], expect=lambda o, rc, d: "no link right now" in o)
    same("H1 status: stopped", "status.sh", expect=lambda o, rc, d: o == f"{C}: stopped\n")

    # H2 — stop · clean
    for door in ("stop.sh", f"{H}/stop.sh"):
        same(f"H2 {door}: already stopped", door, expect=lambda o, rc, d: rc == 0 and "already stopped" in o)
        same(f"H2 {door}: stopped", door, state=[C], expect=lambda o, rc, d: rc == 0 and f"{C} stopped" in o)
        same(f"H2 {door}: docker fails → exit 1", door, state=[C], env={"FAKE_STOP_RC": "1"},
             expect=lambda o, rc, d: rc == 1 and "could not stop" in o and "cannot stop" in o)
    same("H2 clean: container there → removed", f"{H}/clean.sh", env={"FAKE_EXISTS": "1"},
         expect=lambda o, rc, d: rc == 0 and f"{C} removed" in o and "rmi" not in d)
    same("H2 clean --image: neither there", f"{H}/clean.sh", ["--image"],
         expect=lambda o, rc, d: rc == 0 and "did not exist" in o and "rmi updown-dev-image:1.0" in d)
    same("H2 clean --image: both there", f"{H}/clean.sh", ["--image"], env={"FAKE_EXISTS": "1"},
         expect=lambda o, rc, d: o.count("removed") == 2)

    # H3 — start
    def xdg(ws):
        (ws / "home/.config").mkdir(parents=True)
        (ws / "home/.config/user-dirs.dirs").write_text('XDG_DESKTOP_DIR="$HOME/Desktop"\nXDG_DOWNLOAD_DIR="$HOME/Herunterladen"\n')

    for door in ("start.sh", f"{H}/start.sh"):
        same(f"H3 {door}: the link → OK", door, env=LINK,
             expect=lambda o, rc, d: rc == 0 and "updown is ready" in o and "http://localhost:8070" in o
             and "chown" in d and "compose up -d" in d)
    same("H3 start --build → compose up -d --build", "start.sh", ["--build"], env=LINK,
         expect=lambda o, rc, d: rc == 0 and "Rebuilding" in o and "up -d --build" in d)
    same("H3 start: Downloads from XDG (user-dirs.dirs)", f"{H}/start.sh", env=LINK, prepare=xdg,
         expect=lambda o, rc, d: "HOST_DOWNLOADS=<WS>/home/Herunterladen" in o)
    same("H3 start: Downloads default ~/Downloads", f"{H}/start.sh", env=LINK,
         expect=lambda o, rc, d: "HOST_DOWNLOADS=<WS>/home/Downloads" in o)
    same("H3 start: no link but running → WARN, exit 0", f"{H}/start.sh",
         expect=lambda o, rc, d: rc == 0 and "gave no link yet" in o)
    same("H3 start: not up → ERROR exit 1", f"{H}/start.sh", env={"FAKE_UP_DIES": "1"},
         expect=lambda o, rc, d: rc == 1 and "did not come up" in o)
    same("H3 start: compose up fails → exit 1", f"{H}/start.sh", env={"FAKE_UP_RC": "3"},
         expect=lambda o, rc, d: rc == 1 and "ready" not in o)
    same("H3 start: the log volume refused → ERROR exit 1, nothing started", f"{H}/start.sh", env={"FAKE_CHOWN_RC": "1"},
         expect=lambda o, rc, d: rc == 1 and "could not be handed" in o and "up -d" not in d)
    good, detail = True, ""
    for side, pod in PODS.items():
        high = pod.parents[2] / "projectspace/updown"
        shutil.copytree(pod, high)
        out, rc, d = run(high, f"{H}/start.sh", env=LINK)
        good = good and rc == 1 and "this pod is not at its folder" in out and "compose" not in d
        detail += f"  {side}: rc={rc}\n{out}{d}"
        shutil.rmtree(high)
    check("H3 start's pod guard: one level too high → refuses, nothing started", good, detail)

    # H4 — exec · shell · build
    same("H4 exec: not running → exit 1", f"{H}/exec.sh", ["go", "version"],
         expect=lambda o, rc, d: rc == 1 and "is not running" in o)
    same("H4 exec: no command → usage, exit 2", f"{H}/exec.sh", state=[C], expect=lambda o, rc, d: rc == 2 and "usage" in o)
    same("H4 exec: the command in /updown, its exit code", f"{H}/exec.sh", ["go", "version"], state=[C],
         env={"FAKE_EXEC_RC": "4"}, expect=lambda o, rc, d: rc == 4 and "exec -w /updown updown-dev-container go version" in d)
    same("H4 shell: not running → exit 1", f"{H}/shell.sh", expect=lambda o, rc, d: rc == 1)
    same("H4 shell: bash -it in /updown", f"{H}/shell.sh", state=[C],
         expect=lambda o, rc, d: "exec -it -w /updown updown-dev-container bash" in d)
    same("H4 build: the container build with the arguments", f"{H}/build.sh", ["--target", "linux-x64"], state=[C],
         expect=lambda o, rc, d: rc == 0 and "container_scripts/build.sh --target linux-x64" in d and "run" not in d.split())
    same("H4 build --test → then test_release linux-x64", f"{H}/build.sh", ["--all", "--test"], state=[C],
         env={"FAKE_IMAGES": "ubuntu:24.04"},
         expect=lambda o, rc, d: "build.sh --all" in d and "--test" not in d and "fake run in /opt/updown" not in d)
    same("H4 build: the build fails → exit 1, no test", f"{H}/build.sh", ["--all", "--test"], state=[C],
         env={"FAKE_EXEC_RC": "1"}, expect=lambda o, rc, d: rc == 1 and "Release" not in o)

    # H5 — pull_images · test_release
    imgs = "golang:1.25-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437 ubuntu:24.04"
    same("H5 pull_images: here · pulled · missing → exit 1", f"{H}/pull_images.sh",
         env={"FAKE_IMAGES": imgs, "FAKE_PULLABLE": "debian:12-slim"},
         expect=lambda o, rc, d: rc == 1 and "already here" in o and "debian:12-slim — pulled" in o
         and "still missing: fedora:41" in o)
    same("H5 pull_images: all here → exit 0", f"{H}/pull_images.sh",
         env={"FAKE_IMAGES": imgs + " debian:12-slim fedora:41"}, expect=lambda o, rc, d: rc == 0 and "pull" not in d)
    TR = f"{H}/test_release.sh"
    same("H5 test_release: no --target → usage, exit 2", TR, expect=lambda o, rc, d: rc == 2 and "usage" in o)
    same("H5 test_release: unknown option → exit 2", TR, ["--nope"], expect=lambda o, rc, d: rc == 2 and "unknown option" in o)
    same("H5 test_release: no such release → exit 1", TR, ["--target", "macos-x64"],
         expect=lambda o, rc, d: rc == 1 and "build it first" in o)
    same("H5 test_release: the NEWEST release (version order) in every image, offline", TR, ["--target", "linux-x64"],
         env={"FAKE_IMAGES": "ubuntu:24.04 debian:12-slim fedora:41"},
         expect=lambda o, rc, d: rc == 0 and "updown-0.1.10-linux-x64" in o and d.count("--network none") == 3)
    same("H5 test_release: an image missing + one fails → exit 1, listed", TR,
         ["--target", "linux-x64", "--image", "ubuntu:24.04 nope:1"], env={"FAKE_IMAGES": "ubuntu:24.04", "FAKE_RUN_RC": "1"},
         expect=lambda o, rc, d: rc == 1 and "failed in: ubuntu:24.04 nope:1" in o)
    same("H5 test_release --cmd", TR, ["--target", "linux-x64", "--image", "ubuntu:24.04", "--cmd", "./start.sh --version"],
         env={"FAKE_IMAGES": "ubuntu:24.04"}, expect=lambda o, rc, d: rc == 0 and "./start.sh --version" in d)

    # H6 — container login
    LG = f"{H}/updown_container_login.sh"
    same("H6 login: unknown container → refused", LG, ["nope"], expect=lambda o, rc, d: rc == 1 and "is not one of" in o)
    same("H6 login: Docker down → refused", LG, env={"FAKE_DOCKER_DOWN": "1"},
         expect=lambda o, rc, d: rc == 1 and "Docker is not running" in o)
    same("H6 login: not running → how to start it", LG, expect=lambda o, rc, d: rc == 1 and "start it: bash" in o)
    same("H6 login: no terminal → refused", LG, state=[C], session=True,
         expect=lambda o, rc, d: rc == 1 and "terminal" in o)
finally:
    shutil.rmtree(TMP, ignore_errors=True)

if FAILED:
    print(f"\033[31m{len(FAILED)} FAILED\033[0m: " + " · ".join(f.split(":")[0] for f in FAILED))
    sys.exit(1)
print(f"\033[32mALL PASS\033[0m — old = new{'' if 'old' in PODS else ' (new only)'}")
