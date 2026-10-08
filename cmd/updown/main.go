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
	"github.com/nishannorunobi/updown/internal/phonenet"
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
	address := fs.Bool("address", false, "print the address the settings would listen on, then exit (1 = none, with the reason)")
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
	if *address { // the address the phone's door would use now (BIND), or why there is none
		c := choose(cfg)
		if !c.Open() {
			return fail("%s", c.Reason)
		}
		fmt.Fprintln(stdout, c.Listen)
		return 0
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

	// The PC's page: always on http://localhost:HOST_PORT — this PC only, an address that never changes
	// ([2.6], UPDOWN-52). The phone's door opens and closes below, as the phone's network comes and goes.
	hostLn, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.HostPort)))
	if err != nil {
		return fail("cannot open the PC's page on 127.0.0.1:%d (HOST_PORT): %v", cfg.HostPort, err)
	}
	transferred := make(chan struct{}, 1)
	srv := server.New(server.Options{
		Product: rel.ProductName, Version: rel.Version,
		ReceiveDir: cfg.ReceiveDir, ShareDir: cfg.ShareDir, ShareFiles: shareFiles,
		StopMode: cfg.StopMode, MaxBytes: int64(cfg.MaxFileGB) << 30,
		Page: web.Index, HostPage: web.Host, HostParts: web.Parts,
		Logf: func(f string, a ...any) { say(f, a...) },
		OnTransfer: func() {
			select {
			case transferred <- struct{}{}:
			default:
			}
		},
	})
	offered := len(shareFiles)
	if cfg.ShareDir != "" {
		offered = -1
	}
	srv.Log("%s — uploads to: %s — phone sees: %s — stop: %s", rel.Release, cfg.ReceiveDir, offeredText(offered, cfg.ShareDir), stopText(cfg))
	hs := &http.Server{Handler: srv.HostHandler(), ReadHeaderTimeout: 30 * time.Second}
	served := make(chan error, 1)
	go func() { served <- hs.Serve(hostLn) }()
	say("open this page on this PC to show the QR codes: http://localhost:%d", cfg.HostPort)

	// The phone's door: a listener on the address BIND allows, a NEW token + QR codes each time it opens.
	var door *http.Server
	closeDoor := func() {
		if door != nil {
			dctx, dcancel := context.WithTimeout(context.Background(), 2*time.Second)
			if door.Shutdown(dctx) != nil {
				door.Close() // cut open transfers; their part files are removed by the handlers
			}
			dcancel()
			door = nil
		}
		srv.SetLink(server.Link{})
	}
	openDoor := func(c phonenet.Choice) error {
		ln, err := listen(c.Listen, cfg.Port)
		if err != nil {
			return err
		}
		token, err := server.NewToken()
		if err != nil {
			ln.Close()
			return err
		}
		base := "http://" + net.JoinHostPort(c.QR, strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
		send, receive := base+"/send?t="+token, base+"/receive?t="+token // only in the QR codes, never in the console
		sq, err1 := qr.Encode(send)
		rq, err2 := qr.Encode(receive)
		if err1 != nil || err2 != nil {
			ln.Close()
			return errors.New("cannot make the QR codes")
		}
		srv.SetLink(server.Link{Token: token, SendLink: send, ReceiveLink: receive, SendQR: qr.SVG(sq, 4), ReceiveQR: qr.SVG(rq, 4)})
		door = &http.Server{Handler: srv, ReadHeaderTimeout: 30 * time.Second}
		go door.Serve(ln)
		srv.Log("listening on %s (%s) — new QR codes", ln.Addr(), c.How)
		return nil
	}
	watch := &phonenet.Watcher{Choose: func() phonenet.Choice { return choose(cfg) }}
	follow := func() {
		now, changed := watch.Check()
		if !changed {
			return
		}
		closeDoor()
		if now.Open() {
			if err := openDoor(now); err != nil {
				srv.Log("cannot open the phone's door on %s: %v", now.Listen, err)
				srv.SetConnection(server.Connection{State: "none", Reason: err.Error(), Since: time.Now()})
				return
			}
			st, via, iface := "lan", "lan", ""
			if now.Cand != nil {
				via, iface = now.Cand.Via(), now.Cand.Iface
				if now.Cand.Kind == netaddr.Hotspot {
					st = "hotspot"
				}
			}
			srv.SetConnection(server.Connection{State: st, Via: via, PC: now.Listen, Iface: iface, Since: time.Now()})
			srv.Log("waiting for the phone — %s", stopHint(cfg))
			return
		}
		srv.SetConnection(server.Connection{State: "none", Reason: now.Reason, Since: time.Now()})
		srv.Log("no phone network — %s", now.Reason)
	}
	follow()

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
	tick := time.NewTicker(3 * time.Second) // also how often the phone's network is checked
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
			return fail("the PC's page stopped: %v", err)
		case <-tick.C:
			follow()
			if time.Since(srv.LastActivity()) > idle {
				reason = fmt.Sprintf("idle for %d minutes", cfg.IdleMinutes)
			}
		}
	}
	closeDoor()
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	hs.Shutdown(sctx)
	srv.Log("stopped: %s", reason)
	return 0
}

// choose decides where the phone's door goes now (BIND, ADVERTISE) — or why there is none.
func choose(cfg config.Config) phonenet.Choice {
	cands := netaddr.Candidates()
	var c phonenet.Choice
	switch cfg.Bind {
	case "auto", "hotspot":
		cand, err := netaddr.Pick(cfg.Bind, cands)
		if err != nil {
			return phonenet.Choice{Reason: err.Error()}
		}
		ip := cand.IP.String()
		c = phonenet.Choice{Cand: &cand, Listen: ip, QR: ip, How: cand.Kind.String() + " on " + cand.Iface}
	default:
		c = phonenet.Choice{Listen: cfg.Bind, QR: cfg.Bind, How: "BIND"}
		if cfg.Bind == "0.0.0.0" {
			c.How, c.QR = "all interfaces", ""
			if len(cands) > 0 {
				c.QR = cands[0].IP.String()
			}
		}
	}
	if cfg.Advertise != "" {
		c.QR, c.How = cfg.Advertise, c.How+", QR shows ADVERTISE"
	}
	if c.QR == "" {
		return phonenet.Choice{Reason: "BIND=0.0.0.0 needs ADVERTISE (no phone-reachable address found here)"}
	}
	return c
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
