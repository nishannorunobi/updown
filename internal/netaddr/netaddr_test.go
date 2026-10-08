package netaddr

import (
	"net"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		iface, ip string
		kind      Kind
		ok        bool
	}{
		{"enx9e924f6a2218", "172.20.10.6", Hotspot, true}, // iPhone over USB
		{"wlo1", "172.20.10.3", Hotspot, true},            // iPhone hotspot over Wi-Fi
		{"wlo1", "192.168.1.23", WiFi, true},
		{"eth0", "192.168.1.40", LAN, true},
		{"eth0", "10.0.0.5", LAN, true},
		{"docker0", "172.17.0.1", 0, false},
		{"br-3f2a", "172.28.0.1", 0, false},
		{"lo", "127.0.0.1", 0, false},
		{"eth0", "8.8.8.8", 0, false}, // public: not a local network
		{"eth0", "169.254.1.1", 0, false},
	}
	for _, c := range cases {
		k, ok := Classify(c.iface, net.ParseIP(c.ip))
		if ok != c.ok || (ok && k != c.kind) {
			t.Errorf("%s %s: got %v %v, want %v %v", c.iface, c.ip, k, ok, c.kind, c.ok)
		}
	}
}

func TestPick(t *testing.T) {
	hot := Candidate{Iface: "wlo1", IP: net.ParseIP("172.20.10.3").To4(), Kind: Hotspot}
	wifi := Candidate{Iface: "wlo1", IP: net.ParseIP("172.31.43.152").To4(), Kind: WiFi}
	lan := Candidate{Iface: "eth0", IP: net.ParseIP("192.168.1.5").To4(), Kind: LAN}
	cases := []struct {
		bind  string
		cands []Candidate
		want  string // "" = error
	}{
		{"auto", []Candidate{hot, wifi}, "172.20.10.3"},
		{"auto", []Candidate{wifi, lan}, "172.31.43.152"},
		{"hotspot", []Candidate{hot, wifi}, "172.20.10.3"},
		{"hotspot", []Candidate{wifi, lan}, ""}, // a foreign Wi-Fi is never used
		{"hotspot", nil, ""},
		{"auto", nil, ""},
	}
	for _, c := range cases {
		got, err := Pick(c.bind, c.cands)
		if c.want == "" {
			if err == nil {
				t.Errorf("Pick(%s, %v) = %v, want an error", c.bind, c.cands, got.IP)
			}
			continue
		}
		if err != nil || got.IP.String() != c.want {
			t.Errorf("Pick(%s, …) = %v, %v; want %s", c.bind, got.IP, err, c.want)
		}
	}
}

func TestVia(t *testing.T) {
	for iface, want := range map[string]string{"wlo1": "wifi", "wlan0": "wifi", "enx4663b6422c6f": "usb", "usb0": "usb", "bnep0": "bluetooth", "eth0": "lan"} {
		if got := (Candidate{Iface: iface}).Via(); got != want {
			t.Errorf("Via(%s) = %s, want %s", iface, got, want)
		}
	}
	if k, ok := Classify("bnep0", net.ParseIP("192.168.44.2")); !ok || k != Hotspot {
		t.Errorf("bnep0 should be a phone hotspot (Bluetooth tethering), got %v %v", k, ok)
	}
}
