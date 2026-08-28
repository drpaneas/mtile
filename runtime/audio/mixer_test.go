package audio

import (
	"testing"
	"unsafe"
)

func newMixer(t *testing.T, voices int) *Mixer {
	t.Helper()
	var m Mixer
	if !m.Init(voices) {
		t.Fatalf("Init(%d) failed", voices)
	}
	return &m
}

func start(t *testing.T, m *Mixer, r Request) int {
	t.Helper()
	index, ok := m.Start(r)
	if !ok {
		t.Fatalf("Start(%+v) failed", r)
	}
	return index
}

func mix(t *testing.T, m *Mixer, count int) []int8 {
	t.Helper()
	dst := make([]int8, count)
	m.MixMono(dst)
	return dst
}

func requireSamples(t *testing.T, got, want []int8) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sample length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample[%d] = %d, want %d; got %v", i, got[i], want[i], got)
		}
	}
}

func TestSupportedVoiceProfiles(t *testing.T) {
	for _, count := range []int{4, 6, 8, 12} {
		if !SupportedVoiceCount(count) {
			t.Errorf("SupportedVoiceCount(%d) = false", count)
		}
	}
	for _, count := range []int{-1, 0, 1, 5, 10, 13} {
		if SupportedVoiceCount(count) {
			t.Errorf("SupportedVoiceCount(%d) = true", count)
		}
	}
}

func TestOscillatorGoldenSamples(t *testing.T) {
	tests := []struct {
		kind SourceKind
		step uint32
		want []int8
	}{
		{SourceSquare, 1 << 31, []int8{127, -127, 127, -127}},
		{SourceTriangle, 1 << 30, []int8{-127, 1, 127, -1}},
		{SourceTiltedSaw, 1 << 29, []int8{-127, -91, -55, -19, 18, 54, 90, 127}},
		{SourceOrgan, 1 << 29, []int8{-85, 0, 85, 0, -85, -29, 28, -28}},
		{SourcePhaser, 1 << 30, []int8{-127, 0, 126, 0}},
		{SourceSaw, 1 << 30, []int8{-128, -64, 0, 64}},
		{SourceNoise, 1 << 31, []int8{1, -128, 27, -128, 33, 68, -128, 127}},
		{SourcePulse, 1 << 30, []int8{127, 127, -127, -127}},
	}
	for _, tt := range tests {
		m := newMixer(t, 4)
		start(t, m, Request{Kind: tt.kind, Step: tt.step, Volume: MaxVolume, NoiseSeed: 1})
		requireSamples(t, mix(t, m, len(tt.want)), tt.want)
	}
}

func TestTriangleBuzzLookupMatchesPiecewiseFormula(t *testing.T) {
	for position := 0; position < 256; position++ {
		phase := uint32(position) << 24
		plain := triangleSample(phase)
		want := int16((3*int32(plain) + int32(tiltedSawSample(phase))) / 4)
		if got := triangleFilteredSample(phase, true); got != want {
			t.Fatalf("triangle buzz position %d = %d, want %d", position, got, want)
		}
	}
}

func TestPICONoiseParametersMatchCaptureEndpoints(t *testing.T) {
	if alpha, gain := picoNoiseParameters(picoPitchPhaseSteps[0]); alpha != 5 || gain != 1830 {
		t.Fatalf("pitch 0 noise parameters = (%d,%d), want (5,1830)", alpha, gain)
	}
	if alpha, gain := picoNoiseParameters(picoPitchPhaseSteps[63]); alpha != 229 || gain != 534 {
		t.Fatalf("pitch 63 noise parameters = (%d,%d), want (229,534)", alpha, gain)
	}
}

func TestPICONoizUsesPitchClockedSampleAndHold(t *testing.T) {
	voice := Voice{Noise: 1, NoiseValue: 12, Filter: SFXFilterNoiz, Phase2: 0xfffffff0, Step2: 32}
	if got := picoNoiseSample(&voice); got != 12 {
		t.Fatalf("NOIZ held sample = %d, want 12", got)
	}
	if voice.NoiseValue == 12 || voice.Phase2 != 16 {
		t.Fatalf("NOIZ wrap state = %+v, want a new value at phase 16", voice)
	}
	if got := picoNoiseSample(&voice); got != int16(voice.NoiseValue) {
		t.Fatalf("NOIZ post-wrap held sample = %d, want %d", got, voice.NoiseValue)
	}
}

func TestPICONoizClockUsesCapturedKeyHoldLengths(t *testing.T) {
	for _, key := range []int{0, 12, 36, 62} {
		want := uint32((uint64(22050) << 32) / (uint64(MusicSampleRate) * uint64(64-key)))
		if got := picoNoizPhaseStep(picoPitchPhaseSteps[key]); got != want {
			t.Fatalf("NOIZ key %d phase step = %d, want %d", key, got, want)
		}
	}
	if got := picoNoizPhaseStep(picoPitchPhaseSteps[63]); got != ^uint32(0) {
		t.Fatalf("NOIZ key 63 phase step = %d, want target-rate clamp", got)
	}
}

func TestPICODetuneUsesCapturedPhaseRatios(t *testing.T) {
	step := picoPitchPhaseSteps[36]
	tests := []struct {
		kind  SourceKind
		level uint8
		want  uint32
	}{
		{SourceTriangle, 1, uint32((uint64(step)*49385 + 1<<15) >> 16)},
		{SourceTriangle, 2, step + step/2},
		{SourceSquare, 1, scaledPhaseStep(step, 199, 200)},
		{SourceSquare, 2, scaledPhaseStep(step, 199, 100)},
		{SourceOrgan, 2, scaledPhaseStep(step, 199, 50)},
		{SourcePhaser, 1, uint32((uint64(step)*63976 + 1<<15) >> 16)},
		{SourcePhaser, 2, uint32((uint64(step)*130017 + 1<<15) >> 16)},
	}
	for _, test := range tests {
		if got := detunePhaseStep(test.kind, test.level, step); got != test.want {
			t.Fatalf("%v detune %d phase step = %d, want %d", test.kind, test.level, got, test.want)
		}
	}
}

func TestCustomPitchTranspositionUsesExactPICOKeysAndEffectFallback(t *testing.T) {
	if got := transposedPICOPhaseStep(picoPitchPhaseSteps[31], picoPitchPhaseSteps[30]); got != picoPitchPhaseSteps[37] {
		t.Fatalf("exact custom transposition = %d, want key 37 step %d", got, picoPitchPhaseSteps[37])
	}
	if got, want := transposedPICOPhaseStep(picoPitchPhaseSteps[2], picoPitchPhaseSteps[0]), extendedPICOPhaseStep(-22); got != want {
		t.Fatalf("low custom transposition = %d, want extended key -22 step %d", got, want)
	}
	// Celeste Postcard uses inner pitch 0 under outer custom pitch 9. PICO-8
	// keeps the resulting key -15 instead of clamping it to authored pitch 0.
	if got, want := transposedPICOPhaseStep(picoPitchPhaseSteps[0], picoPitchPhaseSteps[9]), extendedPICOPhaseStep(-15); got != want {
		t.Fatalf("Postcard custom transposition = %d, want extended key -15 step %d", got, want)
	}
	interpolated := (picoPitchPhaseSteps[24] + picoPitchPhaseSteps[25]) / 2
	want := scaledPhaseStep(interpolated, picoPitchPhaseSteps[30], picoPitchPhaseSteps[24])
	if got := transposedPICOPhaseStep(interpolated, picoPitchPhaseSteps[30]); got != want {
		t.Fatalf("interpolated custom transposition = %d, want ratio fallback %d", got, want)
	}
}

func TestPICOOrganDetuneTwoUsesSpecialBlend(t *testing.T) {
	voice := Voice{
		Kind: SourceOrgan, Detune: 2,
		Phase: 0, Phase3: 0, Step3: 17,
	}
	primary := organFilteredSample(voice.Phase, false)
	secondary := triangleFilteredSample(voice.Phase3, false)
	want := int16((int32(primary)*15 + int32(secondary)*3) / 16)
	if got := organVoiceSample(&voice); got != want {
		t.Fatalf("organ detune-2 sample = %d, want %d", got, want)
	}
	if voice.Phase3 != 17 {
		t.Fatalf("organ detune-2 secondary phase = %d, want 17", voice.Phase3)
	}
}

func TestWavetableGoldenSamples(t *testing.T) {
	var table [WavetableSamples]int8
	for i := range table {
		table[i] = int8(i - 16)
	}
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourceWavetable, Step: 1 << 27, Volume: MaxVolume, Wavetable: &table})
	requireSamples(t, mix(t, m, 5), []int8{-16, -15, -14, -13, -12})
}

func TestPCMSteppingAndLooping(t *testing.T) {
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, PCM: []int8{10, -20, 30}})
	requireSamples(t, mix(t, m, 4), []int8{10, -20, 30, 0})
	if got := m.Stats().ActiveVoices; got != 0 {
		t.Fatalf("active voices after PCM end = %d, want 0", got)
	}

	m = newMixer(t, 4)
	start(t, m, Request{
		Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume,
		PCM: []int8{1, 2, 3, 4}, Loop: true, LoopStart: 1, LoopEnd: 4,
	})
	requireSamples(t, mix(t, m, 7), []int8{1, 2, 3, 4, 2, 3, 4})

	m = newMixer(t, 4)
	start(t, m, Request{Kind: SourcePCM, Step: 1 << 15, Volume: MaxVolume, PCM: []int8{10, 20, 30}})
	requireSamples(t, mix(t, m, 6), []int8{10, 10, 20, 20, 30, 30})
}

func TestMixSaturatesSignedEightBitOutput(t *testing.T) {
	m := newMixer(t, 4)
	r := Request{Kind: SourceSquare, Step: 1 << 31, Volume: MaxVolume}
	start(t, m, r)
	start(t, m, r)
	requireSamples(t, mix(t, m, 2), []int8{127, -128})
}

func TestVoiceAllocationPriorityProtectionAndTelemetry(t *testing.T) {
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 1})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 2})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 1, Protected: true})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 3})

	index, ok := m.Start(Request{Kind: SourceTriangle, Step: 1, Volume: 1, Priority: 1})
	if !ok || index != 0 {
		t.Fatalf("equal-priority request chose voice %d, ok=%t; want oldest eligible voice 0", index, ok)
	}
	if _, ok := m.Start(Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 0}); ok {
		t.Fatal("priority 0 request unexpectedly stole a higher/protected voice")
	}
	stats := m.Stats()
	if stats.ActiveVoices != 4 || stats.PeakActiveVoices != 4 || stats.VoiceSteals != 1 || stats.FailedStarts != 1 {
		t.Fatalf("stats = %+v, want active=4 peak=4 steals=1 failed=1", stats)
	}
	v, _ := m.Voice(2)
	if !v.Protected || !v.Active {
		t.Fatalf("protected voice changed: %+v", v)
	}
}

func TestStopKeepsStableIDsAndCompactsActiveMixing(t *testing.T) {
	m := newMixer(t, 4)
	r := Request{Kind: SourceSquare, Step: 1 << 31, Volume: 32}
	if got := start(t, m, r); got != 0 {
		t.Fatalf("first voice = %d, want 0", got)
	}
	if got := start(t, m, r); got != 1 {
		t.Fatalf("second voice = %d, want 1", got)
	}
	if got := start(t, m, r); got != 2 {
		t.Fatalf("third voice = %d, want 2", got)
	}
	if !m.Stop(1) {
		t.Fatal("Stop(1) failed")
	}
	if v, _ := m.Voice(2); !v.Active {
		t.Fatal("stopping voice 1 changed stable voice 2")
	}
	if got := start(t, m, r); got != 1 {
		t.Fatalf("reused voice = %d, want inactive physical slot 1", got)
	}
	if stats := m.Stats(); stats.ActiveVoices != 3 || stats.PeakActiveVoices != 3 {
		t.Fatalf("stats after stop/reuse = %+v", stats)
	}
}

func TestSetVoiceStepVolumeRequiresCurrentOwnership(t *testing.T) {
	m := newMixer(t, 4)
	id := start(t, m, Request{Kind: SourceSquare, Step: 10, Volume: 20})
	voice, _ := m.Voice(id)
	if !m.SetVoiceStepVolume(id, voice.Started, 30, 40) {
		t.Fatal("owned synth update failed")
	}
	updated, _ := m.Voice(id)
	if updated.Step != 30 || updated.Volume != 40 || updated.Started != voice.Started {
		t.Fatalf("updated voice = %+v", updated)
	}
	if !m.Stop(id) {
		t.Fatal("stop before ID reuse failed")
	}
	if reused := start(t, m, Request{Kind: SourceTriangle, Step: 50, Volume: 60}); reused != id {
		t.Fatalf("reused ID = %d, want %d", reused, id)
	}
	if m.SetVoiceStepVolume(id, voice.Started, 70, 80) {
		t.Fatal("stale ownership generation changed a reused voice")
	}
	reused, _ := m.Voice(id)
	if reused.Step != 50 || reused.Volume != 60 {
		t.Fatalf("stale update mutated reused voice: %+v", reused)
	}
}

func TestSetVoiceStepVolumeUpdatesPitchDependentSources(t *testing.T) {
	m := newMixer(t, 4)
	noiseID := start(t, m, Request{
		Kind: SourceNoise, Step: picoPitchPhaseSteps[0], Volume: 20, NoiseSeed: 1,
	})
	noise, _ := m.Voice(noiseID)
	if !m.SetVoiceStepVolume(noiseID, noise.Started, picoPitchPhaseSteps[63], 40) {
		t.Fatal("noise pitch update failed")
	}
	noise, _ = m.Voice(noiseID)
	if noise.NoiseAlpha != 229 || noise.NoiseGain != 534 {
		t.Fatalf("updated noise parameters = (%d,%d), want (229,534)", noise.NoiseAlpha, noise.NoiseGain)
	}

	phaserID := start(t, m, Request{Kind: SourcePhaser, Step: picoPitchPhaseSteps[24], Volume: 20})
	phaser, _ := m.Voice(phaserID)
	if !m.SetVoiceStepVolume(phaserID, phaser.Started, picoPitchPhaseSteps[57], 40) {
		t.Fatal("phaser pitch update failed")
	}
	phaser, _ = m.Voice(phaserID)
	if phaser.Step2 != phaserPhaseStep(picoPitchPhaseSteps[57]) {
		t.Fatalf("updated phaser second step = %d, want %d", phaser.Step2, phaserPhaseStep(picoPitchPhaseSteps[57]))
	}
}

func TestVoiceVolumeRampIsSampleContinuousAndOwnershipSafe(t *testing.T) {
	m := newMixer(t, 4)
	id := start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: MaxVolume})
	voice, _ := m.Voice(id)
	if !m.SetVoiceVolumeRamp(id, voice.Started, 0, MaxVolume, 4) {
		t.Fatal("owned fade-in ramp failed")
	}
	requireSamples(t, mix(t, m, 5), []int8{0, 31, 63, 95, 127})
	if m.ramps != 0 {
		t.Fatalf("fade-in left %d live ramps", m.ramps)
	}

	if !m.SetVoiceVolumeRamp(id, voice.Started, MaxVolume, 0, 4) {
		t.Fatal("owned fade-out ramp failed")
	}
	requireSamples(t, mix(t, m, 5), []int8{127, 95, 63, 31, 0})
	if m.ramps != 0 {
		t.Fatalf("fade-out left %d live ramps", m.ramps)
	}

	if !m.Stop(id) {
		t.Fatal("stop before ramp ownership check failed")
	}
	if reused := start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 64}); reused != id {
		t.Fatalf("reused ID = %d, want %d", reused, id)
	}
	if m.SetVoiceVolumeRamp(id, voice.Started, 0, MaxVolume, 4) {
		t.Fatal("stale ownership generation configured a reused voice ramp")
	}
}

func TestMusicVolumeRampLeavesManualVoicesAtFullGain(t *testing.T) {
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 64, Music: true})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 64})
	if !m.SetMusicVolume(0) {
		t.Fatal("SetMusicVolume failed")
	}
	if got := mix(t, m, 1); got[0] != 31 {
		t.Fatalf("muted music + manual sample = %d, want 31", got[0])
	}
	if !m.RampMusicVolume(MaxVolume, 4) {
		t.Fatal("RampMusicVolume failed")
	}
	requireSamples(t, mix(t, m, 5), []int8{31, 38, 46, 54, 62})
}

func TestMixedMusicFadeWordPathMatchesBytePath(t *testing.T) {
	fixture := func() *Mixer {
		m := newMixer(t, 4)
		start(t, m, Request{Kind: SourceTriangle, Step: 1 << 30, Volume: 96, Music: true})
		start(t, m, Request{
			Kind: SourcePCM, Step: 1 << 16, Volume: 128,
			PCM: []int8{32, -16, 8, -4}, Loop: true, LoopStart: 0, LoopEnd: 4,
		})
		if !m.SetMusicVolume(0) || !m.RampMusicVolume(MaxVolume, 8) {
			t.Fatal("configure mixed music fade fixture failed")
		}
		return m
	}

	byteMixer := fixture()
	wordMixer := fixture()
	want := mix(t, byteMixer, 8)
	words := make([]uint32, 2)
	wordMixer.MixMonoWords(words)
	got := make([]int8, 8)
	for index := range got {
		got[index] = int8(words[index/4] >> uint(index%4*8))
	}
	requireSamples(t, got, want)
}

func TestMixedMusicFadePackingPreservesStableVoiceIDs(t *testing.T) {
	m := newMixer(t, 4)
	manualID := start(t, m, Request{
		Kind: SourcePCM, Step: 1 << 16, Volume: 64,
		PCM: []int8{8, 16, 24, 32}, Loop: true, LoopStart: 0, LoopEnd: 4,
	})
	musicID := start(t, m, Request{
		Kind: SourceTriangle, Step: 1 << 30, Volume: 64, Music: true,
	})
	manualBefore, _ := m.Voice(manualID)
	musicBefore, _ := m.Voice(musicID)
	if !m.SetMusicVolume(0) || !m.RampMusicVolume(MaxVolume, 8) {
		t.Fatal("configure packed mixed music fade failed")
	}
	m.MixMonoWords(make([]uint32, 1))
	manualAfter, manualOK := m.Voice(manualID)
	musicAfter, musicOK := m.Voice(musicID)
	if !manualOK || !musicOK || !manualAfter.Active || !musicAfter.Active ||
		manualAfter.Music || !musicAfter.Music || manualAfter.Started != manualBefore.Started ||
		musicAfter.Started != musicBefore.Started || manualAfter.Position != 0 ||
		m.positions[musicID] != 0 || m.positions[manualID] != 1 {
		t.Fatalf("packed voices manual=%+v music=%+v positions=%v", manualAfter, musicAfter, m.positions[:2])
	}
}

func TestMusicVolumeRampReachesEndpointWithRemainderClock(t *testing.T) {
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: MaxVolume, Music: true})
	if !m.SetMusicVolume(0) || !m.RampMusicVolume(MaxVolume, 6) {
		t.Fatal("configure music ramp failed")
	}
	requireSamples(t, mix(t, m, 7), []int8{0, 21, 42, 63, 84, 105, 127})
}

func TestMusicAndNoteVolumeRampsCompose(t *testing.T) {
	m := newMixer(t, 4)
	id := start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: MaxVolume, Music: true})
	voice, _ := m.Voice(id)
	if !m.SetVoiceVolumeRamp(id, voice.Started, 0, MaxVolume, 4) ||
		!m.SetMusicVolume(0) || !m.RampMusicVolume(MaxVolume, 4) {
		t.Fatal("configure composed ramps failed")
	}
	requireSamples(t, mix(t, m, 5), []int8{0, 7, 31, 71, 127})
}

func TestMusicVoiceCountTracksOwnershipChanges(t *testing.T) {
	m := newMixer(t, 4)
	musicPCM := start(t, m, Request{
		Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, Priority: 2, PCM: []int8{1}, Music: true,
	})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 2, Music: true})
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 2, Music: true})
	if m.musicVoices != 3 {
		t.Fatalf("music voices after starts = %d, want 3", m.musicVoices)
	}

	if _, ok := m.Start(Request{Kind: SourceTriangle, Step: 1, Volume: 1, Music: true}); !ok {
		t.Fatal("music voice steal failed")
	}
	if m.musicVoices != 4 {
		t.Fatalf("music voices after manual steal = %d, want 4", m.musicVoices)
	}
	if !m.Stop(musicPCM) || m.musicVoices != 3 {
		t.Fatalf("music voices after stop = %d, want 3", m.musicVoices)
	}
	if _, ok := m.Start(Request{
		Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, PCM: []int8{1}, Music: true,
	}); !ok {
		t.Fatal("replacement music PCM failed")
	}
	m.MixMono(make([]int8, 2))
	if m.musicVoices != 3 {
		t.Fatalf("music voices after PCM expiry = %d, want 3", m.musicVoices)
	}

	var checkpoint Mixer
	if !m.copyStateTo(&checkpoint) || checkpoint.musicVoices != m.musicVoices {
		t.Fatalf("checkpoint music voices = %d, want %d", checkpoint.musicVoices, m.musicVoices)
	}
	start(t, m, Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 2})
	if _, ok := m.Start(Request{Kind: SourceSquare, Step: 1, Volume: 1, Priority: 2}); !ok {
		t.Fatal("manual voice steal failed")
	}
	if m.musicVoices != 2 {
		t.Fatalf("music voices after manual steal = %d, want 2", m.musicVoices)
	}
	m.StopAll()
	if m.musicVoices != 0 {
		t.Fatalf("music voices after StopAll = %d, want 0", m.musicVoices)
	}
}

func TestPCMEndKeepsMovedVoiceIDStable(t *testing.T) {
	m := newMixer(t, 4)
	pcmID := start(t, m, Request{
		Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, PCM: []int8{10},
	})
	synthID := start(t, m, Request{
		Kind: SourceSquare, Step: 1 << 31, Volume: 64,
	})

	requireSamples(t, mix(t, m, 2), []int8{41, -32})
	if v, _ := m.Voice(pcmID); v.Active {
		t.Fatal("completed PCM ID is still active")
	}
	if v, _ := m.Voice(synthID); !v.Active || v.Kind != SourceSquare {
		t.Fatalf("moved synth ID changed: %+v", v)
	}
	if !m.Stop(synthID) {
		t.Fatal("Stop on moved synth ID failed")
	}
}

func TestCopyStateCopiesOnlyActiveVoicesAndRebuildsStableIDs(t *testing.T) {
	source := newMixer(t, 6)
	firstID := start(t, source, Request{Kind: SourceSquare, Step: 1, Volume: 32})
	removedID := start(t, source, Request{Kind: SourcePCM, Step: 1 << 16, Volume: 32, PCM: []int8{1}})
	movedID := start(t, source, Request{Kind: SourceTriangle, Step: 1, Volume: 32, Music: true})
	if !source.Stop(removedID) {
		t.Fatal("source Stop failed")
	}

	destination := newMixer(t, 6)
	for index := 0; index < 5; index++ {
		start(t, destination, Request{
			Kind: SourcePCM, Step: 1 << 16, Volume: 1,
			PCM: []int8{1, 2}, Loop: true, LoopStart: 0, LoopEnd: 2,
		})
	}
	if !source.copyStateTo(destination) {
		t.Fatal("copyStateTo failed")
	}
	first, firstOK := destination.Voice(firstID)
	moved, movedOK := destination.Voice(movedID)
	removed, removedOK := destination.Voice(removedID)
	if !firstOK || !movedOK || !removedOK || !first.Active || !moved.Active || removed.Active ||
		first.Kind != SourceSquare || moved.Kind != SourceTriangle || !moved.Music ||
		destination.active != 2 || destination.positions[movedID] != 1 {
		t.Fatalf("copied destination first=%+v moved=%+v removed=%+v active=%d positions=%v", first, moved, removed, destination.active, destination.positions[:3])
	}
	for position := int(destination.active); position < 5; position++ {
		if destination.voices[position].Active || destination.voices[position].PCM != nil {
			t.Fatalf("destination tail %d retained state: %+v", position, destination.voices[position])
		}
	}
}

func TestCopyStatePreservesPhaserAndNoiseOscillators(t *testing.T) {
	source := newMixer(t, 4)
	start(t, source, Request{
		Kind: SourcePhaser, Step: picoPitchPhaseSteps[57], Volume: 96,
		Filter: SFXFilterBuzz | SFXFilter(2*8),
	})
	start(t, source, Request{
		Kind: SourceNoise, Step: picoPitchPhaseSteps[42], Volume: 64, NoiseSeed: 123,
		Filter: SFXFilterNoiz,
	})
	start(t, source, Request{
		Kind: SourceSaw, Step: picoPitchPhaseSteps[36], Volume: 48,
		Filter: SFXFilterBuzz | SFXFilter(8),
	})
	mix(t, source, 137)

	destination := newMixer(t, 4)
	if !source.copyStateTo(destination) {
		t.Fatal("copyStateTo failed")
	}
	requireSamples(t, mix(t, destination, 512), mix(t, source, 512))
}

func TestInvalidRequestsFailSafely(t *testing.T) {
	m := newMixer(t, 4)
	invalid := []Request{
		{},
		{Kind: SourceSquare, Volume: MaxVolume},
		{Kind: SourceSquare, Step: 1, Volume: MaxVolume + 1},
		{Kind: SourceSquare, Step: 1, Pan: MaxPan + 1},
		{Kind: SourceWavetable, Step: 1},
		{Kind: SourceWavetable, Step: 1, Wavetable: &[WavetableSamples]int8{}, PCM: []int8{1}},
		{Kind: SourcePCM, Step: 1 << 16},
		{Kind: SourcePCM, Step: 1 << 16, Wavetable: &[WavetableSamples]int8{}, PCM: []int8{1}},
		{Kind: SourcePCM, Step: 1 << 16, PCM: []int8{1, 2}, Loop: true, LoopStart: 1, LoopEnd: 1},
	}
	for _, request := range invalid {
		if _, ok := m.Start(request); ok {
			t.Fatalf("invalid request accepted: %+v", request)
		}
	}
	if got := m.Stats().FailedStarts; got != uint32(len(invalid)) {
		t.Fatalf("FailedStarts = %d, want %d", got, len(invalid))
	}
}

func TestSetVoicePanRequiresCurrentOwnershipGeneration(t *testing.T) {
	m := newMixer(t, 4)
	id := start(t, m, Request{Kind: SourceSquare, Step: 1 << 31, Volume: MaxVolume})
	voice, _ := m.Voice(id)
	if !m.SetVoicePan(id, voice.Started, MinPan) {
		t.Fatal("SetVoicePan rejected current voice ownership")
	}
	if got, _ := m.Voice(id); got.Pan != MinPan {
		t.Fatalf("voice pan = %d, want %d", got.Pan, MinPan)
	}
	if m.SetVoicePan(id, voice.Started+1, MaxPan) {
		t.Fatal("SetVoicePan accepted stale ownership generation")
	}
	if m.SetVoicePan(id, voice.Started, MaxPan+1) {
		t.Fatal("SetVoicePan accepted out-of-range pan")
	}
}

func TestMixMonoWordsUsesFIFOByteOrder(t *testing.T) {
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourceSquare, Step: 1 << 31, Volume: MaxVolume})
	dst := []uint32{0}
	m.MixMonoWords(dst)
	if got, want := dst[0], uint32(0x817F817F); got != want {
		t.Fatalf("packed word = %#08x, want %#08x", got, want)
	}
}

func TestPlainOrganWordFastPathMatchesByteMixer(t *testing.T) {
	fixture := func() *Mixer {
		m := newMixer(t, 4)
		for index, step := range []uint32{68583572, 96991818, 137167144} {
			start(t, m, Request{
				Kind: SourceOrgan, Step: step, Volume: uint16(64 + index*32), Music: true,
			})
		}
		return m
	}
	byteMixer := fixture()
	wordMixer := fixture()
	want := mix(t, byteMixer, 260)
	words := make([]uint32, 65)
	wordMixer.MixMonoWords(words)
	got := make([]int8, len(want))
	for sample := range got {
		got[sample] = int8(uint8(words[sample/4] >> uint((sample&3)*8)))
	}
	requireSamples(t, got, want)
}

func TestPlainMixedPICOSynthWordFastPathMatchesByteMixer(t *testing.T) {
	fixture := func() *Mixer {
		m := newMixer(t, 8)
		kinds := []SourceKind{
			SourceSquare, SourceTriangle, SourceSaw,
			SourcePulse, SourceTiltedSaw, SourceOrgan,
			SourceNoise, SourcePhaser,
		}
		for index, kind := range kinds {
			start(t, m, Request{
				Kind: kind, Step: uint32(43204943 + index*6110101),
				Volume: uint16(32 + index*16), Music: index&1 != 0,
			})
		}
		return m
	}
	byteMixer := fixture()
	wordMixer := fixture()
	want := mix(t, byteMixer, 260)
	words := make([]uint32, 65)
	wordMixer.MixMonoWords(words)
	got := make([]int8, len(want))
	for sample := range got {
		got[sample] = int8(uint8(words[sample/4] >> uint((sample&3)*8)))
	}
	requireSamples(t, got, want)
}

func TestPlainMixedPICOSynthRampedWordFastPathMatchesByteMixer(t *testing.T) {
	fixture := func() *Mixer {
		m := newMixer(t, 4)
		kinds := []SourceKind{SourceOrgan, SourcePhaser, SourceNoise, SourceTriangle}
		for index, kind := range kinds {
			id := start(t, m, Request{
				Kind: kind, Step: uint32(68583572 + index*9699181),
				Volume: uint16(80 + index*32), Music: true,
			})
			voice, _ := m.Voice(id)
			if index < 3 && !m.SetVoiceVolumeRamp(
				id, voice.Started, uint16(index*32), uint16(80+index*32), uint32(3+index*37),
			) {
				t.Fatalf("configure ramp %d failed", index)
			}
		}
		return m
	}
	byteMixer := fixture()
	wordMixer := fixture()
	want := mix(t, byteMixer, 260)
	words := make([]uint32, 65)
	wordMixer.MixMonoWords(words)
	got := make([]int8, len(want))
	for sample := range got {
		got[sample] = int8(uint8(words[sample/4] >> uint((sample&3)*8)))
	}
	requireSamples(t, got, want)
}

func TestMixStereoCenteredOutputMatchesMonoWithRampsAndPCM(t *testing.T) {
	fixture := func() *Mixer {
		m := newMixer(t, 4)
		id := start(t, m, Request{
			Kind: SourceSquare, Step: 1 << 31, Volume: 192, Music: true,
		})
		voice, _ := m.Voice(id)
		if !m.SetVoiceVolumeRamp(id, voice.Started, 64, 192, 8) ||
			!m.SetMusicVolume(64) || !m.RampMusicVolume(MaxVolume, 10) {
			t.Fatal("configure centered stereo fixture failed")
		}
		start(t, m, Request{
			Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume,
			PCM: []int8{10, -20, 30, -40, 50, -60},
		})
		return m
	}

	monoMixer := fixture()
	stereoMixer := fixture()
	mono := make([]int8, 16)
	left := make([]int8, len(mono))
	right := make([]int8, len(mono))
	monoMixer.MixMono(mono)
	stereoMixer.MixStereo(left, right)
	requireSamples(t, left, mono)
	requireSamples(t, right, mono)
	if got, want := stereoMixer.Stats(), monoMixer.Stats(); got != want {
		t.Fatalf("stereo state telemetry = %+v, want mono %+v", got, want)
	}
}

func TestMixStereoAllMusicGainPathMatchesMonoAndAppliesPan(t *testing.T) {
	fixture := func(pan int16) *Mixer {
		m := newMixer(t, 4)
		start(t, m, Request{
			Kind: SourceSquare, Step: 1, Volume: MaxVolume, Pan: pan, Music: true,
		})
		if !m.SetMusicVolume(0) || !m.RampMusicVolume(MaxVolume, 8) {
			t.Fatal("configure all-music gain fixture failed")
		}
		return m
	}

	monoMixer := fixture(MinPan)
	stereoMixer := fixture(MinPan)
	mono := make([]int8, 12)
	left := make([]int8, len(mono))
	right := make([]int8, len(mono))
	monoMixer.MixMono(mono)
	stereoMixer.MixStereo(left, right)
	for index := range mono {
		if left[index] != mono[index] || right[index] != 0 {
			t.Fatalf("sample %d stereo=(%d,%d), want hard-left mono %d", index, left[index], right[index], mono[index])
		}
	}
}

func TestMixStereoLinearPanLaw(t *testing.T) {
	tests := []struct {
		pan         int16
		left, right int8
	}{
		{pan: MinPan, left: 100, right: 0},
		{pan: -128, left: 100, right: 50},
		{pan: 0, left: 100, right: 100},
		{pan: 128, left: 50, right: 100},
		{pan: MaxPan, left: 0, right: 100},
	}
	for _, test := range tests {
		m := newMixer(t, 4)
		start(t, m, Request{
			Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, Pan: test.pan,
			PCM: []int8{100},
		})
		left, right := []int8{0}, []int8{0}
		m.MixStereo(left, right)
		if left[0] != test.left || right[0] != test.right {
			t.Errorf("pan %d = (%d,%d), want (%d,%d)", test.pan, left[0], right[0], test.left, test.right)
		}
	}
}

func TestMixStereoSaturatesChannelsIndependently(t *testing.T) {
	m := newMixer(t, 4)
	for index := 0; index < 2; index++ {
		start(t, m, Request{
			Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, Pan: MinPan,
			PCM: []int8{100},
		})
		start(t, m, Request{
			Kind: SourcePCM, Step: 1 << 16, Volume: MaxVolume, Pan: MaxPan,
			PCM: []int8{-100},
		})
	}
	left, right := []int8{0}, []int8{0}
	m.MixStereo(left, right)
	if left[0] != 127 || right[0] != -128 {
		t.Fatalf("independent saturation = (%d,%d), want (127,-128)", left[0], right[0])
	}
}

func TestMixStereoWordsUsesFIFOByteOrder(t *testing.T) {
	fixture := func() *Mixer {
		m := newMixer(t, 4)
		start(t, m, Request{
			Kind: SourceSquare, Step: 1 << 31, Volume: MaxVolume, Pan: MinPan,
		})
		return m
	}
	byteMixer := fixture()
	wordMixer := fixture()
	leftBytes, rightBytes := make([]int8, 4), make([]int8, 4)
	byteMixer.MixStereo(leftBytes, rightBytes)
	leftWords, rightWords := []uint32{0}, []uint32{0}
	wordMixer.MixStereoWords(leftWords, rightWords)
	wantLeft := uint32(uint8(leftBytes[0])) |
		uint32(uint8(leftBytes[1]))<<8 |
		uint32(uint8(leftBytes[2]))<<16 |
		uint32(uint8(leftBytes[3]))<<24
	wantRight := uint32(uint8(rightBytes[0])) |
		uint32(uint8(rightBytes[1]))<<8 |
		uint32(uint8(rightBytes[2]))<<16 |
		uint32(uint8(rightBytes[3]))<<24
	if leftWords[0] != wantLeft || rightWords[0] != wantRight {
		t.Fatalf("stereo words = (%#08x,%#08x), want (%#08x,%#08x)", leftWords[0], rightWords[0], wantLeft, wantRight)
	}
}

func TestMixStereoClearsUnmatchedAndNilTails(t *testing.T) {
	m := newMixer(t, 4)
	start(t, m, Request{Kind: SourceSquare, Step: 1 << 31, Volume: MaxVolume})
	left, right := []int8{9, 9}, []int8{9}
	m.MixStereo(left, right)
	requireSamples(t, left, []int8{127, 0})
	requireSamples(t, right, []int8{127})

	var nilMixer *Mixer
	left, right = []int8{9, 9}, []int8{9, 9, 9}
	nilMixer.MixStereo(left, right)
	requireSamples(t, left, []int8{0, 0})
	requireSamples(t, right, []int8{0, 0, 0})
}

func TestMixerHotPathAllocatesNothing(t *testing.T) {
	m := newMixer(t, 8)
	for i := 0; i < 8; i++ {
		start(t, m, Request{Kind: SourceTriangle, Step: uint32(1<<24 + i), Volume: 32})
	}
	mono := make([]int8, 256)
	words := make([]uint32, 64)
	left := make([]int8, 256)
	right := make([]int8, 256)
	leftWords := make([]uint32, 64)
	rightWords := make([]uint32, 64)
	if got := testing.AllocsPerRun(100, func() {
		m.MixMono(mono)
		m.MixMonoWords(words)
		m.MixStereo(left, right)
		m.MixStereoWords(leftWords, rightWords)
	}); got != 0 {
		t.Fatalf("mix hot path allocations = %v, want 0", got)
	}
}

func TestVoiceStateSizeIsBounded(t *testing.T) {
	if got := unsafe.Sizeof(Voice{}); got > 128 {
		t.Fatalf("Voice is %d bytes on host, exceeds 128-byte design guard", got)
	}
}
