package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func TestContextSFXValueDefaultsAndStop(t *testing.T) {
	var mixer audio.Mixer
	if !mixer.Init(4) {
		t.Fatal("mixer Init failed")
	}
	definitions := []audio.SFXDefinition{{
		ID: 3, Speed: 1, StepCount: 1, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop,
	}}
	steps := []audio.SFXStep{{PhaseStep: 1 << 30, Volume: audio.MaxVolume, Kind: audio.SourceTriangle}}
	var player audio.SFXPlayer
	if !player.Init(&mixer, definitions, steps) {
		t.Fatal("player Init failed")
	}
	var context Context
	context.AttachSFX(&player)
	context.SFXV(IntVal(3), Value{}, Value{}, Value{})
	if stats := context.SFXStats(); stats.ActiveChannels != 1 || stats.Starts != 1 {
		t.Fatalf("stats after default call = %+v", stats)
	}
	context.SFXV(IntVal(-1), Value{}, Value{}, Value{})
	if stats := context.SFXStats(); stats.ActiveChannels != 0 || stats.Stops != 1 {
		t.Fatalf("stats after stop-all = %+v", stats)
	}
}

func TestContextSFXWithoutPlayerFailsClosed(t *testing.T) {
	var context Context
	if context.SFX(0, -1, 0, 0) {
		t.Fatal("SFX succeeded without an attached player")
	}
	context.SFXV(IntVal(0), Value{}, Value{}, Value{})
}

func TestContextAudioStatsCombinesAttachedProductionState(t *testing.T) {
	hw.ResetForTest()
	var mixer audio.Mixer
	if !mixer.Init(4) {
		t.Fatal("mixer Init failed")
	}
	definitions := []audio.SFXDefinition{{
		ID: 1, Speed: 1, StepCount: 1, LoopStart: 0, LoopEnd: 1,
	}}
	steps := []audio.SFXStep{{PhaseStep: 1 << 30, Volume: audio.MaxVolume, Kind: audio.SourceSquare}}
	var player audio.SFXPlayer
	if !player.Init(&mixer, definitions, steps) {
		t.Fatal("player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("looping SFX start failed")
	}
	var stream audio.MonoStream
	if !stream.StartSFX(&player, hw.DirectSoundRate16384) {
		t.Fatal("stream start failed")
	}
	t.Cleanup(stream.Stop)

	var context Context
	context.AttachSFX(&player)
	context.AttachAudioStream(&stream)
	stats := context.AudioStats()
	if !stats.Stream.Running || stats.Stream.QueuedBlocks != 3 || stats.Mixer.ActiveVoices != 1 || stats.SFX.ActiveChannels != 1 {
		t.Fatalf("combined audio stats = %+v", stats)
	}
	if got := context.Stat(StatAudioQueuedBlocks); got != FromInt(3) {
		t.Fatalf("stat(audio queued blocks) = %v, want %v", got, FromInt(3))
	}
	context.AttachAudioStream(nil)
	if stats := context.AudioStats(); stats.Stream.Running || stats.Mixer.ActiveVoices != 1 {
		t.Fatalf("detached stream stats = %+v", stats)
	}
}

func TestContextMusicValueDefaultsStartAndStop(t *testing.T) {
	var mixer audio.Mixer
	if !mixer.Init(4) {
		t.Fatal("mixer Init failed")
	}
	definitions := []audio.SFXDefinition{{
		ID: 3, Speed: 1, StepCount: 1, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop,
	}}
	steps := []audio.SFXStep{{PhaseStep: 1 << 30, Volume: audio.MaxVolume, Kind: audio.SourceTriangle}}
	patterns := []audio.MusicPattern{{
		ID: 2, Channels: [audio.SFXChannels]int8{3, -1, -1, -1}, Next: -1, DurationSamples: 128,
	}}
	var player audio.SFXPlayer
	if !player.Init(&mixer, definitions, steps) || !player.InitMusic(patterns) {
		t.Fatal("player Init failed")
	}
	var context Context
	context.AttachSFX(&player)
	context.MusicV(IntVal(2), Value{})
	if state := player.MusicState(); !state.Active || state.PatternID != 2 {
		t.Fatalf("music state after start = %+v", state)
	}
	context.MusicV(Value{}, Value{})
	if state := player.MusicState(); state.Active {
		t.Fatalf("music() default stop left state = %+v", state)
	}
}

func TestContextMusicValuePassesFadeMilliseconds(t *testing.T) {
	var mixer audio.Mixer
	if !mixer.Init(4) {
		t.Fatal("mixer Init failed")
	}
	definitions := []audio.SFXDefinition{{
		ID: 3, Speed: 1, StepCount: 1, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop,
	}}
	steps := []audio.SFXStep{{PhaseStep: 1, Volume: audio.MaxVolume, Kind: audio.SourceSquare}}
	patterns := []audio.MusicPattern{{
		ID: 2, Channels: [audio.SFXChannels]int8{3, -1, -1, -1}, Next: 2, DurationSamples: 128,
	}}
	var player audio.SFXPlayer
	if !player.Init(&mixer, definitions, steps) || !player.InitMusic(patterns) {
		t.Fatal("player Init failed")
	}
	var context Context
	context.AttachSFX(&player)
	context.MusicV(IntVal(2), IntVal(125))
	if state := player.MusicState(); !state.Active || state.FadeSamplesRemaining != 2048 || state.FadingOut {
		t.Fatalf("music fade-in state = %+v", state)
	}
	player.MixMono(make([]int8, 2048))
	context.MusicV(IntVal(-1), IntVal(125))
	if state := player.MusicState(); state.FadeSamplesRemaining != 2048 || !state.FadingOut {
		t.Fatalf("music fade-out state = %+v", state)
	}
	player.MixMono(make([]int8, 2049))
	if state := player.MusicState(); state.Active {
		t.Fatalf("music fade-out left state = %+v", state)
	}
}

func TestContextSampleValueDefaultsOverridesAndStop(t *testing.T) {
	var mixer audio.Mixer
	if !mixer.Init(6) {
		t.Fatal("mixer init failed")
	}
	var player audio.SFXPlayer
	if !player.Init(&mixer, nil, nil) || !player.InitSamples([]audio.SampleDefinition{{
		ID: 3, DataLength: 2, Volume: 128, Priority: 70,
	}}, []int8{20, -20}) {
		t.Fatal("sample player init failed")
	}
	var context Context
	context.AttachSFX(&player)
	context.SampleV(IntVal(3), Value{}, Value{})
	pcm := make([]int8, 1)
	player.MixMono(pcm)
	if pcm[0] != 10 {
		t.Fatalf("default sample volume PCM = %d, want 10", pcm[0])
	}
	context.SampleV(IntVal(3), IntVal(256), IntVal(200))
	player.MixMono(pcm)
	if pcm[0] != 10 { // two voices: remaining -10 plus new +20
		t.Fatalf("overridden overlapping sample PCM = %d, want 10", pcm[0])
	}
	context.SampleV(IntVal(-1), Value{}, Value{})
	if stats := context.SFXStats(); stats.SampleStarts != 2 || stats.SampleStops != 1 || stats.ActiveSampleVoices != 0 {
		t.Fatalf("sample context stats = %+v", stats)
	}
}

func TestContextMusicWithoutPlayerFailsClosed(t *testing.T) {
	var context Context
	if context.Music(0, 0) {
		t.Fatal("Music succeeded without an attached player")
	}
	context.MusicV(IntVal(0), Value{})
}
