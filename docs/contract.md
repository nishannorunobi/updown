# updown — contract (v1)

Every build of updown, for every OS, follows this document, and one shared test suite
(UPDOWN-14) checks every build against it. Change this file first, then the code.
Scope: Plane UPDOWN-8. Language: Go (UPDOWN-11). Environment: `dockerspace/` (UPDOWN-18).

## 1. Release package

One package per target, containing only what that target needs:

```
<RELEASE>/                    e.g. updown-0.1.0-linux-x64/
├── release.conf              what this package is (below) — the ONLY place for name + version
├── updown.conf               settings (section 2), all optional
├── start.sh | start.cmd      reads release.conf, runs BUILD, passes every argument through
└── bin/updown[.exe]          the program; the web page is embedded in it
```

Targets: `linux-x64`, `linux-arm64`, `windows-x64`, `macos-arm64`, `macos-x64`.

### release.conf

Plain `KEY=VALUE` lines, `#` comments, no spaces around `=`, values without spaces
(so `start.sh` can `.` it and `start.cmd` can parse it with `for /f`). Written once per
release by the build; never edited by hand.

| Key | Example | Meaning |
|---|---|---|
| `PRODUCT_NAME` | `updown` | product name (working name; renamed later in this one place) |
| `VERSION` | `0.1.0` | release version |
| `TARGET` | `linux-x64` | what this package is built for — no OS detection anywhere |
| `RELEASE` | `updown-0.1.0-linux-x64` | `PRODUCT_NAME-VERSION-TARGET`, also the folder name |
| `BUILD` | `bin/updown` | the program, relative to the package |

The program reads `release.conf` from its package at start; it never has the name or the
version compiled in.

## 2. Settings — updown.conf

Same `KEY=VALUE` format. Every key is optional; an empty or missing key takes the default.
`--config FILE` points to another file.

| Key | Default | Meaning |
|---|---|---|
| `RECEIVE_DIR` | the OS Downloads folder¹ | where uploads from the phone are saved |
| `SHARE_DIR` | the OS Downloads folder¹ | the folder whose files the phone sees on the Receive screen (not recursive) |
| `STOP_MODE` | `enter` | `enter` (Enter in the terminal) · `after-transfer` (after the first completed upload or download) · `idle` (no request for `IDLE_MINUTES`) |
| `IDLE_MINUTES` | `15` | used by `STOP_MODE=idle`; also a hard stop for the other modes when nothing happens |
| `PORT` | `0` | `0` = first free port from 8070 to 8079; otherwise that port |
| `BIND` | `auto` | `auto` = pick the phone-reachable address (order: phone hotspot 172.20.10.x / USB tether, then Wi-Fi, then other private LAN) · `hotspot` = ONLY the phone hotspot / USB tether, never another network (none → exit 1 with the reason) · or one IPv4 address |
| `ADVERTISE` | empty | the address put in the QR link when it differs from `BIND` (the dev container binds 0.0.0.0 and advertises the host's hotspot address) |
| `MAX_FILE_GB` | `10` | largest single upload |

¹ Linux: `XDG_DOWNLOAD_DIR` from `~/.config/user-dirs.dirs`, else `~/Downloads`;
Windows: `%USERPROFILE%\Downloads`; macOS: `~/Downloads`. Created if missing.

Owner, 2026-09-30: the Receive screen lists the PC's **Downloads folder** by default — so by
default uploads and downloads use the same folder. Hidden files (name starting with `.`) and
part files of running uploads are never listed.

## 3. Command line

```
start.sh [--config FILE] [--check] [--version] [--address] [FILE ...]
```

- `FILE ...` — extra files offered to the phone (listed before `SHARE_DIR`'s files).
- `--version` — prints `RELEASE` and exits 0.
- `--address` — prints the address the settings would listen on (BIND) and exits 0; none (e.g.
  `BIND=hotspot` and no hotspot) → the reason on one line, exit 1. Used by updown-agent to follow the
  PC's network.
- `--check` — the release self-test used by `test_release.sh`: prints
  `<RELEASE> (version <VERSION>)`, validates `release.conf` and `updown.conf`, confirms it can
  listen on 127.0.0.1, exits 0; anything wrong → the reason on one line, exit 1. Needs no
  network, no write access, runs as any user.
- Exit codes: `0` normal stop · `1` error (config, bind, folder) · `2` wrong usage.

## 4. Start-up output and the host page

The QR code is **never drawn in the console** (owner, 2026-09-30). The console prints only
lines starting with `[updown]`:

1. `RELEASE`, receive folder, offered files, stop mode; the listen address.
2. `open this page on this PC to show the QR codes: http://<address>:<port>/host?k=<host key>`
3. One line per event: phone connected, upload started/finished (name, size, SHA-256,
   seconds), download finished, refused request (reason), stop (reason).

The phone link (with its token) is never printed to the console or a log.

**Host page** — for this PC's browser, protected by its own key (`?k=`, 32 random bytes per
start, different from the phone token; the phone token cannot open it, and it never takes
the phone's session or counts as activity):

| Method + path | Does |
|---|---|
| `GET /host?k=` | the page: **two QR codes** — "Send files" (the phone's upload screen) and "Receive files" (the phone's download screen) — plus who is connected and the live log (one file, no external URL) |
| `GET /host/qr/send.svg?k=` | QR code of the send link (SVG, black on white, quiet zone 4) |
| `GET /host/qr/receive.svg?k=` | QR code of the receive link |
| `GET /host/api/status?k=&after=N` | `{"product","version","send","receive","phone","stop_mode","log":[{"n","time","line"}]}` — log lines after N (last 500 kept) |

## 5. The link and the token

- Two links, one per QR code, same token:
  - send: `http://<address>:<port>/send?t=<token>` → the phone's **upload** screen
  - receive: `http://<address>:<port>/receive?t=<token>` → the phone's **download** screen
- `token`: 32 random bytes, base64url without padding, new every start. It exists only in
  memory and in the QR code.
- Every request must carry it as `?t=<token>` (downloads are plain links, which cannot add
  headers). Without it, or with a wrong one: `403`, empty body. Compared in constant time.
- One phone at a time: the first client address that presents the token owns the session;
  requests from any other address get `403` until the server restarts.

## 6. HTTP API

Plain HTTP (HTTPS later, UPDOWN-10). All responses carry `Cache-Control: no-store`.
JSON errors: `{"error":"<short reason>"}`.

| Method + path | Does | Success |
|---|---|---|
| `GET /send`, `GET /receive` | the phone page, showing the upload or the download screen: one HTML file, CSS + JS inline, **no external URL of any kind**; header `Content-Security-Policy: default-src 'self' 'unsafe-inline'` | `200` |
| `GET /api/info` | `{"product","version","receive_dir","share_dir","stop_mode","max_file_bytes"}` (folder names only, not full paths) | `200` |
| `GET /api/files` | offered files, newest first: `[{"name","size","modified"}]` (`modified` = RFC 3339) | `200` |
| `GET /api/files/<name>` | the file; `Content-Disposition: attachment`, `Content-Length`, supports `Range` (phones use it for video) | `200` / `206` |
| `PUT /api/upload/<name>` | raw body = the file, streamed to disk; `Content-Length` required | `201 {"name","size","sha256"}` |

Upload rules:
- `<name>` is URL-encoded; only its last path element is used; control characters,
  `/ \ : * ? " < > |` and leading dots are replaced by `_`; empty → `upload`.
- Written to `<name>.<random>.updown-part` in `RECEIVE_DIR` (unique, so two uploads of the
  same name never share a part file), SHA-256 computed while streaming,
  renamed to the final name only when complete. If the name exists: `name (1).ext`,
  `name (2).ext`, … — never overwrites.
- Memory use stays constant for any size (streaming, 1 MiB buffers).
- Errors: `400` bad name / missing `Content-Length` · `403` token/session · `404` unknown
  file · `413` larger than `MAX_FILE_GB` · `507` disk full (the part file is removed) ·
  connection lost → part file removed (resume is not in v1).

## 7. The page (phone)

One page file, two screens — which one depends on the QR code scanned (`/send` or
`/receive`); works in iPhone Safari (v1) without anything installed:
- **Upload** (`/send`) — file picker (`multiple`; the iPhone offers Photo Library, Take Photo,
  Choose File); each file is sent with `PUT` via `XMLHttpRequest` (for the progress bar);
  after each: name, size, "saved on the PC ✓".
- **Download** (`/receive`) — the PC's Downloads folder files from `/api/files`, newest first,
  each a link to `/api/files/<name>?t=<token>` (tap = save on the phone).
- The token is read from the page's own URL and kept in memory only.
- No external fonts, scripts, images or analytics.

## 8. Stopping

- `enter`: Enter in the terminal where it runs.
- `after-transfer`: after the first completed upload or download (a finished download = all
  bytes sent).
- `idle`: no request for `IDLE_MINUTES`.
- Every mode also stops on Ctrl+C / SIGTERM: open transfers are cut, part files removed.

## 9. Shared test suite (UPDOWN-14)

Run against every build (Linux builds in the clean test containers, network off):
`--check` · `--version` · request without / with a wrong token → 403 · second client
address → 403 · upload a file → same SHA-256 on disk, no part file left · name
sanitising + `name (1).ext` collision · upload over `MAX_FILE_GB` → 413 · download →
same bytes, `Range` → 206 · the phone and host pages contain no `http://` or `https://`
reference · host page needs its own key · each `STOP_MODE` stops as described.
