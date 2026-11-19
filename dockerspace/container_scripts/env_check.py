#!/usr/bin/env python3
"""env_check.py — (inside updown-dev-container) is the dev environment ready? The door: env_check.sh (its --help).

Go present and pinned offline (GOTOOLCHAIN=local · GOPROXY=off · CGO_ENABLED=0) · its caches writable · a tiny pure-Go
program cross-compiles for every product target with no network · go run works. Exit 0 ready · 1 not (listed).
"""
import os
import shutil
import subprocess
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import container_door as d

failed = []
ok = lambda t: d.say(f"\033[32m[  OK  ]\033[0m {t}")


def bad(text):
    d.say(f"\033[31m[ERROR]\033[0m  {text}")
    failed.append(text)


def go(*args, cwd=None, env=None):
    r = subprocess.run(["go", *args], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, cwd=cwd, env=env)
    return r.returncode, r.stdout.strip()


if shutil.which("go"):
    ok(go("version")[1])
else:
    bad("go not found")
for key, want, meaning in (("GOTOOLCHAIN", "local", "never downloads a toolchain"),
                           ("GOPROXY", "off", "never fetches modules"),
                           ("CGO_ENABLED", "0", "static binaries")):
    value = go("env", key)[1]
    if value == want:
        ok(f"{key}={want} ({meaning})")
    else:
        bad(f"{key} is {value}")
for folder in (go("env", "GOCACHE")[1], go("env", "GOPATH")[1]):
    try:
        os.makedirs(folder, exist_ok=True)
        writable = os.access(folder, os.W_OK)
    except OSError:
        writable = False
    ok(f"writable: {folder}") if writable else bad(f"not writable: {folder}")

# Cross-compile a throwaway program for every product target (no network needed).
tmp = tempfile.mkdtemp()
try:
    open(f"{tmp}/go.mod", "w").write("module check\n\ngo 1.25\n")
    open(f"{tmp}/main.go", "w").write('package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("ok") }\n')
    for tgt in ("linux/amd64", "linux/arm64", "windows/amd64", "darwin/arm64", "darwin/amd64"):
        goos, goarch = tgt.split("/")
        rc, out = go("build", "-o", f"{tmp}/out-{goos}-{goarch}", ".", cwd=tmp,
                     env={**os.environ, "GOOS": goos, "GOARCH": goarch})
        if out:
            d.say(out)
        ok(f"cross-compile {tgt}") if rc == 0 else bad(f"cross-compile {tgt}")
    ok("go run works") if go("run", ".", cwd=tmp)[1] == "ok" else bad("go run")
finally:
    shutil.rmtree(tmp, ignore_errors=True)

if failed:
    d.say(f"\033[31m[ERROR]\033[0m  not ready: {' '.join(failed)}")
    sys.exit(1)
d.say("\033[32m[  OK  ]\033[0m dev environment ready — Go builds for Linux, Windows and macOS, offline")
sys.exit(0)
