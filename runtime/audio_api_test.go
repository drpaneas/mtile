package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func editorSongAndGameplaySFXLibrary() audio.Library {
	envelope := hw.HardwareEnvelope{Initial: 15, Direction: hw.EnvelopeDown}
	pulse := hw.Pulse1Config{Pulse: hw.PulseConfig{Duty: hw.PulseDuty50, Envelope: envelope, Route: hw.SoundRouteBoth}}
	return audio.Library{
		Sounds: []audio.Sound{{ID: 10, Kind: audio.SoundCH1, Pulse1: pulse}},
		Clips: []audio.Clip{
			{ID: 1, Kind: audio.SoundCH1, LengthTicks: 8, NoteEvents: []audio.NoteEvent{
				{Tick: 0, DurationTicks: 8, Note: 60, SoundID: 10, Priority: audio.NativeEditorNotePriority},
			}},
			{ID: 2, Kind: audio.SoundCH1, LengthTicks: 4, NoteEvents: []audio.NoteEvent{
				{Tick: 0, DurationTicks: 2, Note: 72, SoundID: 10, Priority: 100},
			}},
		},
		Parts: []audio.Part{{ID: 30, LengthTicks: 8, PSGClips: [4]int16{1, -1, -1, -1}}},
		Songs: []audio.Song{{ID: 40, Tempo: 120, Sequence: []uint8{30}, LoopStart: -1}},
	}
}

func TestContextOmittedSFXPriorityStealsEditorSongNote(t *testing.T) {
	hw.ResetForTest()
	var controller audio.Controller
	if !controller.Init(audio.ProfilePSG) {
		t.Fatal("controller init failed")
	}
	if !controller.Bind(editorSongAndGameplaySFXLibrary()) {
		t.Fatal("controller bind failed")
	}
	var context Context
	context.AttachNativeAudio(&controller)
	context.MusicV(IntVal(40), Value{})
	controller.Service()
	if got := controller.Stats(); got.Channels[0].Owner != audio.OwnerSong || got.Channels[0].Note != 60 {
		t.Fatalf("song setup = %+v", got.Channels[0])
	}
	context.SFXV(IntVal(2), Value{}, Value{}, Value{})
	controller.Service()
	got := controller.Stats()
	if got.PriorityRejects != 0 || got.Channels[0].Owner != audio.OwnerSFX || got.Channels[0].Note != 72 {
		t.Fatalf("omitted sfx() priority lost to editor music: %+v", got)
	}
}

func TestContextNativeAudioStopsAndDiagnostics(t *testing.T) {
	hw.ResetForTest()
	var controller audio.Controller
	if !controller.Init(audio.ProfilePSG) || !controller.Bind(editorSongAndGameplaySFXLibrary()) {
		t.Fatal("controller setup failed")
	}
	var context Context
	context.AttachNativeAudio(&controller)
	context.MusicV(IntVal(40), Value{})
	controller.Service()
	context.SFXV(IntVal(2), Value{}, Value{}, Value{})
	controller.Service()
	context.SFXV(IntVal(-1), Value{}, Value{}, Value{})
	controller.Service()
	if got := context.AudioStats().Native; got.Channels[0].Owner == audio.OwnerSFX {
		t.Fatalf("SFX stop failed: %+v", got)
	}
	context.MusicV(IntVal(-1), Value{})
	controller.Service()
	if got := context.AudioStats().Native; got.Channels[0].Active {
		t.Fatalf("music stop failed: %+v", got)
	}
	if context.Stat(StatAudioServiceCalls) != FromInt(4) {
		t.Fatal("PSG service counter was not exposed")
	}
	context.AttachNativeAudio(nil)
	if context.AudioStats() != (AudioStats{}) || context.Stat(StatAudioServiceCalls) != 0 {
		t.Fatal("detached controller leaked telemetry")
	}
}

func TestContextAssetFreeAudioCallsRemainNoOps(t *testing.T) {
	for _, context := range []*Context{nil, {}} {
		context.SFXV(IntVal(1), Value{}, Value{}, Value{})
		context.MusicV(IntVal(1), Value{})
		context.SampleV(IntVal(1), Value{}, Value{})
		if context.AudioStats() != (AudioStats{}) {
			t.Fatal("asset-free call created audio state")
		}
	}
}
