package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nishannorunobi/updown/web"
)

const (
	tok     = "test-token"
	hostKey = "host-key"
)

func newTest(t *testing.T) (*Server, string, string) {
	t.Helper()
	recv, share := t.TempDir(), t.TempDir()
	s := New(Options{Product: "updown", Version: "0.1.0", ReceiveDir: recv, ShareDir: share,
		StopMode: "enter", MaxBytes: 1 << 20, Token: tok, Page: web.Index,
		HostKey: hostKey, HostPage: web.Host, HostParts: web.Parts,
		SendLink: "http://172.20.10.6:8070/send?t=" + tok, ReceiveLink: "http://172.20.10.6:8070/receive?t=" + tok,
		SendQR: "<svg>send</svg>", ReceiveQR: "<svg>receive</svg>"})
	return s, recv, share
}

// do sends one request from client address ip ("" = the phone).
func do(s *Server, method, path, ip string, body []byte) *httptest.ResponseRecorder {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if ip == "" {
		ip = "172.20.10.2"
	}
	req.RemoteAddr = ip + ":50000"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func q(path string) string { return path + "?t=" + tok }

func TestTokenRequired(t *testing.T) {
	s, _, _ := newTest(t)
	for _, p := range []string{"/send", "/receive", "/api/info", "/send?t=wrong", "/api/files?t="} {
		if w := do(s, "GET", p, "", nil); w.Code != http.StatusForbidden || w.Body.Len() != 0 {
			t.Errorf("%s: got %d %q, want 403 with empty body", p, w.Code, w.Body.String())
		}
	}
	for _, p := range []string{"/send", "/receive"} {
		if w := do(s, "GET", q(p), "", nil); w.Code != http.StatusOK {
			t.Fatalf("%s with the token: got %d", p, w.Code)
		}
	}
	// Only the two screens exist: "/" is not a page.
	if w := do(s, "GET", q("/"), "", nil); w.Code != http.StatusNotFound {
		t.Fatalf("/: got %d, want 404", w.Code)
	}
}

func TestOnePhonePerSession(t *testing.T) {
	s, _, _ := newTest(t)
	if w := do(s, "GET", q("/api/info"), "172.20.10.2", nil); w.Code != http.StatusOK {
		t.Fatalf("first phone: %d", w.Code)
	}
	if w := do(s, "GET", q("/api/info"), "172.20.10.3", nil); w.Code != http.StatusForbidden {
		t.Fatalf("second phone: got %d, want 403", w.Code)
	}
}

func TestPageIsSelfContained(t *testing.T) {
	s, _, _ := newTest(t)
	w := do(s, "GET", q("/send"), "", nil)
	if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing CSP / no-store headers")
	}
	if m := regexp.MustCompile(`https?://`).FindString(w.Body.String()); m != "" {
		t.Errorf("page references an external URL (%s)", m)
	}
}

func TestUploadChecksumAndNoPartFile(t *testing.T) {
	s, recv, _ := newTest(t)
	data := bytes.Repeat([]byte("updown "), 50000) // ~350 KB
	w := do(s, "PUT", q("/api/upload/"+url.PathEscape("holiday video.mov")), "", data)
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	var r struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	json.Unmarshal(w.Body.Bytes(), &r)
	sum := sha256.Sum256(data)
	if r.Name != "holiday video.mov" || r.Size != int64(len(data)) || r.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("response %+v", r)
	}
	disk, _ := os.ReadFile(filepath.Join(recv, r.Name))
	if !bytes.Equal(disk, data) {
		t.Fatal("file on disk differs")
	}
	ents, _ := os.ReadDir(recv)
	if len(ents) != 1 {
		t.Fatalf("receive folder has %d entries (a part file left?)", len(ents))
	}
}

func TestUploadNeverOverwrites(t *testing.T) {
	s, recv, _ := newTest(t)
	for i, want := range []string{"a.txt", "a (1).txt", "a (2).txt"} {
		w := do(s, "PUT", q("/api/upload/a.txt"), "", []byte{byte('0' + i)})
		if !strings.Contains(w.Body.String(), `"name":"`+want+`"`) {
			t.Fatalf("upload %d: got %s, want name %s", i, w.Body.String(), want)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(recv, "a.txt")); string(b) != "0" {
		t.Fatal("the first file was overwritten")
	}
}

func TestUploadTooLarge(t *testing.T) {
	s, recv, _ := newTest(t)
	w := do(s, "PUT", q("/api/upload/big.bin"), "", make([]byte, 2<<20))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", w.Code)
	}
	if ents, _ := os.ReadDir(recv); len(ents) != 0 {
		t.Fatal("something was written for a refused upload")
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"photo.jpg":        "photo.jpg",
		"../../etc/passwd": "passwd",
		`C:\Users\x\a.txt`: "a.txt",
		".hidden":          "_hidden",
		"..":               "__",
		"a:b*c?.txt":       "a_b_c_.txt",
		"":                 "upload",
		"tab\there.txt":    "tab_here.txt",
	} {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadAndRange(t *testing.T) {
	s, _, share := newTest(t)
	data := []byte("0123456789abcdef")
	os.WriteFile(filepath.Join(share, "notes.txt"), data, 0o644)
	os.WriteFile(filepath.Join(share, "x"+partSuffix), []byte("hidden"), 0o644)

	w := do(s, "GET", q("/api/files"), "", nil)
	if !strings.Contains(w.Body.String(), `"name":"notes.txt"`) || strings.Contains(w.Body.String(), partSuffix) {
		t.Fatalf("list: %s", w.Body.String())
	}
	w = do(s, "GET", q("/api/files/notes.txt"), "", nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %q", w.Code, w.Body.String())
	}
	req := httptest.NewRequest("GET", q("/api/files/notes.txt"), nil)
	req.RemoteAddr = "172.20.10.2:50000"
	req.Header.Set("Range", "bytes=4-7")
	rw := httptest.NewRecorder()
	s.ServeHTTP(rw, req)
	if rw.Code != http.StatusPartialContent || rw.Body.String() != "4567" {
		t.Fatalf("range: %d %q", rw.Code, rw.Body.String())
	}
	if w := do(s, "GET", q("/api/files/missing.txt"), "", nil); w.Code != http.StatusNotFound {
		t.Fatalf("missing file: %d", w.Code)
	}
}

func TestOnlyOfferedFilesVisible(t *testing.T) {
	s, recv, _ := newTest(t)
	os.WriteFile(filepath.Join(recv, "private.txt"), []byte("x"), 0o644)
	if w := do(s, "GET", q("/api/files/private.txt"), "", nil); w.Code != http.StatusNotFound {
		t.Fatalf("a received (not offered) file was downloadable: %d", w.Code)
	}
}

func TestHostPage(t *testing.T) {
	s, _, _ := newTest(t)
	for _, p := range []string{"/host", "/host?k=wrong", "/host?t=" + tok, "/host/api/status", "/host/qr/send.svg?t=" + tok} {
		if w := do(s, "GET", p, "127.0.0.1", nil); w.Code != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403 (the phone token must not open the host page)", p, w.Code)
		}
	}
	w := do(s, "GET", "/host?k="+hostKey, "172.28.0.1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("host page: %d", w.Code)
	}
	for _, part := range []string{"connection.html", "qr_codes.html", "transfer_log.html"} { // the blocks are part files (UPDOWN-44)
		if !strings.Contains(w.Body.String(), `data-part="`+part+`"`) {
			t.Errorf("host page has no placeholder for %s", part)
		}
	}
	if q := do(s, "GET", "/host/part/qr_codes.html?k="+hostKey, "172.28.0.1", nil).Body.String(); !strings.Contains(q, "qr/send.svg") || !strings.Contains(q, "qr/receive.svg") {
		t.Errorf("the QR part does not show both QR codes")
	}
	if m := regexp.MustCompile(`https?://`).FindString(w.Body.String()); m != "" {
		t.Errorf("host page references an external URL (%s)", m)
	}
	for name, want := range map[string]string{"send": "<svg>send</svg>", "receive": "<svg>receive</svg>"} {
		w := do(s, "GET", "/host/qr/"+name+".svg?k="+hostKey, "172.28.0.1", nil)
		if w.Header().Get("Content-Type") != "image/svg+xml" || w.Body.String() != want {
			t.Errorf("qr/%s.svg: %q", name, w.Body.String())
		}
	}
	// The host page never takes the phone's session.
	if w := do(s, "GET", q("/api/info"), "172.20.10.2", nil); w.Code != http.StatusOK {
		t.Fatalf("phone after host page: %d", w.Code)
	}
	s.Log("hello")
	w = do(s, "GET", "/host/api/status?k="+hostKey+"&after=0", "172.28.0.1", nil)
	body := w.Body.String()
	if !strings.Contains(body, `"phone":"172.20.10.2"`) || !strings.Contains(body, "hello") || !strings.Contains(body, `"send":"http://172.20.10.6:8070/send?t=`) || !strings.Contains(body, `"receive":"http://172.20.10.6:8070/receive?t=`) {
		t.Fatalf("status: %s", body)
	}
}

func TestReceiveListNewestFirstNoHidden(t *testing.T) {
	s, _, share := newTest(t)
	for i, n := range []string{"old.pdf", "new.jpg", ".hidden"} {
		p := filepath.Join(share, n)
		os.WriteFile(p, []byte(n), 0o644)
		mt := time.Now().Add(time.Duration(i-5) * time.Hour)
		os.Chtimes(p, mt, mt)
	}
	body := do(s, "GET", q("/api/files"), "", nil).Body.String()
	if strings.Contains(body, ".hidden") {
		t.Error("hidden file listed")
	}
	if i, j := strings.Index(body, "new.jpg"), strings.Index(body, "old.pdf"); i < 0 || j < 0 || i > j {
		t.Errorf("not newest first: %s", body)
	}
}

func TestHostParts(t *testing.T) {
	s, _, _ := newTest(t)
	for _, name := range []string{"connection.html", "qr_codes.html", "transfer_log.html"} {
		w := do(s, "GET", "/host/part/"+name+"?k="+hostKey, "172.28.0.1", nil)
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("part %s: %d %s", name, w.Code, w.Header().Get("Content-Type"))
		}
		if m := regexp.MustCompile(`(?i)(src|href)=["']?(https?:)?//`).FindString(w.Body.String()); m != "" {
			t.Errorf("part %s references an external URL (%s)", name, m)
		}
		if w := do(s, "GET", "/host/part/"+name, "172.28.0.1", nil); w.Code != 403 {
			t.Errorf("part %s without the host key: %d, want 403", name, w.Code)
		}
	}
	for _, bad := range []string{"nope.html", "..%2Fhost.html", "CONNECTION.html", "connection.js"} {
		if w := do(s, "GET", "/host/part/"+bad+"?k="+hostKey, "172.28.0.1", nil); w.Code != 404 {
			t.Errorf("part %q: %d, want 404", bad, w.Code)
		}
	}
}

// [2.6] UPDOWN-52: the PC's page on its own loopback listener — no key, but only for this PC's Host names.
func TestHostHandlerLocalhostOnly(t *testing.T) {
	s, _, _ := newTest(t)
	h := s.HostHandler()
	get := func(path, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, host := range []string{"localhost:8070", "127.0.0.1:8070", "[::1]:8070", "localhost"} {
		if w := get("/", host); w.Code != 200 || !strings.Contains(w.Body.String(), `data-part="connection.html"`) {
			t.Errorf("Host %s: / gave %d, want the page", host, w.Code)
		}
	}
	for _, host := range []string{"evil.example:8070", "172.20.10.3:8070", "attacker.localhost.example"} {
		if w := get("/host/api/status", host); w.Code != http.StatusForbidden {
			t.Errorf("Host %s: %d, want 403 (no other site may read the page — DNS rebinding)", host, w.Code)
		}
	}
	if w := get("/host/part/qr_codes.html", "localhost:8070"); w.Code != 200 {
		t.Errorf("a part without a key on localhost: %d", w.Code)
	}
	// the phone's door never serves the PC's page when no host key is set
	s2 := New(Options{Token: tok, Page: web.Index, HostPage: web.Host, HostParts: web.Parts})
	if w := do(s2, "GET", "/host?t="+tok, "172.20.10.2", nil); w.Code != http.StatusForbidden {
		t.Errorf("/host on the phone's door: %d, want 403", w.Code)
	}
}

func TestSetLinkRotatesAndCloses(t *testing.T) {
	s, _, _ := newTest(t)
	if w := do(s, "GET", "/api/info?t="+tok, "", nil); w.Code != 200 {
		t.Fatalf("old token before the rotation: %d", w.Code)
	}
	s.SetLink(Link{Token: "new-token", SendLink: "http://172.20.10.3:8070/send?t=new-token"})
	if w := do(s, "GET", "/api/info?t="+tok, "", nil); w.Code != http.StatusForbidden {
		t.Errorf("old token after a new link: %d, want 403", w.Code)
	}
	if w := do(s, "GET", "/api/info?t=new-token", "172.20.10.9", nil); w.Code != 200 {
		t.Errorf("new token (the session is free again after a new link): %d", w.Code)
	}
	s.SetLink(Link{}) // the door closes
	if w := do(s, "GET", "/api/info?t=", "172.20.10.9", nil); w.Code != http.StatusForbidden {
		t.Errorf("empty token on a closed door: %d, want 403", w.Code)
	}
}

func TestStatusConnectionAndBrowser(t *testing.T) {
	s, _, _ := newTest(t)
	s.SetConnection(Connection{State: "hotspot", Via: "wifi", PC: "172.20.10.3", Iface: "wlo1"})
	r := httptest.NewRequest("GET", "/api/info?t="+tok, nil)
	r.RemoteAddr = "172.20.10.1:5000"
	r.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1")
	s.ServeHTTP(httptest.NewRecorder(), r)
	w := do(s, "GET", "/host/api/status?k="+hostKey+"&after=0", "172.28.0.1", nil)
	var st struct {
		Connection Connection `json:"connection"`
		Phone      string     `json:"phone"`
		Agent      string     `json:"phone_agent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Connection.Via != "wifi" || st.Connection.State != "hotspot" || st.Phone != "172.20.10.1" || st.Agent != "iPhone · Safari" {
		t.Errorf("status: %+v phone=%q agent=%q", st.Connection, st.Phone, st.Agent)
	}
}

// [2.7]: the page's layout comes from updown.conf; the control points run only for this page or a program
// on this PC — never for another web site (CSRF).
func TestHostLayoutAndControls(t *testing.T) {
	checks := 0
	s := New(Options{Token: tok, Page: web.Index, HostPage: web.Host, HostParts: web.Parts,
		PageParts: []string{"qr_codes", "connection"}, PageRefreshSec: 2, ControlsURL: "http://localhost:8069",
		OnCheck: func() (Connection, error) { checks++; return Connection{State: "hotspot", Type: "usb"}, nil }})
	h := s.HostHandler()
	call := func(method, path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Host = "localhost:8070"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var lay struct {
		Parts       []string `json:"parts"`
		Refresh     int      `json:"refresh_seconds"`
		ControlsURL string   `json:"controls_url"`
	}
	json.Unmarshal(call("GET", "/host/api/page", "").Body.Bytes(), &lay)
	if strings.Join(lay.Parts, ",") != "qr_codes,connection" || lay.Refresh != 2 || lay.ControlsURL != "http://localhost:8069" {
		t.Errorf("layout: %+v", lay)
	}
	if w := call("POST", "/host/api/check", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"usb"`) {
		t.Errorf("check from a program on this PC: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/host/api/check", "http://localhost:8070"); w.Code != 200 {
		t.Errorf("check from the page itself: %d", w.Code)
	}
	if w := call("POST", "/host/api/check", "https://evil.example"); w.Code != http.StatusForbidden {
		t.Errorf("check from another web site: %d, want 403", w.Code)
	}
	if checks != 2 {
		t.Errorf("OnCheck ran %d times, want 2", checks)
	}
	if w := call("POST", "/host/api/new-link", ""); w.Code != http.StatusNotImplemented {
		t.Errorf("new-link without a handler: %d, want 501", w.Code)
	}
	if w := call("GET", "/host/api/check", ""); w.Code == 200 {
		t.Errorf("GET must not run a control point")
	}
}
