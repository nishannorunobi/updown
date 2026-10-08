package connection

import "net"

// wifi — any other Wi-Fi this PC is on (home, office, café). NOT the phone's own network: other people's
// devices share it, so only BIND=auto uses it (for a Wi-Fi you trust).
func init() {
	Register(Type{Name: "wifi", Label: "Wi-Fi", Via: "wifi", Rank: 30,
		Match: func(iface string, _ net.IP) bool { return hasPrefix(iface, "wl") }})
}
