package connection

import (
	"net"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		iface, ip string
		want      string // "" = never usable
	}{
		{"enx9e924f6a2218", "172.20.10.6", "usb"}, // iPhone over USB: the cable wins over the address range
		{"wlo1", "172.20.10.3", "hotspot"},        // iPhone hotspot over Wi-Fi
		{"wlan0", "192.168.43.12", "hotspot"},     // Android hotspot
		{"bnep0", "192.168.44.2", "bluetooth"},    // Bluetooth tethering
		{"wlo1", "192.168.1.23", "wifi"},
		{"eth0", "192.168.1.40", "lan"},
		{"eth0", "10.0.0.5", "lan"},
		{"docker0", "172.17.0.1", ""},
		{"br-3f2a", "172.28.0.1", ""},
		{"lo", "127.0.0.1", ""},
		{"eth0", "8.8.8.8", ""}, // public: not a local network
		{"eth0", "169.254.1.1", ""},
	}
	for _, c := range cases {
		got, ok := Classify(c.iface, net.ParseIP(c.ip))
		if ok != (c.want != "") || got.Name != c.want {
			t.Errorf("%s %s: got %q %v, want %q", c.iface, c.ip, got.Name, ok, c.want)
		}
	}
}

// Every way named in DefaultOrder has its own file, and every file is in DefaultOrder (one list, nothing missing).
func TestEveryTypeHasAFile(t *testing.T) {
	if got, want := strings.Join(sortedNames(DefaultOrder), ","), strings.Join(sortedNames(strings.Join(Names(), ",")), ","); got != want {
		t.Errorf("DefaultOrder %q ≠ registered types %q", got, want)
	}
	for _, n := range Names() {
		if ty, _ := Get(n); ty.Label == "" || ty.Via == "" || ty.Match == nil {
			t.Errorf("type %s is incomplete: %+v", n, ty)
		}
	}
}

func sortedNames(list string) []string {
	out := strings.Split(list, ",")
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func cand(iface, ip string) Candidate {
	t, _ := Classify(iface, net.ParseIP(ip))
	return Candidate{Iface: iface, IP: net.ParseIP(ip).To4(), Type: t}
}

func TestPick(t *testing.T) {
	hot, usb, wifi, lan := cand("wlo1", "172.20.10.3"), cand("enx1", "172.20.10.6"), cand("wlo1", "172.31.43.152"), cand("eth0", "192.168.1.5")
	all := strings.Split(DefaultOrder, ",")
	cases := []struct {
		bind  string
		order []string
		cands []Candidate
		want  string // "" = error
	}{
		{"auto", all, []Candidate{wifi, hot}, "172.20.10.3"},
		{"auto", all, []Candidate{lan, wifi}, "172.31.43.152"},
		{"hotspot", all, []Candidate{wifi, hot}, "172.20.10.3"},
		{"hotspot", all, []Candidate{wifi, lan}, ""}, // a foreign Wi-Fi is never used
		{"hotspot", all, nil, ""},
		{"auto", all, nil, ""},
		{"hotspot", []string{"usb", "hotspot"}, []Candidate{hot, usb}, "172.20.10.6"}, // CONNECTIONS order wins
		{"hotspot", []string{"usb"}, []Candidate{hot}, ""},                            // a way not listed is not watched
		{"auto", []string{"lan"}, []Candidate{wifi, lan}, "192.168.1.5"},
	}
	for i, c := range cases {
		got, err := Pick(c.bind, Order(c.cands, c.order), "")
		if c.want == "" {
			if err == nil {
				t.Errorf("%d: Pick(%s) = %v, want an error", i, c.bind, got.IP)
			}
			continue
		}
		if err != nil || got.IP.String() != c.want {
			t.Errorf("%d: Pick(%s) = %v, %v; want %s", i, c.bind, got.IP, err, c.want)
		}
	}
}

func TestVia(t *testing.T) {
	for addr, want := range map[[2]string]string{{"wlo1", "172.20.10.3"}: "wifi", {"wlo1", "192.168.1.2"}: "wifi",
		{"enx4663b6422c6f", "172.20.10.2"}: "usb", {"usb0", "10.1.1.2"}: "usb", {"bnep0", "192.168.44.2"}: "bluetooth", {"eth0", "10.0.0.2"}: "lan"} {
		if got := cand(addr[0], addr[1]).Type.Via; got != want {
			t.Errorf("Via(%s) = %s, want %s", addr, got, want)
		}
	}
}

func TestDevice(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1": "iPhone · Safari",
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36":                                                    "Android · Chrome",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) AppleWebKit/605.1.15 CriOS/120.0 Mobile/15E148 Safari/604.1":                                    "iPhone · Chrome",
		"": "phone",
	} {
		if got := Device(ua); got != want {
			t.Errorf("Device(%q) = %q, want %q", ua, got, want)
		}
	}
}

// [2.8]: the door keeps a working address while it is allowed; it moves only when that address is gone.
func TestPickKeepsTheDoor(t *testing.T) {
	hot, usb, wifi := cand("wlo1", "172.20.10.3"), cand("enx1", "172.20.10.6"), cand("wlo1", "192.168.1.9")
	all := strings.Split(DefaultOrder, ",")
	cases := []struct {
		bind, prefer string
		cands        []Candidate
		want         string
	}{
		{"hotspot", "172.20.10.6", []Candidate{hot, usb}, "172.20.10.6"},  // the Wi-Fi hotspot appears → stay on USB
		{"hotspot", "172.20.10.6", []Candidate{hot}, "172.20.10.3"},       // USB gone → move
		{"hotspot", "192.168.1.9", []Candidate{wifi, usb}, "172.20.10.6"}, // a foreign Wi-Fi is never kept under hotspot
		{"auto", "192.168.1.9", []Candidate{hot, wifi}, "192.168.1.9"},
	}
	for i, c := range cases {
		got, err := Pick(c.bind, Order(c.cands, all), c.prefer)
		if err != nil || got.IP.String() != c.want {
			t.Errorf("%d: got %v %v, want %s", i, got.IP, err, c.want)
		}
	}
}
