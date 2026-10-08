package phonenet

import (
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	seq := []Choice{
		{Reason: "no phone hotspot"},               // start: none → changed (first call)
		{Reason: "no phone hotspot (still)"},       // same: no door → not changed
		{Listen: "172.20.10.3", QR: "172.20.10.3"}, // the hotspot appears → changed
		{Listen: "172.20.10.3", QR: "172.20.10.3"}, // the same → not changed
		{Listen: "172.20.10.2", QR: "172.20.10.2"}, // a new address → changed
		{Reason: "no phone hotspot"},               // gone → changed
	}
	want := []bool{true, false, true, false, true, true}
	i := 0
	w := &Watcher{Choose: func() Choice { c := seq[i]; return c }}
	for ; i < len(seq); i++ {
		now, changed := w.Check()
		if changed != want[i] || now.Open() != (seq[i].Listen != "") {
			t.Errorf("step %d: changed=%v open=%v, want changed=%v", i, changed, now.Open(), want[i])
		}
	}
}

func TestRelink(t *testing.T) {
	t0 := time.Now()
	r := &Relink{Grace: time.Minute}
	r.Remember("172.20.10.6", 8070, "tok", t0)
	if p, tok, ok := r.Reuse("172.20.10.6", t0.Add(6*time.Second)); !ok || p != 8070 || tok != "tok" {
		t.Errorf("same address 6 s later: %d %q %v, want the same link", p, tok, ok)
	}
	if _, _, ok := r.Reuse("172.20.10.3", t0.Add(6*time.Second)); ok {
		t.Error("another address must get a new link")
	}
	if _, _, ok := r.Reuse("172.20.10.6", t0.Add(2*time.Minute)); ok {
		t.Error("after the grace time: a new link")
	}
	r.Forget()
	if _, _, ok := r.Reuse("172.20.10.6", t0.Add(time.Second)); ok {
		t.Error("after Forget (New link): a new link")
	}
	r0 := &Relink{}
	r0.Remember("172.20.10.6", 8070, "tok", t0)
	if _, _, ok := r0.Reuse("172.20.10.6", t0); ok {
		t.Error("RELINK_GRACE_SECONDS=0: always a new link")
	}
}
