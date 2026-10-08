// Package config reads updown.conf — the user's settings (docs/contract.md §2). Every key
// is optional; a missing or empty key takes its default.
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
	Port        int    // PORT — 0 = first free from 8070 to 8079
	Bind        string // BIND — "auto", "hotspot" or one IPv4 address
	Advertise   string // ADVERTISE — address for the QR link when it differs from BIND
	MaxFileGB   int    // MAX_FILE_GB
	Path        string // the file read ("" = defaults only)
}

// Default is the configuration with no updown.conf at all.
func Default() Config {
	return Config{
		ReceiveDir:  DownloadsDir(),
		ShareDir:    DownloadsDir(),
		StopMode:    StopEnter,
		IdleMinutes: 15,
		Bind:        "auto",
		MaxFileGB:   10,
	}
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
	for key, dst := range map[string]*int{"IDLE_MINUTES": &c.IdleMinutes, "PORT": &c.Port, "MAX_FILE_GB": &c.MaxFileGB} {
		if err := num(key, dst); err != nil {
			return c, err
		}
	}
	c.ReceiveDir = expandHome(c.ReceiveDir)
	c.ShareDir = expandHome(c.ShareDir)
	return c, c.Validate()
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
