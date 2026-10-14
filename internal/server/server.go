// Package server is the updown HTTP server (docs/contract.md §5–§6): a token on every
// request, one phone per session, the embedded page, the list of offered files, downloads
// with Range, and uploads streamed to disk with a SHA-256.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nishannorunobi/updown/internal/connection"
)

// Options configure one server run.
type Options struct {
	Product, Version string
	ReceiveDir       string   // uploads land here
	ShareDir         string   // the Receive screen lists its files (not recursive); "" = none
	ShareFiles       []string // files named on the command line
	StopMode         string
	MaxBytes         int64
	Token            string
	Page             []byte
	Logf             func(format string, args ...any) // one line per event (the console)
	OnTransfer       func()                           // after each completed upload / download

	// The PC's host page (contract §4): its own key, never shown to the phone.
	HostKey     string
	HostPage    []byte
	HostParts   fs.FS  // the host page's part files (parts/NAME.html), served at /host/part/NAME.html
	SendLink    string // QR "Send files"    → the phone's upload screen (/send)
	ReceiveLink string // QR "Receive files" → the phone's download screen (/receive)
	SendQR      string // the two QR codes as SVG
	ReceiveQR   string

	// The PC's page and its two control points ([2.7]) — updown.conf: PAGE_PARTS, PAGE_REFRESH_SECONDS,
	// CONTROLS_URL, LOG_LINES. The buttons themselves live in updown-agent (actions.conf); they call these.
	PageParts      []string                   // the page's blocks, top to bottom
	PageRefreshSec int                        // how often the page asks for news
	ControlsURL    string                     // where the page finds its buttons ("" = none)
	LogKeep        int                        // log lines kept for the page (0 = 500)
	OnCheck        func() (Connection, error) // POST /host/api/check: look at the phone's network now
	OnNewLink      func() (Connection, error) // POST /host/api/new-link: a new token + new QR codes now
}

// Link is the phone's current way in: a token and the two QR codes that carry it (contract §5).
// updown hands out a new Link whenever it opens the phone's door (a new network = a new link).
type Link struct {
	Token, SendLink, ReceiveLink, SendQR, ReceiveQR string
}

// Connection is what the host page shows about the phone's network ([2.6], UPDOWN-52).
type Connection struct {
	State  string    `json:"state"`  // none · hotspot · lan
	Type   string    `json:"type"`   // the way (internal/connection): hotspot · usb · bluetooth · wifi · lan · "" (none)
	Via    string    `json:"via"`    // wifi · usb · bluetooth · lan · "" (none)
	PC     string    `json:"pc"`     // this PC's address on it ("" = none)
	Iface  string    `json:"iface"`  // e.g. wlo1, enx…, bnep0
	Reason string    `json:"reason"` // why there is none, in plain words
	Since  time.Time `json:"since"`
}

// logLine is one event, kept for the host page.
type logLine struct {
	N    int       `json:"n"`
	Time time.Time `json:"time"`
	Line string    `json:"line"`
}

// Server handles every request. Create it with New.
type Server struct {
	o        Options
	mu       sync.Mutex
	owner    string // client address that owns the session ("" until the first valid request)
	agent    string // the owner's browser, short ("iPhone · Safari")
	link     Link   // the phone's current link (token + QR); empty Token = the phone's door is closed
	conn     Connection
	lastSeen atomic.Int64
	active   atomic.Int64 // downloads + uploads running now: they count as activity ([2.8.1] UPDOWN-86)
	mux      *http.ServeMux
	hostMux  *http.ServeMux
	logMu    sync.Mutex
	logs     []logLine
	logN     int
}

// Log records one event: printed by Logf (the console) and kept for the host page.
func (s *Server) Log(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	s.logMu.Lock()
	s.logN++
	s.logs = append(s.logs, logLine{N: s.logN, Time: time.Now(), Line: line})
	if len(s.logs) > s.o.LogKeep {
		s.logs = s.logs[len(s.logs)-s.o.LogKeep:]
	}
	s.logMu.Unlock()
	s.o.Logf("%s", line)
}

// NewToken returns 32 random bytes, base64url without padding (contract §5).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// New builds the server.
func New(o Options) *Server {
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	if o.OnTransfer == nil {
		o.OnTransfer = func() {}
	}
	if o.LogKeep <= 0 {
		o.LogKeep = 500
	}
	if o.PageRefreshSec <= 0 {
		o.PageRefreshSec = 1
	}
	s := &Server{o: o, mux: http.NewServeMux(), hostMux: http.NewServeMux(),
		link: Link{o.Token, o.SendLink, o.ReceiveLink, o.SendQR, o.ReceiveQR}}
	s.lastSeen.Store(time.Now().UnixNano())
	s.mux.HandleFunc("GET /send", s.page)
	s.mux.HandleFunc("GET /receive", s.page)
	s.mux.HandleFunc("GET /api/info", s.info)
	s.mux.HandleFunc("GET /api/files", s.list)
	s.mux.HandleFunc("GET /api/files/{name}", s.download)
	s.mux.HandleFunc("PUT /api/upload/{name}", s.upload)
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "not found") })
	s.hostMux.HandleFunc("GET /host", s.hostPage)
	s.hostMux.HandleFunc("GET /{$}", s.hostPage) // the PC's page at http://localhost:8070 ([2.6])
	s.hostMux.HandleFunc("GET /host/qr/send.svg", s.hostQR(func() string { return s.Link().SendQR }))
	s.hostMux.HandleFunc("GET /host/qr/receive.svg", s.hostQR(func() string { return s.Link().ReceiveQR }))
	s.hostMux.HandleFunc("GET /host/api/status", s.hostStatus)
	s.hostMux.HandleFunc("GET /host/part/{name}", s.hostPart)
	s.hostMux.HandleFunc("GET /host/api/page", s.hostLayout)
	s.hostMux.HandleFunc("POST /host/api/check", s.hostControl(func() func() (Connection, error) { return s.o.OnCheck }))
	s.hostMux.HandleFunc("POST /host/api/new-link", s.hostControl(func() func() (Connection, error) { return s.o.OnNewLink }))
	s.hostMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "not found") })
	return s
}

// Link returns the phone's current link.
func (s *Server) Link() Link { s.mu.Lock(); defer s.mu.Unlock(); return s.link }

// SetLink hands out a new phone link (a new door, a new network) — the old token stops working and the
// next phone that uses the new one owns the session. An empty Link closes the phone's door.
func (s *Server) SetLink(l Link) {
	s.mu.Lock()
	s.link, s.owner, s.agent = l, "", ""
	s.mu.Unlock()
}

// SetConnection records the phone's network for the host page.
func (s *Server) SetConnection(c Connection) { s.mu.Lock(); s.conn = c; s.mu.Unlock() }

// Connection returns the phone's network as the host page sees it.
func (s *Server) Connection() Connection { s.mu.Lock(); defer s.mu.Unlock(); return s.conn }

// HostHandler serves the PC's page on its own loopback listener (http://localhost:8070): no key — it can
// only be reached from this PC — but the Host header must name this PC, so no other web site can read it
// (DNS rebinding). A POST (the control points) must come from this page itself or from a program on this PC
// (no Origin) — never from another web site (CSRF). It never claims the phone's session and never counts as activity.
func (s *Server) HostHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); r.Method != http.MethodGet && r.Method != http.MethodHead && o != "" && o != "http://"+r.Host {
			fail(w, http.StatusForbidden, "only this page may do that")
			return
		}
		s.hostMux.ServeHTTP(w, r)
	})
}

// LastActivity is the time of the last accepted request or finished transfer (for STOP_MODE=idle) — and "now"
// while a download or upload is running: a transfer of hours is ONE request, and must never look idle ([2.8.1]).
func (s *Server) LastActivity() time.Time {
	if s.active.Load() > 0 {
		return time.Now()
	}
	return time.Unix(0, s.lastSeen.Load())
}

// Transfers is the number of downloads and uploads running now.
func (s *Server) Transfers() int { return int(s.active.Load()) }

// transfer marks one download/upload as running; call the returned func when it ends (its end counts as activity).
func (s *Server) transfer() func() {
	s.active.Add(1)
	return func() {
		s.lastSeen.Store(time.Now().UnixNano())
		s.active.Add(-1)
	}
}

// ServeHTTP checks the token and the session, then routes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// The PC's host page: its own key; it never claims the phone session and never
	// counts as activity (its polling must not keep an idle server alive).
	if r.URL.Path == "/host" || strings.HasPrefix(r.URL.Path, "/host/") {
		k := r.URL.Query().Get("k")
		if s.o.HostKey == "" || subtle.ConstantTimeCompare([]byte(k), []byte(s.o.HostKey)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		s.hostMux.ServeHTTP(w, r)
		return
	}
	// What a browser asks for by itself (an icon, robots.txt) carries no token: a quiet 404 — not a "refused"
	// line that looks like an attack ([2.8], a false alarm in the owner's iPhone test).
	if browserAsk(r.URL.Path) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	got := r.URL.Query().Get("t")
	cur := s.Link().Token
	if cur == "" || subtle.ConstantTimeCompare([]byte(got), []byte(cur)) != 1 {
		s.Log("refused %s %s from %s: no/wrong token", r.Method, r.URL.Path, clientIP(r))
		w.WriteHeader(http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	s.mu.Lock()
	if s.owner == "" {
		s.owner, s.agent = ip, connection.Device(r.UserAgent())
		s.Log("phone connected: %s (%s)", ip, s.agent)
	}
	owner := s.owner
	s.mu.Unlock()
	if ip != owner {
		s.Log("refused %s %s from %s: session belongs to %s", r.Method, r.URL.Path, ip, owner)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	s.lastSeen.Store(time.Now().UnixNano())
	s.mux.ServeHTTP(w, r)
}

// browserAsk reports the paths a browser requests on its own: /favicon.ico, /apple-touch-icon*.png, /robots.txt.
func browserAsk(p string) bool {
	return p == "/favicon.ico" || p == "/robots.txt" || strings.HasPrefix(p, "/apple-touch-icon") && strings.HasSuffix(p, ".png")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, reason string) {
	writeJSON(w, code, map[string]string{"error": reason})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'")
	w.Write(s.o.Page)
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"product":        s.o.Product,
		"version":        s.o.Version,
		"receive_dir":    filepath.Base(s.o.ReceiveDir),
		"share_dir":      filepath.Base(s.o.ShareDir),
		"stop_mode":      s.o.StopMode,
		"max_file_bytes": s.o.MaxBytes,
	})
}

// fileEntry is one offered file.
type fileEntry struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	path     string
}

// offered lists what the Receive screen shows: files named at start, then SHARE_DIR's
// regular files (not recursive; by default the PC's Downloads folder), newest first. Read at
// request time, so new files show up. Hidden files and upload part files are never listed.
// A name appears once (the first wins).
func (s *Server) offered() []fileEntry {
	seen := map[string]bool{}
	var out []fileEntry
	add := func(p string) {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() || seen[st.Name()] {
			return
		}
		seen[st.Name()] = true
		out = append(out, fileEntry{Name: st.Name(), Size: st.Size(), Modified: st.ModTime().UTC(), path: p})
	}
	for _, p := range s.o.ShareFiles {
		add(p)
	}
	named := len(out)
	if s.o.ShareDir != "" {
		if ents, err := os.ReadDir(s.o.ShareDir); err == nil {
			for _, e := range ents {
				if n := e.Name(); !strings.HasPrefix(n, ".") && !strings.HasSuffix(n, partSuffix) {
					add(filepath.Join(s.o.ShareDir, n))
				}
			}
		}
	}
	dir := out[named:]
	sort.SliceStable(dir, func(i, j int) bool { return dir[i].Modified.After(dir[j].Modified) })
	return out
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	files := s.offered()
	if files == nil {
		files = []fileEntry{}
	}
	writeJSON(w, http.StatusOK, files)
}

// countingWriter counts the body bytes a download actually sent.
type countingWriter struct {
	http.ResponseWriter
	n    int64
	code int
}

func (c *countingWriter) WriteHeader(code int) { c.code = code; c.ResponseWriter.WriteHeader(code) }
func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var entry *fileEntry
	for _, f := range s.offered() {
		if f.Name == name {
			entry = &f
			break
		}
	}
	if entry == nil {
		fail(w, http.StatusNotFound, "unknown file")
		return
	}
	f, err := os.Open(entry.path)
	if err != nil {
		fail(w, http.StatusNotFound, "unknown file")
		return
	}
	defer f.Close()
	defer s.transfer()()
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": entry.Name}))
	start := time.Now()
	cw := &countingWriter{ResponseWriter: w, code: http.StatusOK}
	http.ServeContent(cw, r, entry.Name, entry.Modified, f)
	if cw.code == http.StatusOK && cw.n == entry.Size && r.Method == http.MethodGet {
		s.Log("download finished: %s (%s, %.1fs)", entry.Name, human(entry.Size), time.Since(start).Seconds())
		s.o.OnTransfer()
	} else if cw.code == http.StatusPartialContent {
		s.Log("download part: %s (%s sent)", entry.Name, human(cw.n))
	}
}

const partSuffix = ".updown-part"

// SafeName applies the contract's upload name rules (§6).
func SafeName(raw string) string {
	name := raw
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`/\:*?"<>|`, r) {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	name = b.String()
	for i := 0; i < len(name) && name[i] == '.'; i++ {
		name = name[:i] + "_" + name[i+1:]
	}
	if strings.TrimSpace(name) == "" {
		name = "upload"
	}
	if len(name) > 200 { // stay under every filesystem's name limit
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		name = name[:200-len(ext)] + ext
	}
	return name
}

// freeName returns name, or "name (1).ext", "name (2).ext", … — never an existing file.
func freeName(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	cand := name
	for i := 1; ; i++ {
		if _, err := os.Lstat(filepath.Join(dir, cand)); errors.Is(err, os.ErrNotExist) {
			return cand
		}
		cand = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	name := SafeName(r.PathValue("name")) // PathValue is already unescaped
	if r.ContentLength < 0 {
		fail(w, http.StatusBadRequest, "Content-Length required")
		return
	}
	if r.ContentLength > s.o.MaxBytes {
		s.Log("refused upload %s: %s is over the limit", name, human(r.ContentLength))
		fail(w, http.StatusRequestEntityTooLarge, "file too large")
		return
	}
	if err := os.MkdirAll(s.o.ReceiveDir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "receive folder not writable")
		return
	}
	part, err := os.CreateTemp(s.o.ReceiveDir, name+".*"+partSuffix)
	if err != nil {
		fail(w, http.StatusInternalServerError, "receive folder not writable")
		return
	}
	partPath := part.Name()
	done := false
	defer func() {
		if !done {
			part.Close()
			os.Remove(partPath)
		}
	}()

	defer s.transfer()()
	s.Log("upload started: %s (%s)", name, human(r.ContentLength))
	start := time.Now()
	h := sha256.New()
	body := http.MaxBytesReader(w, r.Body, r.ContentLength)
	n, err := io.CopyBuffer(io.MultiWriter(part, h), body, make([]byte, 1<<20))
	if err == nil && n != r.ContentLength {
		err = io.ErrUnexpectedEOF
	}
	if err == nil {
		err = part.Sync()
	}
	if cerr := part.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if errors.Is(err, syscall.ENOSPC) {
			s.Log("upload failed: %s — disk full", name)
			fail(w, http.StatusInsufficientStorage, "disk full")
			return
		}
		s.Log("upload failed: %s — %v (part file removed)", name, err)
		fail(w, http.StatusBadRequest, "upload interrupted")
		return
	}

	s.mu.Lock() // one rename at a time, so two uploads never pick the same free name
	final := freeName(s.o.ReceiveDir, name)
	err = os.Rename(partPath, filepath.Join(s.o.ReceiveDir, final))
	s.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not save the file")
		return
	}
	done = true
	sum := hex.EncodeToString(h.Sum(nil))
	s.Log("upload finished: %s (%s, %.1fs, sha256 %s)", final, human(n), time.Since(start).Seconds(), sum)
	writeJSON(w, http.StatusCreated, map[string]any{"name": final, "size": n, "sha256": sum})
	s.o.OnTransfer()
}

func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (s *Server) hostPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page talks to updown and — only when CONTROLS_URL is set (always on this PC, checked by config) — to
	// updown-agent's buttons; nothing else. data: images = the empty favicon.
	w.Header().Set("Content-Security-Policy", strings.TrimSpace("default-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' "+s.o.ControlsURL))
	w.Write(s.o.HostPage)
}

// hostPart serves one part file of the host page (parts/NAME.html) — only plain names, only what is embedded.
func (s *Server) hostPart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.o.HostParts == nil || !partName.MatchString(name) {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	b, err := fs.ReadFile(s.o.HostParts, "parts/"+name)
	if err != nil {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

var partName = regexp.MustCompile(`^[a-z0-9_]+\.html$`)

func (s *Server) hostQR(svg func() string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(svg()))
	}
}

// hostStatus: the phone link, who owns the session, and the log lines after ?after=N.
func (s *Server) hostStatus(w http.ResponseWriter, r *http.Request) {
	after := 0
	fmt.Sscanf(r.URL.Query().Get("after"), "%d", &after)
	s.mu.Lock()
	owner, agent, link, conn := s.owner, s.agent, s.link, s.conn
	s.mu.Unlock()
	s.logMu.Lock()
	lines := []logLine{}
	for _, l := range s.logs {
		if l.N > after {
			lines = append(lines, l)
		}
	}
	s.logMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"product": s.o.Product, "version": s.o.Version, "send": link.SendLink, "receive": link.ReceiveLink,
		"connection": conn, "phone_agent": agent,
		"phone": owner, "stop_mode": s.o.StopMode, "log": lines,
	})
}

// hostLayout: how the PC's page is built — its blocks (PAGE_PARTS), how often it asks for news, where its buttons are.
func (s *Server) hostLayout(w http.ResponseWriter, r *http.Request) {
	parts := s.o.PageParts
	if parts == nil {
		parts = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"parts": parts, "refresh_seconds": s.o.PageRefreshSec, "controls_url": s.o.ControlsURL})
}

// hostControl runs one control point and answers with the connection after it.
func (s *Server) hostControl(f func() func() (Connection, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		do := f()
		if do == nil {
			fail(w, http.StatusNotImplemented, "not available")
			return
		}
		c, err := do()
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "connection": c})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": c})
	}
}
