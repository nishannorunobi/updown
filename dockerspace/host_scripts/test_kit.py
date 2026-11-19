#!/usr/bin/env python3
"""test_kit.py — what updown's product tests share (test_agent · test_controls · test_long_transfer).

    ok(text) / bad(text) · FAILED      the [  OK  ] / [ERROR] lines; bad() remembers the failure
    http(url, method, headers, timeout, data) → (status, body)   0 = nothing answered · redirects are answers (curl)
    js(body) → the JSON (None when it is not JSON)
    docker(*args) → (exit code, stdout+stderr) · logs(name, since, until) → the container's output
    conf(text, **keys) → updown.conf with KEY=… lines replaced (sed 's/^KEY=.*/KEY=value/')
    warm_cache(dst)                    a copy of this workspace's Go cache: an offline build
    open_tmp(*folders) → a temp folder with those sub-folders
Standard library only.
"""
import json
import os
import re
import shutil
import subprocess
import tempfile
import urllib.error
import urllib.request

FAILED = []


def ok(text):
    print(f"\033[32m[  OK  ]\033[0m {text}", flush=True)


def bad(text):
    print(f"\033[31m[ERROR]\033[0m  {text}", flush=True)
    FAILED.append(text)


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


_OPENER = urllib.request.build_opener(_NoRedirect)


def http(url, method="GET", headers=None, timeout=5, data=None):
    req = urllib.request.Request(url, method=method, data=data, headers=headers or {})
    try:
        with _OPENER.open(req, timeout=timeout) as r:
            return r.status, r.read().decode(errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode(errors="replace")
    except (OSError, ValueError):
        return 0, ""


def js(body):
    try:
        return json.loads(body)
    except (ValueError, TypeError):
        return None


def docker(*args):
    r = subprocess.run(["docker", *args], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, errors="replace")
    return r.returncode, r.stdout


def logs(name, since=None, until=None):
    extra = (["--since", str(since)] if since else []) + (["--until", str(until)] if until else [])
    return docker("logs", *extra, name)[1]


def conf(text, **keys):
    for key, value in keys.items():
        text = re.sub(rf"^{key}=.*$", lambda _: f"{key}={value}", text, flags=re.M)
    return text


def warm_cache(ws, dst):
    src = os.path.join(ws, "mountspace/data/myworkspace/projectspace/updown/gocache")
    try:
        shutil.copytree(src, dst, dirs_exist_ok=True, symlinks=True)
    except (OSError, shutil.Error):
        pass                                           # a cold cache only makes the first build slower


def open_tmp(*folders):
    tmp = tempfile.mkdtemp()
    for f in folders:
        os.makedirs(os.path.join(tmp, f), exist_ok=True)
    return tmp


def all_writable(path):
    """chmod -R a+rwX — the container runs as this user but other uids must reach it too."""
    for dp, dirs, files in os.walk(path):
        for name in [dp] + [os.path.join(dp, f) for f in files]:
            mode = os.stat(name).st_mode
            os.chmod(name, mode | 0o666 | (0o111 if os.path.isdir(name) or mode & 0o111 else 0))
