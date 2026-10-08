package connection

import "net"

// lan — any other private network (a cable to a router: 192.168.x.x, 10.x.x.x, 172.16-31.x.x). NOT the
// phone's own network: only BIND=auto uses it.
func init() {
	Register(Type{Name: "lan", Label: "LAN", Via: "lan", Rank: 40,
		Match: func(_ string, ip net.IP) bool { return ip.IsPrivate() }})
}
