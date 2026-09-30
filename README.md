# updown

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
bash dockerspace/host_scripts/run.sh [--stop MODE]            # run the server for a real phone test
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
├── web/                             the phone page, embedded into the program
├── packaging/                       copied into every release: unix/start.sh,
│                                    windows/start.cmd, updown.conf (defaults)
├── docs/contract.md                 what every build must do
├── tools/mkzip/                     build-time zip tool (standard library only)
├── start.sh, stop.sh, status.sh     dev container entry points (workspace: svcmgt/svc_policy.conf)
├── dockerspace/
│   ├── Dockerfile                   dev image: Go toolchain (never on the host)
│   ├── docker-compose.yml           updown-dev-container · my_docker_network 172.28.0.70 · ports 8070-8079
│   ├── .env                         GO_IMAGE, TEST_IMAGES (non-secret)
│   ├── host_scripts/                start, stop, exec, shell, clean, pull_images, build, test_release
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

## Testing with the phone (development)

`bash dockerspace/host_scripts/run.sh` runs the server inside the dev container: it finds
this PC's phone-reachable address (phone hotspot / USB tether first, then Wi-Fi), and
Docker's published ports 8070–8079 carry the phone in. The console prints only a link to
the host page plus the event log. Open that link in this PC's browser: it shows **two QR
codes** and the live log.

- **Send files** QR → the phone's upload screen; files land in this PC's **Downloads**.
- **Receive files** QR → the phone's download screen: this PC's **Downloads** files, newest
  first — tap one to save it on the phone.
- `--stop enter | after-transfer | idle` (`--idle N` minutes); Enter or Ctrl+C stops it;
  a closed terminal or killed run.sh also stops the server in the container.

