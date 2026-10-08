// Package connection knows the ways this PC can reach the phone (docs/contract.md §2, UPDOWN-61 [2.7]).
// Every way is its own file — hotspot.go, usb.go, bluetooth.go, wifi.go, lan.go — that registers a Type at
// start; a new way = a new file here + its name in updown.conf's CONNECTIONS. Nothing else changes.
// device_connection.go describes the phone itself (the browser that opened the page).
//
// updown.conf decides which ways are watched and in which order (CONNECTIONS) and whether only the phone's own
// network may be used (BIND=hotspot) or any of them (BIND=auto). Container and virtual bridges are never used.
package connection

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// Type is one way to reach the phone.
type Type struct {
	Name  string                             // its CONNECTIONS name, e.g. "usb"
	Label string                             // for the console and the page, e.g. "phone hotspot over USB"
	Via   string                             // what carries it: wifi · usb · bluetooth · lan (the page's icons)
	Phone bool                               // the phone's OWN network (private) — the only ones BIND=hotspot uses
	Rank  int                                // which type an address belongs to: the lowest Rank that matches
	Match func(iface string, ip net.IP) bool // ip is IPv4, never loopback, link-local or a virtual interface
}

var types = map[string]Type{}

// Register adds a way (each type file calls it from init).
func Register(t Type) { types[t.Name] = t }

// Get returns a registered way.
func Get(name string) (Type, bool) { t, ok := types[name]; return t, ok }

// Names lists every registered way, in Rank order.
func Names() []string {
	var out []string
	for _, t := range byRank() {
		out = append(out, t.Name)
	}
	return out
}

// DefaultOrder is CONNECTIONS when updown.conf does not set it: the phone's own networks first, best first.
const DefaultOrder = "hotspot,usb,bluetooth,wifi,lan"

func byRank() []Type {
	out := make([]Type, 0, len(types))
	for _, t := range types {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

// virtual reports interfaces that are never phone-reachable.
func virtual(name string) bool {
	for _, p := range []string{"lo", "docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "tun", "tap", "wg"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// Classify says which way one interface address is (false = never usable).
func Classify(iface string, ip net.IP) (Type, bool) {
	ip4 := ip.To4()
	if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || virtual(iface) {
		return Type{}, false
	}
	for _, t := range byRank() {
		if t.Match(iface, ip4) {
			return t, true
		}
	}
	return Type{}, false
}

// Candidate is one usable IPv4 address of this PC.
type Candidate struct {
	Iface string
	IP    net.IP
	Type  Type
}

// Candidates lists this PC's addresses of the watched ways (order = CONNECTIONS), best first.
func Candidates(order []string) []Candidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if t, ok := Classify(ifc.Name, ipn.IP); ok {
					out = append(out, Candidate{Iface: ifc.Name, IP: ipn.IP.To4(), Type: t})
				}
			}
		}
	}
	return Order(out, order)
}

// Order keeps the candidates whose way is in order (the watched ones), sorted by it.
func Order(cands []Candidate, order []string) []Candidate {
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	var out []Candidate
	for _, c := range cands {
		if _, ok := pos[c.Type.Name]; ok {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return pos[out[i].Type.Name] < pos[out[j].Type.Name] })
	return out
}

// Pick chooses the address for BIND=auto (the first) or BIND=hotspot (the first of the phone's own networks —
// never a foreign Wi-Fi or LAN). None → an error with the reason in plain words.
func Pick(bind string, cands []Candidate) (Candidate, error) {
	for _, c := range cands {
		if bind == "auto" || c.Type.Phone {
			return c, nil
		}
	}
	if bind == "hotspot" {
		return Candidate{}, errors.New("no phone hotspot — connect this PC to the phone's hotspot (USB or Wi-Fi); BIND=hotspot never uses another network")
	}
	return Candidate{}, errors.New("no phone-reachable network — connect this PC to the phone's hotspot (USB or Wi-Fi) or to the same Wi-Fi, or set BIND")
}

func hasPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
