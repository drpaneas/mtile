// Package stereocapture defines the deterministic two-voice profile shared by
// the target stereo-routing ROM and its host reference renderer.
package stereocapture

import "github.com/drpaneas/mtile/runtime/audio"

const (
	MixerVoiceLimit = 4
	ActiveVoices    = 2
)

// Start initializes mixer with spectrally distinct hard-left and hard-right
// voices. Keeping this profile in one package prevents the target capture and
// host reference from drifting apart.
func Start(mixer *audio.Mixer) bool {
	if mixer == nil || !mixer.Init(MixerVoiceLimit) {
		return false
	}
	requests := [...]audio.Request{
		{
			Kind:     audio.SourceSquare,
			Step:     1 << 27, // 512 Hz at 16,384 Hz.
			Volume:   32,
			Pan:      audio.MinPan,
			Priority: 1,
		},
		{
			Kind:     audio.SourceTriangle,
			Step:     3 << 26, // 768 Hz at 16,384 Hz.
			Volume:   32,
			Pan:      audio.MaxPan,
			Priority: 1,
		},
	}
	for _, request := range requests {
		if _, ok := mixer.Start(request); !ok {
			return false
		}
	}
	return true
}
