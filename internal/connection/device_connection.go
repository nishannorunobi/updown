package connection

import "strings"

// device_connection — the PHONE's side: which device opened updown's page. A browser never sends the phone's
// own name, so this is what its User-Agent says, short: "iPhone · Safari".

// Device turns a User-Agent into "iPhone · Safari".
func Device(ua string) string {
	dev := "phone"
	for _, d := range []string{"iPhone", "iPad", "Android", "Windows", "Macintosh", "Linux"} {
		if strings.Contains(ua, d) {
			dev = d
			break
		}
	}
	if dev == "Macintosh" {
		dev = "Mac"
	}
	br := ""
	switch {
	case strings.Contains(ua, "CriOS"), strings.Contains(ua, "Chrome/") && !strings.Contains(ua, "Edg/"):
		br = "Chrome"
	case strings.Contains(ua, "FxiOS"), strings.Contains(ua, "Firefox/"):
		br = "Firefox"
	case strings.Contains(ua, "Edg"):
		br = "Edge"
	case strings.Contains(ua, "Safari/"):
		br = "Safari"
	}
	if br == "" {
		return dev
	}
	return dev + " · " + br
}
