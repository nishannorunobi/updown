// Package netaddr finds the address a phone can reach this PC on (docs/contract.md §2):
// BIND=auto — the phone's hotspot / USB tether first, then Wi-Fi, then any other private LAN;
// BIND=hotspot — ONLY the phone's hotspot / USB tether (never a foreign Wi-Fi or LAN).
// Container and virtual bridges are never used.
package netaddr

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// Kind ranks a candidate: lower is preferred.
type Kind int

const (
	Hotspot Kind = iota // phone hotspot or USB tether
	WiFi                // wireless card
	LAN                 // other private network
)

func (k Kind) String() string { return [...]string{"phone hotspot", "Wi-Fi", "LAN"}[k] }

// Candidate is one usable IPv4 address.
type Candidate struct {
	Iface string
	IP    net.IP
	Kind  Kind
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

// Classify ranks one interface address (exported for tests).
func Classify(iface string, ip net.IP) (Kind, bool) {
	ip4 := ip.To4()
	if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || virtual(iface) {
		return 0, false
	}
	switch {
	case ip4[0] == 172 && ip4[1] == 20 && ip4[2] == 10, // iPhone hotspot
		ip4[0] == 192 && ip4[1] == 168 && ip4[2] == 43, // Android hotspot
		strings.HasPrefix(iface, "enx"), strings.HasPrefix(iface, "usb"), strings.HasPrefix(iface, "rndis"),
		strings.HasPrefix(iface, "bnep"): // Bluetooth tethering (the phone's hotspot over Bluetooth)
		return Hotspot, true
	case strings.HasPrefix(iface, "wl"):
		return WiFi, true
	case ip4.IsPrivate():
		return LAN, true
	}
	return 0, false
}

// Candidates lists this machine's phone-reachable IPv4 addresses, best first.
func Candidates() []Candidate {
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
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if k, ok := Classify(ifc.Name, ipn.IP); ok {
				out = append(out, Candidate{Iface: ifc.Name, IP: ipn.IP.To4(), Kind: k})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// Pick chooses the address for BIND=auto or BIND=hotspot from the candidates (best first).
// hotspot never falls back to Wi-Fi or a LAN: no phone hotspot → an error with the reason.
func Pick(bind string, cands []Candidate) (Candidate, error) {
	for _, c := range cands {
		if bind == "auto" || c.Kind == Hotspot {
			return c, nil
		}
	}
	if bind == "hotspot" {
		return Candidate{}, errors.New("no phone hotspot — connect this PC to the phone's hotspot (USB or Wi-Fi); BIND=hotspot never uses another network")
	}
	return Candidate{}, errors.New("no phone-reachable network — connect this PC to the phone's hotspot (USB or Wi-Fi) or to the same Wi-Fi, or set BIND")
}

// Via says how this PC reaches the phone's network, for the host page: wifi · usb · bluetooth · lan.
func (c Candidate) Via() string {
	switch {
	case strings.HasPrefix(c.Iface, "bnep"):
		return "bluetooth"
	case strings.HasPrefix(c.Iface, "enx"), strings.HasPrefix(c.Iface, "usb"), strings.HasPrefix(c.Iface, "rndis"):
		return "usb"
	case strings.HasPrefix(c.Iface, "wl"):
		return "wifi"
	}
	return "lan"
}
