package audio

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

// FuzzNativeControllerCommandsNeverEscapeFourChannels drives arbitrary public
// commands through the fixed queue and scheduler. Rejection is valid; panic,
// an invalid channel owner, or an unbounded queue is not.
func FuzzNativeControllerCommandsNeverEscapeFourChannels(f *testing.F) {
	f.Add([]byte{2, 0, 60, 0, 24, 1, 0, 3, 1, 2, 3})
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})
	f.Fuzz(func(t *testing.T, data []byte) {
		sound := Sound{ID: 0, Kind: SoundCH1, Pulse1: hw.Pulse1Config{Pulse: hw.PulseConfig{Duty: hw.PulseDuty50, Envelope: hw.HardwareEnvelope{Initial: 12}, Route: hw.SoundRouteBoth}}}
		events := []NoteEvent{{Tick: 0, DurationTicks: 24, Note: 60, SoundID: 0, Priority: 100}}
		library := Library{
			Sounds: []Sound{sound},
			Clips:  []Clip{{ID: 0, Kind: SoundCH1, LengthTicks: 24, NoteEvents: events}},
			Parts:  []Part{{ID: 0, LengthTicks: 24, PSGClips: [4]int16{0, -1, -1, -1}}},
			Songs:  []Song{{ID: 0, Tempo: 120, Sequence: []uint8{0}, LoopStart: 0}},
		}
		var controller Controller
		if !controller.Init(ProfilePSG) || !controller.Bind(library) {
			t.Fatal("valid fuzz fixture was rejected")
		}
		for index := 0; index+3 < len(data); index += 4 {
			switch data[index] % 6 {
			case 0:
				controller.NoteOn(uint8(data[index+1]%6), data[index+2], data[index+3], uint16(data[index+1]))
			case 1:
				controller.StopChannel(uint8(data[index+1] % 6))
			case 2:
				controller.PlaySFX(data[index+1], data[index+2])
			case 3:
				controller.StopSFX()
			case 4:
				controller.PlaySong(data[index+1])
			case 5:
				controller.StopSong()
			}
			controller.serviceCommit()
		}
		stats := controller.Stats()
		if controller.commandLen > NativeMaxCommandQueue {
			t.Fatalf("queue length = %d", controller.commandLen)
		}
		for channel, state := range stats.Channels {
			if state.Owner > OwnerDirect {
				t.Fatalf("CH%d owner = %d", channel+1, state.Owner)
			}
		}
	})
}
