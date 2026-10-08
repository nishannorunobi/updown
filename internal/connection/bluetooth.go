package connection

import "net"

// bluetooth — the phone's hotspot over Bluetooth (Bluetooth tethering, interface bnep…). Private, but slow:
// fine for small files. (Bluetooth file push is not possible with an iPhone — only AirDrop does that.)
func init() {
	Register(Type{Name: "bluetooth", Label: "phone hotspot over Bluetooth", Via: "bluetooth", Phone: true, Rank: 0,
		Match: func(iface string, _ net.IP) bool { return hasPrefix(iface, "bnep") }})
}
