// Command updown — offline file transfer between a PC and a phone (docs/contract.md).
//
//	updown [--config FILE] [--check] [--version] [FILE ...]
//
// Name and version come from release.conf at run time; settings from updown.conf.
// Exit codes: 0 normal stop, 1 error, 2 wrong usage.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/nishannorunobi/updown/internal/config"
	"github.com/nishannorunobi/updown/internal/netaddr"
	"github.com/nishannorunobi/updown/internal/qr"
	"github.com/nishannorunobi/updown/internal/release"
	"github.com/nishannorunobi/updown/internal/server"
	"github.com/nishannorunobi/updown/web"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("updown", flag.ContinueOnError)
	fs.SetOutput(stderr)
	confPath := fs.String("config", "", "settings file (default: updown.conf next to release.conf)")
	check := fs.Bool("check", false, "offline self-test: validate the release and settings, then exit")
	version := fs.Bool("version", false, "print the release and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	say := func(format string, a ...any) { fmt.Fprintf(stdout, "[updown] "+format+"\n", a...) }
	fail := func(format string, a ...any) int { fmt.Fprintf(stderr, "[updown] "+format+"\n", a...); return 1 }

	exe, err := os.Executable()
	if err != nil {
		return fail("cannot locate the program: %v", err)
	}
	wd, _ := os.Getwd()
	relPath, err := release.Find(filepath.Dir(exe), wd)
	if err != nil {
		return fail("%v", err)
	}
	rel, err := release.Load(relPath)
	if err != nil {
		return fail("%v", err)
	}
	if *version {
		fmt.Fprintln(stdout, rel.Release)
		return 0
	}

	optional := *confPath == ""
	if optional {
		*confPath = filepath.Join(filepath.Dir(relPath), "updown.conf")
	}
	cfg, err := config.Load(*confPath, optional)
	if err != nil {
		return fail("%v", err)
	}
	if *check {
		return selfCheck(rel, cfg, stdout, stderr)
	}

	// Files named on the command line must exist.
	var shareFiles []string
	for _, p := range fs.Args() {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			return fail("not a file: %s", p)
		}
		abs, _ := filepath.Abs(p)
		shareFiles = append(shareFiles, abs)
	}

	// Where to listen, and which address the QR code carries.
	listenIP, adIP, how, err := addresses(cfg)
	if err != nil {
		return fail("%v", err)
	}
	ln, err := listen(listenIP, cfg.Port)
	if err != nil {
		return fail("%v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	token, err := server.NewToken()
	if err != nil {
		return fail("cannot make a token: %v", err)
	}
	hostKey, err := server.NewToken()
	if err != nil {
		return fail("cannot make a key: %v", err)
	}
	base := "http://" + net.JoinHostPort(adIP, strconv.Itoa(port))
	// The phone's two links — only in the QR codes, never in the console.
	sendLink, receiveLink := base+"/send?t="+token, base+"/receive?t="+token
	sendQR, err := qr.Encode(sendLink)
	if err != nil {
		return fail("cannot make the QR code: %v", err)
	}
	receiveQR, err := qr.Encode(receiveLink)
	if err != nil {
		return fail("cannot make the QR code: %v", err)
	}
	transferred := make(chan struct{}, 1)
	srv := server.New(server.Options{
		Product: rel.ProductName, Version: rel.Version,
		ReceiveDir: cfg.ReceiveDir, ShareDir: cfg.ShareDir, ShareFiles: shareFiles,
		StopMode: cfg.StopMode, MaxBytes: int64(cfg.MaxFileGB) << 30,
		Token: token, Page: web.Index,
		HostKey: hostKey, HostPage: web.Host,
		SendLink: sendLink, ReceiveLink: receiveLink,
		SendQR: qr.SVG(sendQR, 4), ReceiveQR: qr.SVG(receiveQR, 4),
		Logf: func(f string, a ...any) { say(f, a...) },
		OnTransfer: func() {
			select {
			case transferred <- struct{}{}:
			default:
			}
		},
	})

	// The console prints only the host page's link and the event log; the two QR codes are
	// on the host page (contract §4).
	offered := len(shareFiles)
	if cfg.ShareDir != "" {
		offered = -1
	}
	srv.Log("%s — uploads to: %s — phone sees: %s — stop: %s", rel.Release, cfg.ReceiveDir, offeredText(offered, cfg.ShareDir), stopText(cfg))
	srv.Log("listening on %s (%s)", ln.Addr(), how)
	say("open this page on this PC to show the QR codes: %s/host?k=%s", base, hostKey)
	srv.Log("waiting for the phone — %s", stopHint(cfg))

	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 30 * time.Second}
	served := make(chan error, 1)
	go func() { served <- hs.Serve(ln) }()

	// Stopping (contract §8).
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	enter := make(chan struct{})
	if cfg.StopMode == config.StopEnter {
		go func() {
			if _, err := bufio.NewReader(stdin).ReadString('\n'); err == nil {
				close(enter)
			} else {
				srv.Log("no terminal to press Enter in — stop with Ctrl+C, or it stops after %d idle minutes", cfg.IdleMinutes)
			}
		}()
	}
	idle := time.Duration(cfg.IdleMinutes) * time.Minute
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	reason := ""
	for reason == "" {
		select {
		case <-ctx.Done():
			reason = "interrupted"
		case <-enter:
			reason = "Enter pressed"
		case <-transferred:
			if cfg.StopMode == config.StopAfterTransfer {
				time.Sleep(time.Second) // let the last response reach the phone
				reason = "transfer finished"
			}
		case err := <-served:
			return fail("server stopped: %v", err)
		case <-tick.C:
			if time.Since(srv.LastActivity()) > idle {
				reason = fmt.Sprintf("idle for %d minutes", cfg.IdleMinutes)
			}
		}
	}
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	if err := hs.Shutdown(sctx); err != nil {
		hs.Close() // cut open transfers; their part files are removed by the handlers
	}
	srv.Log("stopped: %s", reason)
	return 0
}

// addresses decides the listen address and the address the QR carries.
func addresses(cfg config.Config) (listenIP, adIP, how string, err error) {
	cands := netaddr.Candidates()
	switch {
	case cfg.Bind == "auto":
		if len(cands) == 0 {
			return "", "", "", errors.New("no phone-reachable network — connect this PC to the phone's hotspot (USB or Wi-Fi) or to the same Wi-Fi, or set BIND")
		}
		c := cands[0]
		listenIP, adIP, how = c.IP.String(), c.IP.String(), c.Kind.String()+" on "+c.Iface
	default:
		listenIP, adIP, how = cfg.Bind, cfg.Bind, "BIND"
		if cfg.Bind == "0.0.0.0" {
			how = "all interfaces"
			adIP = ""
			if len(cands) > 0 {
				adIP = cands[0].IP.String()
			}
		}
	}
	if cfg.Advertise != "" {
		adIP, how = cfg.Advertise, how+", QR shows ADVERTISE"
	}
	if adIP == "" {
		return "", "", "", errors.New("BIND=0.0.0.0 needs ADVERTISE (no phone-reachable address found here)")
	}
	return listenIP, adIP, how, nil
}

// listen opens PORT, or with PORT=0 the first free port from 8070 to 8079.
func listen(ip string, port int) (net.Listener, error) {
	if port != 0 {
		ln, err := net.Listen("tcp4", net.JoinHostPort(ip, strconv.Itoa(port)))
		if err != nil {
			return nil, fmt.Errorf("cannot listen on %s:%d: %v", ip, port, err)
		}
		return ln, nil
	}
	for p := 8070; p <= 8079; p++ {
		if ln, err := net.Listen("tcp4", net.JoinHostPort(ip, strconv.Itoa(p))); err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port from 8070 to 8079 on %s", ip)
}

func offeredText(n int, dir string) string {
	if n < 0 {
		return "files in " + dir
	}
	return strconv.Itoa(n) + " file(s)"
}

func stopText(cfg config.Config) string {
	if cfg.StopMode == config.StopIdle {
		return fmt.Sprintf("idle %d min", cfg.IdleMinutes)
	}
	return cfg.StopMode
}

func stopHint(cfg config.Config) string {
	switch cfg.StopMode {
	case config.StopAfterTransfer:
		return "stops after the first finished transfer (Ctrl+C stops now)"
	case config.StopIdle:
		return fmt.Sprintf("stops after %d idle minutes (Ctrl+C stops now)", cfg.IdleMinutes)
	}
	return "press Enter to stop"
}

// selfCheck is --check (docs/contract.md §3): needs no network, no write access, any user.
func selfCheck(rel release.Info, cfg config.Config, stdout, stderr io.Writer) int {
	fmt.Fprintf(stdout, "%s (version %s)\n", rel.Release, rel.Version)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(stderr, "[updown] check failed: cannot listen on 127.0.0.1:", err)
		return 1
	}
	ln.Close()
	if _, err := qr.Encode("http://255.255.255.255:65535/?t=" + string(make([]byte, 43))); err != nil {
		fmt.Fprintln(stderr, "[updown] check failed: QR encoder:", err)
		return 1
	}
	fmt.Fprintf(stdout, "[updown] check ok — release.conf: %s · settings: %s · receive: %s\n",
		rel.Path, orDefault(cfg.Path), cfg.ReceiveDir)
	return 0
}

func orDefault(p string) string {
	if p == "" {
		return "defaults"
	}
	return p
}
