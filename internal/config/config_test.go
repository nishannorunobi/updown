package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingOptionalFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.conf"), true)
	if err != nil {
		t.Fatal(err)
	}
	if c.StopMode != StopEnter || c.IdleMinutes != 15 || c.Bind != "auto" || c.MaxFileGB != 10 || c.Port != 0 ||
		c.ShareDir == "" || c.ShareDir != c.ReceiveDir {
		t.Errorf("defaults wrong: %+v", c)
	}
}

func TestMissingRequiredFileIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "none.conf"), false); err == nil {
		t.Fatal("want an error for a --config file that does not exist")
	}
}

func TestShippedFileIsValid(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "packaging", "updown.conf"), false)
	if err != nil {
		t.Fatal(err)
	}
	if c.StopMode != StopEnter || c.ReceiveDir == "" {
		t.Errorf("got %+v", c)
	}
}

func TestValuesAndValidation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "updown.conf")
	os.WriteFile(p, []byte("RECEIVE_DIR=/tmp/in\nSTOP_MODE=idle\nIDLE_MINUTES=5\nPORT=8071\nBIND=172.20.10.6\nMAX_FILE_GB=2\n"), 0o644)
	c, err := Load(p, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReceiveDir != "/tmp/in" || c.StopMode != StopIdle || c.IdleMinutes != 5 || c.Port != 8071 || c.Bind != "172.20.10.6" || c.MaxFileGB != 2 {
		t.Errorf("got %+v", c)
	}

	os.WriteFile(p, []byte("STOP_MODE=never\nPORT=70000\nBIND=phone\n"), 0o644)
	_, err = Load(p, false)
	if err == nil {
		t.Fatal("want validation errors")
	}
	for _, want := range []string{"STOP_MODE", "PORT", "BIND"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestBindHotspot(t *testing.T) {
	p := filepath.Join(t.TempDir(), "updown.conf")
	os.WriteFile(p, []byte("BIND=hotspot\n"), 0o644)
	c, err := Load(p, false)
	if err != nil || c.Bind != "hotspot" {
		t.Fatalf("BIND=hotspot: got %q, %v", c.Bind, err)
	}
}
