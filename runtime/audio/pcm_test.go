package audio

import "testing"

func TestPCMControlRejectsInvalidBindingsAndAllocatesNothing(t *testing.T) {
	bank := []uint32{1<<16 | 1, 0x78616d2a, 0x2a646f6d, 20, 28, 0, 0, 0, 0}
	for _, tc := range []struct {
		bank           []uint32
		music, samples []uint16
	}{
		{nil, nil, nil}, {bank, nil, []uint16{0}}, {bank, []uint16{1}, []uint16{0}}, {bank, []uint16{0}, []uint16{1}},
	} {
		var p PCMController
		if p.Init(tc.bank, tc.music, tc.samples) {
			t.Fatal("invalid binding accepted")
		}
	}
	var p PCMController
	if !p.Init(bank, []uint16{0}, []uint16{0}) || !p.PCMStats().Ready {
		t.Fatal("valid binding rejected")
	}
	if p.Init(bank, []uint16{0}, []uint16{0}) {
		t.Fatal("live controller rebound")
	}
	if got := testing.AllocsPerRun(1000, func() {
		p.Music(0, true)
		p.Sample(0, 255, 160)
		p.Music(-1, false)
		p.Sample(-1, 255, 160)
		p.PCMStats()
	}); got != 0 {
		t.Fatalf("audio commands allocated %g objects", got)
	}
}
