package phonenet

import "testing"

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
