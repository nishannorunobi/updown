// Package phonenet follows this PC's phone-reachable network for updown ([2.6], UPDOWN-52): updown asks
// Check every few seconds; when the chosen address changes — or disappears, or comes back — the phone's
// door must be reopened (a new address = a new link) or closed. The choice itself is connection.Pick
// (BIND=hotspot · auto) or a fixed BIND address; this package only remembers it and reports changes.
package phonenet

import (
	"github.com/nishannorunobi/updown/internal/connection"
)

// Choice is one decision: where the phone's door should be, or why there is none.
type Choice struct {
	Cand   *connection.Candidate // nil = no usable network
	Listen string                // the address to listen on
	QR     string                // the address the QR codes carry (ADVERTISE, else Listen)
	How    string                // e.g. "phone hotspot on wlo1"
	Reason string                // when Cand == nil and Listen == "": why
}

// Open reports whether the choice has a door.
func (c Choice) Open() bool { return c.Listen != "" }

// Watcher remembers the last choice.
type Watcher struct {
	Choose func() Choice
	last   Choice
	seen   bool
}

// Check asks Choose again; changed is true on the first call and whenever the door's address appears,
// moves or disappears (a different reason alone is not a change).
func (w *Watcher) Check() (now Choice, changed bool) {
	now = w.Choose()
	changed = !w.seen || now.Listen != w.last.Listen || now.QR != w.last.QR
	w.last, w.seen = now, true
	return now, changed
}

// Last is the latest choice.
func (w *Watcher) Last() Choice { return w.last }

// Reset forgets the last choice: the next Check reports a change, so the door is reopened with a new link.
func (w *Watcher) Reset() { w.seen = false }
