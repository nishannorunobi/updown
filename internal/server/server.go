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
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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
	SendLink    string // QR "Send files"    → the phone's upload screen (/send)
	ReceiveLink string // QR "Receive files" → the phone's download screen (/receive)
	SendQR      string // the two QR codes as SVG
	ReceiveQR   string
}

// logLine is one event, kept for the host page.
type logLine struct {
	N    int       `json:"n"`
	Time time.Time `json:"time"`
	Line string    `json:"line"`
}

const logKeep = 500

// Server handles every request. Create it with New.
type Server struct {
	o        Options
	mu       sync.Mutex
	owner    string // client address that owns the session ("" until the first valid request)
	lastSeen atomic.Int64
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
	if len(s.logs) > logKeep {
		s.logs = s.logs[len(s.logs)-logKeep:]
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
	s := &Server{o: o, mux: http.NewServeMux(), hostMux: http.NewServeMux()}
	s.lastSeen.Store(time.Now().UnixNano())
	s.mux.HandleFunc("GET /send", s.page)
	s.mux.HandleFunc("GET /receive", s.page)
	s.mux.HandleFunc("GET /api/info", s.info)
	s.mux.HandleFunc("GET /api/files", s.list)
	s.mux.HandleFunc("GET /api/files/{name}", s.download)
	s.mux.HandleFunc("PUT /api/upload/{name}", s.upload)
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "not found") })
	s.hostMux.HandleFunc("GET /host", s.hostPage)
	s.hostMux.HandleFunc("GET /host/qr/send.svg", s.hostQR(func() string { return s.o.SendQR }))
	s.hostMux.HandleFunc("GET /host/qr/receive.svg", s.hostQR(func() string { return s.o.ReceiveQR }))
	s.hostMux.HandleFunc("GET /host/api/status", s.hostStatus)
	s.hostMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "not found") })
	return s
}

// LastActivity is the time of the last accepted request (for STOP_MODE=idle).
func (s *Server) LastActivity() time.Time { return time.Unix(0, s.lastSeen.Load()) }

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
	got := r.URL.Query().Get("t")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.o.Token)) != 1 {
		s.Log("refused %s %s from %s: no/wrong token", r.Method, r.URL.Path, clientIP(r))
		w.WriteHeader(http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	s.mu.Lock()
	if s.owner == "" {
		s.owner = ip
		s.Log("phone connected: %s", ip)
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
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'")
	w.Write(s.o.HostPage)
}

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
	owner := s.owner
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
		"product": s.o.Product, "version": s.o.Version, "send": s.o.SendLink, "receive": s.o.ReceiveLink,
		"phone": owner, "stop_mode": s.o.StopMode, "log": lines,
	})
}
