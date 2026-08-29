package gbapu

import (
	"math"
	"testing"

	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func rendererPulseSound(route hw.SoundRoute) runtimeaudio.Sound {
	return runtimeaudio.Sound{Kind: runtimeaudio.SoundCH1, Pulse1: hw.Pulse1Config{
		Pulse: hw.PulseConfig{
			Duty: hw.PulseDuty50, Envelope: hw.HardwareEnvelope{Initial: 15}, Route: route,
		},
	}}
}

func TestRendererProducesExpectedPulsePitch(t *testing.T) {
	renderer := NewRenderer(48_000)
	sound := rendererPulseSound(hw.SoundRouteBoth)
	if !renderer.Start(1, 60, sound) {
		t.Fatal("CH1 start rejected")
	}
	left, right := make([]int16, 48_000), make([]int16, 48_000)
	renderer.Render(left, right)

	transitions := 0
	previousPositive := left[2_000] >= 0
	for _, sample := range left[2_001:46_000] {
		positive := sample >= 0
		if positive != previousPositive {
			transitions++
			previousPositive = positive
		}
	}
	measured := float64(transitions) / 2 / (44_000.0 / 48_000.0)
	want := 261.625565
	if math.Abs(measured-want) > 1.5 {
		t.Fatalf("CH1 C4 frequency = %.3f Hz, want %.3f", measured, want)
	}
	for index := range left {
		if left[index] != right[index] {
			t.Fatalf("both-routed sample %d = %d/%d", index, left[index], right[index])
		}
	}
}

func TestRendererHonorsRoutingAndLengthCounter(t *testing.T) {
	renderer := NewRenderer(48_000)
	sound := rendererPulseSound(hw.SoundRouteLeft)
	sound.Pulse1.Pulse.UseLength = true
	sound.Pulse1.Pulse.Length = 63
	if !renderer.Start(1, 69, sound) {
		t.Fatal("CH1 start rejected")
	}
	left, right := make([]int16, 1024), make([]int16, 1024)
	renderer.Render(left, right)
	if renderer.Active(1) {
		t.Fatal("one-tick hardware length remained active")
	}
	leftNonzero := false
	for index := range left {
		leftNonzero = leftNonzero || left[index] != 0
		if right[index] != 0 {
			t.Fatalf("left-routed voice leaked right at sample %d: %d", index, right[index])
		}
	}
	if !leftNonzero {
		t.Fatal("short hardware-length voice rendered silence")
	}
}

func TestRendererWaveAndNoiseAreAudible(t *testing.T) {
	wave := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH3, Wave: hw.WaveConfig{
		Wave: hw.Waveform{Length: 32}, Level: hw.Wave100, Route: hw.SoundRouteBoth,
	}}
	for index := 0; index < 32; index++ {
		wave.Wave.Wave.Samples[index] = uint8(index / 2)
	}
	noise := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH4, Noise: hw.NoiseConfig{
		Divisor: 3, Shift: 4, Width: hw.Noise15Bit,
		Envelope: hw.HardwareEnvelope{Initial: 12, Pace: 2}, Route: hw.SoundRouteBoth,
	}}
	for channel, sound := range []runtimeaudio.Sound{wave, noise} {
		renderer := NewRenderer(48_000)
		physical := uint8(channel + 3)
		if !renderer.Start(physical, 60, sound) {
			t.Fatalf("CH%d start rejected", physical)
		}
		left, right := make([]int16, 4096), make([]int16, 4096)
		renderer.Render(left, right)
		peak := int16(0)
		for _, sample := range left {
			if sample < 0 {
				sample = -sample
			}
			peak = max(peak, sample)
		}
		if peak == 0 {
			t.Fatalf("CH%d renderer is silent", physical)
		}
	}
}

func TestRendererCallbackPathAllocatesNothing(t *testing.T) {
	renderer := NewRenderer(48_000)
	if !renderer.Start(1, 60, rendererPulseSound(hw.SoundRouteBoth)) {
		t.Fatal("CH1 start rejected")
	}
	left, right := make([]int16, 64), make([]int16, 64)
	renderer.Render(left, right)
	if allocations := testing.AllocsPerRun(100, func() {
		renderer.Render(left, right)
	}); allocations != 0 {
		t.Fatalf("Renderer.Render allocations = %g", allocations)
	}
}

func TestRendererPreservesPulsePhaseAndHonorsWaveStartBank(t *testing.T) {
	renderer := NewRenderer(48_000)
	pulse := rendererPulseSound(hw.SoundRouteBoth)
	if !renderer.Start(1, 60, pulse) {
		t.Fatal("CH1 start rejected")
	}
	left, right := make([]int16, 137), make([]int16, 137)
	renderer.Render(left, right)
	phase := renderer.channels[0].phase
	if !renderer.Start(1, 72, pulse) {
		t.Fatal("CH1 retrigger rejected")
	}
	if renderer.channels[0].phase != phase {
		t.Fatalf("CH1 retrigger reset duty phase %d -> %d", phase, renderer.channels[0].phase)
	}

	wave := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH3, Wave: hw.WaveConfig{
		Wave: hw.Waveform{Length: 64}, StartBank: 1, Level: hw.Wave100, Route: hw.SoundRouteBoth,
	}}
	if !renderer.Start(3, 60, wave) {
		t.Fatal("CH3 start rejected")
	}
	if renderer.channels[2].phase != 32 {
		t.Fatalf("CH3 bank-1 phase = %d, want 32", renderer.channels[2].phase)
	}
}

func TestRendererMonitorReleaseUsesShortDeClickTail(t *testing.T) {
	renderer := NewRenderer(48_000)
	if !renderer.Start(1, 60, rendererPulseSound(hw.SoundRouteBoth)) {
		t.Fatal("CH1 start rejected")
	}
	left, right := make([]int16, 137), make([]int16, 137)
	renderer.Render(left, right)

	const releaseFrames = 240 // 5 ms at 48 kHz.
	renderer.Release(1, releaseFrames)
	left, right = make([]int16, releaseFrames), make([]int16, releaseFrames)
	renderer.Render(left, right)
	if renderer.Active(1) {
		t.Fatal("monitor release did not finish")
	}
	if renderer.channels[0].monitorGain != 0 {
		t.Fatalf("release gain = %d, want 0", renderer.channels[0].monitorGain)
	}
	audibleTail := false
	for _, sample := range left[32 : releaseFrames/2] {
		if sample != 0 {
			audibleTail = true
			break
		}
	}
	if !audibleTail {
		t.Fatal("monitor note-off was still an immediate hard cut")
	}

	left, right = make([]int16, 128), make([]int16, 128)
	renderer.Render(left, right)
	for index := 64; index < len(left); index++ {
		if left[index] != 0 || right[index] != 0 {
			t.Fatalf("release retained PCM at frame %d: %d/%d", index, left[index], right[index])
		}
	}
}

func TestRendererNR43ShiftFourteenHoldsNoiseLFSR(t *testing.T) {
	renderer := NewRenderer(48_000)
	noise := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH4, Noise: hw.NoiseConfig{
		Divisor: 1, Shift: 14, Envelope: hw.HardwareEnvelope{Initial: 12}, Route: hw.SoundRouteBoth,
	}}
	if !renderer.Start(4, 60, noise) {
		t.Fatal("CH4 start rejected")
	}
	left, right := make([]int16, 48_000), make([]int16, 48_000)
	renderer.Render(left, right)
	if renderer.channels[3].lfsr != 0 {
		t.Fatalf("NR43 shift 14 clocked LFSR to %#04x", renderer.channels[3].lfsr)
	}
}

func BenchmarkRendererFourChannels480Frames(b *testing.B) {
	renderer := NewRenderer(48_000)
	pulse1 := rendererPulseSound(hw.SoundRouteBoth)
	pulse2 := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH2, Pulse2: hw.PulseConfig{
		Duty: hw.PulseDuty25, Envelope: hw.HardwareEnvelope{Initial: 12}, Route: hw.SoundRouteBoth,
	}}
	wave := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH3, Wave: hw.WaveConfig{
		Wave: hw.Waveform{Length: 32}, Level: hw.Wave100, Route: hw.SoundRouteBoth,
	}}
	for index := range 32 {
		wave.Wave.Wave.Samples[index] = uint8(index / 2)
	}
	noise := runtimeaudio.Sound{Kind: runtimeaudio.SoundCH4, Noise: hw.NoiseConfig{
		Divisor: 3, Shift: 4, Width: hw.Noise15Bit,
		Envelope: hw.HardwareEnvelope{Initial: 10}, Route: hw.SoundRouteBoth,
	}}
	for channel, sound := range []runtimeaudio.Sound{pulse1, pulse2, wave, noise} {
		if !renderer.Start(uint8(channel+1), uint8(60+channel*5), sound) {
			b.Fatalf("CH%d start rejected", channel+1)
		}
	}
	left, right := make([]int16, 480), make([]int16, 480)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		renderer.Render(left, right)
	}
}
