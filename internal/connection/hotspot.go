package connection

import "net"

// hotspot — the phone's Personal Hotspot over Wi-Fi: this PC joined the phone's own Wi-Fi network, known by
// the address range phones hand out (iPhone 172.20.10.x, Android 192.168.43.x). Private: the phone's network.
func init() {
	Register(Type{Name: "hotspot", Label: "phone hotspot over Wi-Fi", Via: "wifi", Phone: true, Rank: 20, Match: phoneRange})
}

func phoneRange(_ string, ip net.IP) bool {
	return ip[0] == 172 && ip[1] == 20 && ip[2] == 10 || // iPhone hotspot
		ip[0] == 192 && ip[1] == 168 && ip[2] == 43 // Android hotspot
}
