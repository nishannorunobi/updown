package release

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadSourceTree(t *testing.T) {
	p := write(t, t.TempDir(), "release.conf", "PRODUCT_NAME=updown\nVERSION=0.1.0\n")
	in, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if in.Target != "dev" || in.Release != "updown-0.1.0-dev" {
		t.Errorf("got target %q release %q", in.Target, in.Release)
	}
}

func TestLoadPackage(t *testing.T) {
	p := write(t, t.TempDir(), "release.conf",
		"PRODUCT_NAME=updown\nVERSION=1.2.3\nTARGET=linux-x64\nRELEASE=updown-1.2.3-linux-x64\nBUILD=bin/updown\n")
	in, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if in.Release != "updown-1.2.3-linux-x64" || in.Build != "bin/updown" {
		t.Errorf("got %+v", in)
	}
}

func TestLoadRequiresNameAndVersion(t *testing.T) {
	p := write(t, t.TempDir(), "release.conf", "PRODUCT_NAME=updown\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want an error without VERSION")
	}
}

func TestFindPrefersPackageLayout(t *testing.T) {
	pkg := t.TempDir()
	bin := filepath.Join(pkg, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	want := write(t, pkg, "release.conf", "PRODUCT_NAME=a\nVERSION=1\n")
	write(t, t.TempDir(), "release.conf", "PRODUCT_NAME=b\nVERSION=2\n")
	got, err := Find(bin, t.TempDir())
	if err != nil || got != want {
		t.Errorf("Find = %q, %v; want %q", got, err, want)
	}
}
