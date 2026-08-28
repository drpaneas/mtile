package audio

import "testing"

func TestPICODampenUsesCapturedOnePoleFilters(t *testing.T) {
	tests := []struct {
		level uint8
		pole  int
		want  []int32
	}{
		{1, sfxDampenOnePole, []int32{61, 24, 9, 4}},
		{2, sfxDampenTwoPole, []int32{32, 22, 15, 10}},
	}
	for _, test := range tests {
		var state sfxFilterState
		state.configure(SFXFilter(test.level*72), false, 0)
		if test.level == 1 && test.pole != 1611 || test.level == 2 && test.pole != 2781 {
			t.Fatalf("dampen %d target pole = %d", test.level, test.pole)
		}
		for index, input := range []int32{100, 0, 0, 0} {
			if got := state.process(input); got != test.want[index] {
				t.Fatalf("dampen %d sample %d = %d, want %d", test.level, index, got, test.want[index])
			}
		}
	}
}

func TestPICOFilterCheckpointRestoresDampenAndReverbState(t *testing.T) {
	var state sfxFilterState
	state.configure(SFXFilter(24+2*72), true, -37)
	for index := 0; index < 400; index++ {
		state.process(int32(index%31 - 15))
	}
	checkpoint := state.checkpoint()
	want := make([]int32, 700)
	for index := range want {
		want[index] = state.process(0)
	}
	state.restore(checkpoint)
	for index, expected := range want {
		if got := state.process(0); got != expected {
			t.Fatalf("restored filter sample %d = %d, want %d", index, got, expected)
		}
	}
}
