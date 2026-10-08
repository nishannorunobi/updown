// Package config reads updown.conf — the user's settings (docs/contract.md §2). Every key
// is optional; a missing or empty key takes its default. The page's and the network watcher's settings
// ([2.7]: CHECK_SECONDS, CONNECTIONS, PAGE_PARTS, PAGE_REFRESH_SECONDS, LOG_LINES, CONTROLS_URL) never stop
// updown: a wrong value runs on its default and is reported in Warnings.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/nishannorunobi/updown/internal/connection"
	"github.com/nishannorunobi/updown/internal/kv"
)

// Stop modes (STOP_MODE).
const (
	StopEnter         = "enter"
	StopAfterTransfer = "after-transfer"
	StopIdle          = "idle"
)

// Config is updown.conf after defaults and validation.
type Config struct {
	ReceiveDir  string // RECEIVE_DIR — default: the OS Downloads folder
	ShareDir    string // SHARE_DIR — default: the OS Downloads folder (the Receive screen)
	StopMode    string // STOP_MODE
	IdleMinutes int    // IDLE_MINUTES
	Port        int    // PORT — the phone's door: 0 = first free from 8070 to 8079
	HostPort    int    // HOST_PORT — the PC's page at http://localhost:HOST_PORT (loopback only)
	Bind        string // BIND — "auto", "hotspot" or one IPv4 address
	Advertise   string // ADVERTISE — address for the QR link when it differs from BIND
	MaxFileGB   int    // MAX_FILE_GB
	Path        string // the file read ("" = defaults only)

	CheckSeconds   int      // CHECK_SECONDS — how often the phone's network is checked
	Connections    []string // CONNECTIONS — the ways to the phone that are watched, best first (internal/connection)
	PageParts      []string // PAGE_PARTS — the blocks of the PC's page, top to bottom (web/parts/NAME.html)
	PageRefreshSec int      // PAGE_REFRESH_SECONDS — how often the PC's page asks for news
	LogLines       int      // LOG_LINES — log lines kept for the page
	ControlsURL    string   // CONTROLS_URL — where the page finds its buttons (updown-agent); "" = no buttons
	Warnings       []string // values that were wrong and replaced by their default
}

// DefaultPageParts is PAGE_PARTS when updown.conf does not set it.
const DefaultPageParts = "connection,qr_codes,transfer_log"

// Default is the configuration with no updown.conf at all.
func Default() Config {
	return Config{
		ReceiveDir:  DownloadsDir(),
		ShareDir:    DownloadsDir(),
		StopMode:    StopEnter,
		IdleMinutes: 15,
		HostPort:    8070,
		Bind:        "auto",
		MaxFileGB:   10,

		CheckSeconds:   3,
		Connections:    list(connection.DefaultOrder),
		PageParts:      list(DefaultPageParts),
		PageRefreshSec: 1,
		LogLines:       500,
	}
}

// list splits "a, b,c" into its names.
func list(v string) []string {
	var out []string
	for _, n := range strings.Split(v, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Load reads path over the defaults. A missing file is fine when optional is true.
func Load(path string, optional bool) (Config, error) {
	c := Default()
	m, err := kv.ParseFile(path)
	if err != nil {
		if optional && errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return c, err
	}
	c.Path = path
	str := func(key string, dst *string) {
		if v := strings.TrimSpace(m[key]); v != "" {
			*dst = v
		}
	}
	num := func(key string, dst *int) error {
		v := strings.TrimSpace(m[key])
		if v == "" {
			return nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s: %s must be a number, got %q", path, key, v)
		}
		*dst = n
		return nil
	}
	str("RECEIVE_DIR", &c.ReceiveDir)
	str("SHARE_DIR", &c.ShareDir)
	str("STOP_MODE", &c.StopMode)
	str("BIND", &c.Bind)
	str("ADVERTISE", &c.Advertise)
	for key, dst := range map[string]*int{"IDLE_MINUTES": &c.IdleMinutes, "PORT": &c.Port, "HOST_PORT": &c.HostPort, "MAX_FILE_GB": &c.MaxFileGB} {
		if err := num(key, dst); err != nil {
			return c, err
		}
	}
	c.soft(m)
	c.ReceiveDir = expandHome(c.ReceiveDir)
	c.ShareDir = expandHome(c.ShareDir)
	return c, c.Validate()
}

// soft reads the keys that never stop updown: a wrong value → its default + a warning.
func (c *Config) soft(m map[string]string) {
	warn := func(f string, a ...any) { c.Warnings = append(c.Warnings, fmt.Sprintf(f, a...)) }
	num := func(key string, dst *int, min, max int) {
		v := strings.TrimSpace(m[key])
		if v == "" {
			return
		}
		if n, err := strconv.Atoi(v); err == nil && n >= min && n <= max {
			*dst = n
			return
		}
		warn("%s=%s is not a number from %d to %d — using the default %d", key, v, min, max, *dst)
	}
	num("CHECK_SECONDS", &c.CheckSeconds, 1, 3600)
	num("PAGE_REFRESH_SECONDS", &c.PageRefreshSec, 1, 60)
	num("LOG_LINES", &c.LogLines, 10, 100000)

	if v := strings.TrimSpace(m["CONNECTIONS"]); v != "" {
		var known []string
		for _, n := range list(v) {
			if _, ok := connection.Get(n); ok {
				known = append(known, n)
			} else {
				warn("CONNECTIONS: unknown way %q (known: %s) — left out", n, strings.Join(connection.Names(), ", "))
			}
		}
		if len(known) == 0 {
			warn("CONNECTIONS=%s names no known way — using the default %s", v, connection.DefaultOrder)
			known = list(connection.DefaultOrder)
		}
		c.Connections = known
	}
	if v := strings.TrimSpace(m["PAGE_PARTS"]); v != "" {
		c.PageParts = list(v) // checked against the embedded parts by updown (it knows them)
	}
	if v := strings.TrimSpace(m["CONTROLS_URL"]); v != "" {
		if strings.HasPrefix(v, "http://localhost:") || strings.HasPrefix(v, "http://127.0.0.1:") {
			c.ControlsURL = strings.TrimRight(v, "/")
		} else {
			warn("CONTROLS_URL=%s must be on this PC (http://localhost:PORT) — no buttons on the page", v)
		}
	}
}

// Validate checks every value against the contract.
func (c Config) Validate() error {
	var bad []string
	switch c.StopMode {
	case StopEnter, StopAfterTransfer, StopIdle:
	default:
		bad = append(bad, fmt.Sprintf("STOP_MODE must be enter, after-transfer or idle, got %q", c.StopMode))
	}
	if c.IdleMinutes < 1 {
		bad = append(bad, "IDLE_MINUTES must be 1 or more")
	}
	if c.Port < 0 || c.Port > 65535 {
		bad = append(bad, "PORT must be 0 (auto) or 1-65535")
	}
	if c.HostPort < 1 || c.HostPort > 65535 {
		bad = append(bad, "HOST_PORT must be 1-65535")
	}
	if c.Bind != "auto" && c.Bind != "hotspot" && net.ParseIP(c.Bind).To4() == nil {
		bad = append(bad, fmt.Sprintf("BIND must be auto, hotspot or an IPv4 address, got %q", c.Bind))
	}
	if c.Advertise != "" && net.ParseIP(c.Advertise).To4() == nil {
		bad = append(bad, fmt.Sprintf("ADVERTISE must be an IPv4 address, got %q", c.Advertise))
	}
	if c.MaxFileGB < 1 {
		bad = append(bad, "MAX_FILE_GB must be 1 or more")
	}
	if c.ReceiveDir == "" {
		bad = append(bad, "RECEIVE_DIR is empty and no Downloads folder was found")
	}
	if len(bad) > 0 {
		where := c.Path
		if where == "" {
			where = "defaults"
		}
		return fmt.Errorf("%s: %s", where, strings.Join(bad, "; "))
	}
	return nil
}

// DownloadsDir is the OS's standard Downloads folder: on Linux XDG_DOWNLOAD_DIR from
// ~/.config/user-dirs.dirs, else ~/Downloads; on Windows %USERPROFILE%\Downloads; on
// macOS ~/Downloads.
func DownloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "linux" {
		if m, err := kv.ParseFile(filepath.Join(home, ".config", "user-dirs.dirs")); err == nil {
			if d := strings.ReplaceAll(m["XDG_DOWNLOAD_DIR"], "$HOME", home); d != "" {
				return d
			}
		}
	}
	return filepath.Join(home, "Downloads")
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
