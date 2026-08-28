//go:build tinygo

// audiopsg is the smallest production-shaped Audio v2 link check. It starts a
// looping CH1 phrase through the 256 Hz Timer0 scheduler and never touches
// Direct Sound, DMA, or FIFO registers.
package main

import (
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

var (
	controller audio.Controller
	sounds     = [...]audio.Sound{
		{
			ID:   0,
			Kind: audio.SoundCH1,
			Pulse1: hw.Pulse1Config{
				Pulse: hw.PulseConfig{
					Duty: hw.PulseDuty50,
					Envelope: hw.HardwareEnvelope{
						Initial: 12,
					},
					Route: hw.SoundRouteBoth,
				},
			},
		},
	}
	clips = [...]audio.Clip{
		{
			ID:          0,
			Kind:        audio.SoundCH1,
			LengthTicks: 96,
			NoteEvents: []audio.NoteEvent{
				{Tick: 0, DurationTicks: 24, Note: 60, SoundID: 0, Priority: 100},
				{Tick: 24, DurationTicks: 24, Note: 64, SoundID: 0, Priority: 100},
				{Tick: 48, DurationTicks: 24, Note: 67, SoundID: 0, Priority: 100},
				{Tick: 72, DurationTicks: 24, Note: 72, SoundID: 0, Priority: 100},
			},
		},
	}
	parts = [...]audio.Part{
		{ID: 0, LengthTicks: 96, PSGClips: [4]int16{0, -1, -1, -1}},
	}
	songs = [...]audio.Song{
		{ID: 0, Tempo: 120, Sequence: []uint8{0}, LoopStart: 0},
	}
)

func main() {
	if !controller.Init(audio.ProfilePSG) || !controller.Bind(audio.Library{
		Sounds: sounds[:],
		Clips:  clips[:],
		Parts:  parts[:],
		Songs:  songs[:],
	}) || !controller.PlaySong(0) || !controller.Start() {
		panic("audio v2 PSG startup failed")
	}
	for {
	}
}
