package stereocapture

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
)

func TestProfileProducesDistinctActiveChannels(t *testing.T) {
	var mixer audio.Mixer
	if !Start(&mixer) {
		t.Fatal("Start failed")
	}
	left := make([]int8, 4096)
	right := make([]int8, len(left))
	mixer.MixStereo(left, right)

	leftActive := false
	rightActive := false
	different := false
	for index := range left {
		leftActive = leftActive || left[index] != 0
		rightActive = rightActive || right[index] != 0
		different = different || left[index] != right[index]
	}
	if !leftActive || !rightActive || !different {
		t.Fatalf("profile activity left=%v right=%v distinct=%v", leftActive, rightActive, different)
	}
}
