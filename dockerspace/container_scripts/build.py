#!/usr/bin/env python3
"""build.py — (inside updown-dev-container) build release packages. The door: build.sh (its --help).

For each target, from the project root:
    release/<RELEASE>/            only the files that OS needs (FILES_FOR)
    release/<RELEASE>.zip         the package to hand out          release/<RELEASE>.zip.sha256   its checksum
RELEASE = PRODUCT_NAME-VERSION-TARGET from release.conf (the only place for name + version); the package's own
release.conf adds TARGET, RELEASE, BUILD. Offline: static pure-Go binaries, no module downloads.
A target is swapped in only when complete — a failed build never leaves a half package.
Exit 0 every requested target built · 1 a failure · 2 wrong arguments.
"""
import hashlib
import os
import shutil
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import container_door as d

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.chdir(ROOT)
ALL_TARGETS = "linux-x64 linux-arm64 windows-x64 macos-arm64 macos-x64"
GOOS_GOARCH = {"linux-x64": ("linux", "amd64"), "linux-arm64": ("linux", "arm64"), "windows-x64": ("windows", "amd64"),
               "macos-arm64": ("darwin", "arm64"), "macos-x64": ("darwin", "amd64")}
ok = lambda t: d.say(f"\033[32m[  OK  ]\033[0m {t}")
err = lambda t: d.say(f"\033[31m[ERROR]\033[0m  {t}")


def files_for(target):
    """(source, destination) — ONLY what that OS needs."""
    start = ("packaging/windows/start.cmd", "start.cmd") if target.startswith("windows-") \
        else ("packaging/unix/start.sh", "start.sh")
    return [start, ("packaging/updown.conf", "updown.conf")]


def crlf(path):
    """Windows text files: every line ends in \\r\\n (as sed 's/\\r*$/\\r/')."""
    text = open(path, newline="").read()
    trailing = text.endswith("\n")
    lines = text.split("\n")[:-1] if trailing else text.split("\n")
    open(path, "w", newline="").write("\n".join(line.rstrip("\r") + "\r" for line in lines) + ("\n" if trailing else ""))


def du(path):
    return subprocess.run(["du", "-h", path], stdout=subprocess.PIPE, text=True).stdout.split("\t")[0]


targets, tests = "", True
args = sys.argv[1:]
while args:
    a = args.pop(0)
    if a == "--target":
        targets += " " + (args.pop(0) if args else "")
    elif a == "--all":
        targets = ALL_TARGETS
    elif a == "--skip-tests":
        tests = False
    else:
        err(f"unknown option: {a} (try --help)")
        sys.exit(2)
if not targets.replace(" ", ""):
    err(f"usage: build.sh --target <{'|'.join(ALL_TARGETS.split())}> | --all")
    sys.exit(2)

conf = {}
for line in open(os.path.join(ROOT, "release.conf")).read().splitlines():
    if "=" in line and not line.lstrip().startswith("#"):
        k, _, v = line.partition("=")
        conf[k.strip()] = v.strip().strip('"').strip("'")
name, version = conf.get("PRODUCT_NAME", ""), conf.get("VERSION", "")
if not name or not version:
    err("release.conf needs PRODUCT_NAME and VERSION")
    sys.exit(1)

if tests:
    d.say("==> go vet + go test")
    if d.run(["go", "vet", "./..."]) or d.run(["go", "test", "./..."]):
        err("tests failed — nothing built")
        sys.exit(1)

failed = []
os.makedirs("release", exist_ok=True)
for t in targets.split():
    if t not in GOOS_GOARCH:
        err(f"unknown target: {t}")
        failed.append(t)
        continue
    goos, goarch = GOOS_GOARCH[t]
    rel = f"{name}-{version}-{t}"
    binary = f"bin/{name}" + (".exe" if goos == "windows" else "")
    stage = f"release/.{rel}.tmp"
    d.say(f"==> {rel}  (GOOS={goos} GOARCH={goarch})")
    shutil.rmtree(stage, ignore_errors=True)
    os.makedirs(f"{stage}/bin")
    if d.run(["go", "build", "-trimpath", "-ldflags", "-s -w", "-o", f"{stage}/{binary}", "./cmd/updown"],
             env={**os.environ, "GOOS": goos, "GOARCH": goarch}):
        err(f"{t}: go build failed")
        shutil.rmtree(stage, ignore_errors=True)
        failed.append(t)
        continue
    missing = next((src for src, _ in files_for(t) if not os.path.isfile(src)), None)
    if missing:
        err(f"{t}: missing {missing}")
        failed.append(t)
        continue
    for src, dst in files_for(t):
        shutil.copy(src, f"{stage}/{dst}")
    kept = [l for l in open("release.conf").read().splitlines() if not l.startswith("#") and l]
    open(f"{stage}/release.conf", "w").write("\n".join(kept + [f"TARGET={t}", f"RELEASE={rel}", f"BUILD={binary}"]) + "\n")
    if goos == "windows":
        for f in sorted(os.listdir(stage)):
            if f.endswith((".cmd", ".conf")):
                crlf(f"{stage}/{f}")
    else:
        for f in (f"{stage}/start.sh", f"{stage}/{binary}"):
            os.chmod(f, os.stat(f).st_mode | 0o111)
    # Swap in only when complete: a failed build never leaves a half package.
    shutil.rmtree(f"release/{rel}", ignore_errors=True)
    os.rename(stage, f"release/{rel}")
    for f in (f"release/{rel}.zip", f"release/{rel}.zip.sha256"):
        if os.path.exists(f):
            os.remove(f)
    if d.run(["go", "run", "./tools/mkzip", f"release/{rel}", f"release/{rel}.zip"]) == 0:
        digest = hashlib.sha256(open(f"release/{rel}.zip", "rb").read()).hexdigest()
        open(f"release/{rel}.zip.sha256", "w").write(f"{digest}  {rel}.zip\n")
        files = sorted(os.path.relpath(os.path.join(dp, f), f"release/{rel}")
                       for dp, _, fs in os.walk(f"release/{rel}") for f in fs)
        ok(f"{rel} — {du(f'release/{rel}/{binary}')} program · {du(f'release/{rel}.zip')} zip · files: "
           + "".join(f + " " for f in files))
    else:
        err(f"{t}: zip failed")
        failed.append(t)

if failed:
    err(f"failed: {' '.join(failed)}")
    sys.exit(1)
ok(f"built:{targets} → release/")
sys.exit(0)
