package transpiler

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
)

func nativeTranspilerOptions(t *testing.T) Options {
	t.Helper()
	loop := 0
	sounds := assetsync.NativeSoundLibrary{Version: assetsync.NativeAudioSchemaVersion, Sounds: []assetsync.NativeSoundAsset{{
		ID: 0, Name: "lead", Kind: "ch1", Route: "both",
		Pulse: &assetsync.NativePulseAsset{
			Duty:     50,
			Envelope: assetsync.NativeEnvelopeAsset{Initial: 12, Direction: "down", Pace: 2},
			Sweep:    &assetsync.NativeSweepAsset{Pace: 2, Direction: "up", Shift: 1},
		},
	}}}
	clipID := 0
	music := assetsync.NativeMusicLibrary{
		Version: assetsync.NativeAudioSchemaVersion,
		PPQN:    audio.NativePPQN,
		Clips: []assetsync.NativeClipAsset{{
			ID: 0, Name: "lead-a", Kind: "ch1", LengthTicks: 96,
			Events: []assetsync.NativeEventAsset{{Tick: 0, DurationTicks: 24, Note: 69, SoundID: 0, Priority: 100}},
		}},
		Parts: []assetsync.NativePartAsset{{
			ID: 0, Name: "intro", LengthTicks: 96,
			Lanes: assetsync.NativeLaneAssets{CH1: &clipID},
		}},
		Songs: []assetsync.NativeSongAsset{{ID: 0, Name: "song", Tempo: 120, Sequence: []int{0}, LoopStart: &loop}},
	}
	settings := assetsync.DefaultNativeAudioSettings()
	library, err := assetsync.CompileNativeAudio(settings, sounds, music)
	if err != nil {
		t.Fatal(err)
	}
	return Options{NativeAudio: &assetsync.NativeAudioAssets{Settings: settings, Sounds: sounds, Music: music, Library: library}}
}

func TestNativeAudioTranspilesToPSGControllerWithoutSoftwareStream(t *testing.T) {
	got, err := Transpile("native.lua", `
function _init()
  music(0)
  sfx(0, 100)
  gba_note(1, 69, 0, 24)
  gba_stop(1)
end
`, nativeTranspilerOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{
		"var nativeAudioController audio.Controller", "nativeSoundROM", "nativeClipROM",
		"ctx.AttachNativeAudio(&nativeAudioController)", "nativeAudioController.Start()",
		"c.GBANoteV(", "c.GBAStopV(", "c.SFXV(", "c.MusicV(",
	} {
		if !strings.Contains(got, wanted) {
			t.Errorf("generated native cartridge is missing %q", wanted)
		}
	}
	for _, forbidden := range []string{"audio.MonoStream", "audio.Mixer", "audio.SFXPlayer", "DirectSoundRate", "samplePCMROM", ".Service(audio.StreamPageCount)"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("generated native cartridge contains legacy token %q", forbidden)
		}
	}
	compileGenerated(t, got)
}

func TestNativeAudioRejectsLegacyCallsWrongKindsAndMissingIDs(t *testing.T) {
	options := nativeTranspilerOptions(t)
	tests := []struct {
		source string
		want   string
	}{
		{`function _init() sample(0) end`, "not part of four-channel audio v2"},
		{`function _init() sfx(0, 1, 2) end`, "optional priority"},
		{`function _init() music() end`, "exactly one id"},
		{`function _init() music(9) end`, "not present"},
		{`function _init() gba_note(2, 69, 0) end`, "belongs to CH1"},
		{`function _init() gba_note(1, 69, 9) end`, "not present"},
		{`function _init() gba_stop(5) end`, "1 through 4"},
	}
	for _, test := range tests {
		if _, err := Transpile("invalid-native.lua", test.source, options); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("Transpile(%q) error = %v, want %q", test.source, err, test.want)
		}
	}
}

func TestNativeAudioStaticNoteValidationUsesCH3WaveDimension(t *testing.T) {
	options := nativeTranspilerOptions(t)
	wave := make([]int, 64)
	for index := range 64 {
		wave[index] = index & 15
	}
	options.NativeAudio.Sounds.Sounds = append(options.NativeAudio.Sounds.Sounds, assetsync.NativeSoundAsset{
		ID: 1, Name: "wide wave", Kind: "ch3", Route: "both",
		Wave: &assetsync.NativeWaveAsset{Samples: wave, Level: 100},
	})
	if _, err := Transpile("wave64.lua", `function _init() gba_note(3, 12, 1) end`, options); err != nil {
		t.Fatalf("64-sample CH3 rejected its low hardware note: %v", err)
	}
	options.NativeAudio.Sounds.Sounds[1].Wave.Samples = wave[:32]
	if _, err := Transpile("wave32.lua", `function _init() gba_note(3, 12, 1) end`, options); err == nil || !strings.Contains(err.Error(), "hardware range") {
		t.Fatalf("32-sample CH3 low-note error = %v, want hardware range", err)
	}
}

func TestNativeBuiltinsRequireNativeAudioMode(t *testing.T) {
	if _, err := Transpile("legacy.lua", `function _init() gba_note(1, 69, 0) end`, Options{}); err == nil || !strings.Contains(err.Error(), "requires four-channel audio v2") {
		t.Fatalf("legacy gba_note error = %v", err)
	}
}
