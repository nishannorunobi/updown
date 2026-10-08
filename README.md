# updown

```
bash start.sh      # starts updown — then open http://localhost:8070 on this PC (always the same), scan a QR code with the phone
bash status.sh     # running? + the current link
bash stop.sh
```

Offline file transfer between a PC and a phone: the PC runs a small server, the phone
scans a QR code and uses one web page to upload and download — nothing installed on the
phone, no internet at install or run time. Working name; the product name comes later.

Tracked in Plane (`myportfolio` workspace, project `UPDOWN`). v1 scope: UPDOWN-8.
Language: Go (single self-contained binary per OS).
The contract every build follows (package, settings, command line, token, HTTP API, page,
stopping, shared tests): [docs/contract.md](docs/contract.md).

## Development — everything in Docker

Nothing is installed on the host PC: the Go toolchain, builds and tests run in the
`updown-dev-container` (Go 1.25, offline settings: `GOTOOLCHAIN=local`, `GOPROXY=off`,
`CGO_ENABLED=0`).

```
bash start.sh [--build]        # start the dev container (idempotent; --build rebuilds the image)
bash stop.sh                   # stop it (idempotent)
bash status.sh [--state]       # running / stopped

bash dockerspace/host_scripts/exec.sh <cmd>     # run a command inside, in /updown
bash dockerspace/host_scripts/shell.sh          # interactive shell inside
bash dockerspace/host_scripts/pull_images.sh    # the ONE online step: fetch the images once
bash dockerspace/host_scripts/test_release.sh --target linux-x64   # release test, network OFF
bash dockerspace/host_scripts/clean.sh [--image]                   # remove container (+ dev image)
bash dockerspace/host_scripts/exec.sh bash dockerspace/container_scripts/env_check.sh
bash dockerspace/host_scripts/exec.sh go test ./...           # unit tests
bash dockerspace/host_scripts/test_agent.sh                   # updown-agent: link, restarts (crash, idle), clean stop
bash dockerspace/host_scripts/build.sh --all [--test]         # release packages (+ offline Linux test)
bash dockerspace/host_scripts/build.sh --target linux-x64     # one target
bash dockerspace/host_scripts/exec.sh go run ./cmd/updown --check
```

```
updown/
├── release.conf                     PRODUCT_NAME + VERSION — the only place for them
├── go.mod                           Go module (no external dependencies)
├── cmd/updown/                      the program: --version, --check, (server: next task)
├── internal/kv/                     KEY=VALUE reader (release.conf, updown.conf)
├── internal/release/                release.conf: find + load (name, version, target)
├── internal/config/                 updown.conf: defaults + validation (docs/contract.md §2)
├── internal/connection/             the ways to the phone, ONE FILE EACH: hotspot.go, usb.go, bluetooth.go,
│                                    wifi.go, lan.go · device_connection.go (the phone's browser)
├── web/                             the phone page + the PC's page (host.html + parts/: one file per block)
├── packaging/                       copied into every release: unix/start.sh,
│                                    windows/start.cmd, updown.conf (defaults)
├── docs/contract.md                 what every build must do
├── tools/mkzip/                     build-time zip tool (standard library only)
├── start.sh, stop.sh, status.sh     start / stop the container; start + status show the link (workspace: svcmgt)
├── updown-agent/                    the container's ENTRY POINT: builds + runs updown, hands out the link
│   ├── agent.py                     (python3, standard library) · `agent.py link` = the current link
│   ├── updown.conf                  the service's settings (STOP_MODE=idle, 15 min, Downloads, CONTROLS_URL …)
│   ├── actions.conf                 the page's buttons, one line each → a file in actions/
│   └── actions/                     one file per button: check_now.py, new_link.py, network_mode.py
├── dockerspace/
│   ├── Dockerfile                   dev image: Go toolchain (never on the host)
│   ├── docker-compose.yml           updown-dev-container · this PC's network (network_mode: host) · 8070-8079
│   ├── .env                         GO_IMAGE, TEST_IMAGES (non-secret)
│   ├── host_scripts/                start, stop, exec, shell, clean, pull_images, build, test_release, test_agent, test_controls
│   └── container_scripts/           env_check, build (run inside the container)
└── release/                         build output (git-ignored): <RELEASE>/ + <RELEASE>.zip + .zip.sha256
```

Mounts: the project at `/updown`; the Go cache in
`mountspace/data/myworkspace/projectspace/updown/gocache`; logs in
`mountspace/logs/myworkspace/projectspace/updown/` (workspace mirror-log convention).
The container runs as the host user, so every file it writes stays owned by you.
Release tests run each release read-only in clean `ubuntu:24.04`, `debian:12-slim` and
`fedora:41` containers with `--network none`, as a non-root user — proving an offline install.

## Release packages

`build.sh` reads `PRODUCT_NAME` + `VERSION` from `release.conf` and makes, per target
(`linux-x64`, `linux-arm64`, `windows-x64`, `macos-arm64`, `macos-x64`), a folder with
ONLY that OS's files — `bin/updown[.exe]`, `start.sh` or `start.cmd`, `updown.conf`,
`release.conf` (+ `TARGET`, `RELEASE`, `BUILD`) — plus a zip and its SHA-256. Windows text
files get CRLF line endings. `go vet` + `go test` run first; a failed build never replaces
a good package.

## Using it — updown runs as a service ([2.1], UPDOWN-26)

`bash start.sh` starts the container; inside, **updown-agent** (`updown-agent/agent.py`, the entry point)
builds updown from this source (offline), starts it, and hands out its link. start.sh prints it:

```
[  OK  ] updown-dev-container running — updown is ready
         open this link in this PC's browser for the QR codes (send / receive files):
         http://172.20.10.2:8070/host?k=…
```

Open it in this PC's browser: **two QR codes** and the live log.

- **Send files** QR → the phone's upload screen; files land in this PC's **Downloads**.
- **Receive files** QR → the phone's download screen: this PC's **Downloads** files, newest
  first — tap one to save it on the phone.
- The container uses this PC's network. By default (`BIND=hotspot`) updown serves **only on your phone's
  hotspot** (USB or Wi-Fi) — never on a foreign Wi-Fi. On another network the agent waits ("waiting — no
  phone hotspot …") and starts updown within seconds of this PC joining the hotspot; when the address
  changes it restarts updown with a new link. `BIND=auto` (any network) is in `updown-agent/updown.conf`.
- **The page is always http://localhost:8070** (HOST_PORT) — only this PC can open it, the address never
  changes. updown itself opens the phone's door on the hotspot when it appears, moves it when the address
  changes and closes it when the hotspot is gone (the page turns red); each new door = new QR codes.
- **The buttons** on the page ([2.7], `docs/ui/host_page_buttons_*.png`): 📶 Hotspot only / 🌐 Any network
  (writes `BIND` into `updown-agent/updown.conf`, restarts updown), ⟳ Check now, 🔑 New link. Each is one line
  in `updown-agent/actions.conf` + one file in `updown-agent/actions/` — a new button = a new line + a new file.
- After 15 idle minutes updown stops and the agent starts it again (same page). A crash is restarted the
  same way. Settings: `updown-agent/updown.conf`.

