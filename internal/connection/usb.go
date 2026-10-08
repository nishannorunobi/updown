package connection

import "net"

// usb — the phone's hotspot over a USB cable (USB tethering): the cable shows up as a network card
// (enx…, usb0, rndis0). Private: a cable straight to the phone.
func init() {
	Register(Type{Name: "usb", Label: "phone hotspot over USB", Via: "usb", Phone: true, Rank: 10,
		Match: func(iface string, _ net.IP) bool { return hasPrefix(iface, "enx", "usb", "rndis") }})
}
