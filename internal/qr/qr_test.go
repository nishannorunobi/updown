package qr

import (
	"os/exec"
	"strings"
	"testing"
)

// matrixText renders a code as 0/1 lines, the same way the reference script prints it.
func matrixText(m [][]bool) string {
	var b strings.Builder
	for _, row := range m {
		for _, d := range row {
			if d {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Inputs covering several versions (1 … 10), incl. the real link shape.
var samples = []string{
	"a",
	"HELLO WORLD",
	"http://172.20.10.6:8070/?t=Zm9vYmFyYmF6cXV4LWV4YW1wbGUtdG9rZW4tMzJieXRlcw",
	"http://192.168.1.23:8079/?t=" + strings.Repeat("x", 43),
	strings.Repeat("0123456789", 12),
	strings.Repeat("updown ", 30),
}

// TestMatchesReference: every sample must give exactly the same modules as
// testdata/qr_reference.py (the workspace's backup/qr.py, proven with real phones).
// Skipped when python3 is not available.
func TestMatchesReference(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	for _, s := range samples {
		m, err := Encode(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		script := "import sys; sys.path.insert(0, 'testdata'); import qr_reference as q\n" +
			"m = q.make_qr(sys.argv[1])\n" +
			"print(''.join(''.join('1' if d else '0' for d in r) + '\\n' for r in m), end='')"
		out, err := exec.Command(py, "-c", script, s).Output()
		if err != nil {
			t.Fatalf("reference failed for %q: %v", s, err)
		}
		if got := matrixText(m); got != string(out) {
			t.Errorf("%q (%d chars): Go and reference differ (size %d)", s, len(s), len(m))
		}
	}
}

func TestTooLong(t *testing.T) {
	if _, err := Encode(strings.Repeat("x", 300)); err != ErrTooLong {
		t.Fatalf("want ErrTooLong, got %v", err)
	}
}

// TestSVGMatchesReference: the host page's SVG is byte-identical to qr.py's to_svg.
func TestSVGMatchesReference(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	s := samples[2]
	m, _ := Encode(s)
	script := "import sys; sys.path.insert(0, 'testdata'); import qr_reference as q\n" +
		"print(q.to_svg(q.make_qr(sys.argv[1])), end='')"
	out, err := exec.Command(py, "-c", script, s).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := SVG(m, 4); got != string(out) {
		t.Fatal("SVG differs from the reference")
	}
}
