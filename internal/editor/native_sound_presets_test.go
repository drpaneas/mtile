package editor

import (
	"math"
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
)

func TestNativeSoundPresetsMatchPhysicalChannelCapabilities(t *testing.T) {
	wantCounts := [...]int{14, 10, 14, 14}
	for channel, want := range wantCounts {
		presets := NativeSoundPresets(channel)
		if len(presets) != want {
			t.Fatalf("CH%d presets = %d, want %d", channel+1, len(presets), want)
		}
		for index, preset := range presets {
			if preset.Name == "" || preset.Sound.Name != preset.Name || preset.Sound.Kind != nativeKindName(channel) {
				t.Fatalf("CH%d preset %d = %#v", channel+1, index, preset)
			}
			switch channel {
			case 0:
				if preset.Sound.Pulse == nil || preset.Sound.Pulse.Sweep == nil {
					t.Fatalf("CH1 preset %q lacks pulse+sweep state", preset.Name)
				}
			case 1:
				if preset.Sound.Pulse == nil || preset.Sound.Pulse.Sweep != nil {
					t.Fatalf("CH2 preset %q exposes unsupported sweep state", preset.Name)
				}
			case 2:
				if preset.Sound.Wave == nil || len(preset.Sound.Wave.Samples) != 32 {
					t.Fatalf("CH3 preset %q waveform = %#v", preset.Name, preset.Sound.Wave)
				}
			case 3:
				if preset.Sound.Noise == nil {
					t.Fatalf("CH4 preset %q lacks noise state", preset.Name)
				}
			}
		}
	}

	ch1 := NativeSoundPresets(0)
	if got := ch1[0].Sound.Pulse; got.Duty != 25 || got.Envelope.Pace != 5 || got.Envelope.Direction != "down" {
		t.Fatalf("fade-out pulse = %#v", got)
	}
	if got := ch1[10].Sound.Pulse; got.Sweep.Pace != 1 || got.Sweep.Direction != "down" || got.Sweep.Shift != 4 {
		t.Fatalf("bass-drum sweep = %#v", got.Sweep)
	}
	if got := ch1[9].Sound.Macro; got == nil || len(got.Steps) != 8 || got.LoopStart == nil || *got.LoopStart != 0 ||
		got.Steps[0].PeriodDelta != 1 || got.Steps[3].PeriodDelta != 1 || got.Steps[4].PeriodDelta != -1 ||
		got.Steps[7].PeriodDelta != -1 {
		t.Fatalf("custom vibrato macro = %#v", got)
	}
	ch3 := NativeSoundPresets(2)
	if got := ch3[10].Sound.Wave.Samples; got[0] != 15 || got[15] != 0 || got[31] != 15 {
		t.Fatalf("triangle wave = %#v", got)
	}
	ch4 := NativeSoundPresets(3)
	if got := ch4[1].Sound.Noise; got.Shift != 8 || got.Divisor != 7 || got.Length != 32 || !got.UseLength {
		t.Fatalf("closed hi-hat 2 = %#v", got)
	}
	if got := ch4[11].Sound.Macro; got == nil || len(got.Steps) != 6 || got.LoopStart == nil || *got.LoopStart != 5 ||
		!got.Steps[0].HasNoiseNote || got.Steps[0].NoiseNoteOffset != 27 ||
		!got.Steps[3].HasNoiseNote || got.Steps[3].NoiseNoteOffset != -2 {
		t.Fatalf("bass drum 2 macro = %#v", got)
	}
}

func TestNativeSoundPresetsAreDetachedAndReusedByHardwareState(t *testing.T) {
	first := NativeSoundPresets(2)
	first[0].Sound.Wave.Samples[0] = 3
	if got := NativeSoundPresets(2)[0].Sound.Wave.Samples[0]; got != 15 {
		t.Fatalf("preset catalog mutated through caller: sample=%d", got)
	}

	m := nativeEditorFixture(t)
	id, created, err := m.EnsureNativeSoundPreset(2, 10)
	if err != nil || !created {
		t.Fatalf("first ensure = id %d created %t err %v", id, created, err)
	}
	again, created, err := m.EnsureNativeSoundPreset(2, 10)
	if err != nil || created || again != id {
		t.Fatalf("second ensure = id %d created %t err %v, want reused %d", again, created, err, id)
	}
	sound, ok := m.NativeSoundByID(id)
	if !ok || NativeSoundPresetIndex(2, sound) != 10 {
		t.Fatalf("materialized preset = %#v ok=%t", sound, ok)
	}
	if err := m.RenameNativeSound(id, "My triangle"); err != nil {
		t.Fatal(err)
	}
	sound, _ = m.NativeSoundByID(id)
	if NativeSoundPresetIndex(2, sound) != 10 {
		t.Fatal("renaming hid the preset identity")
	}
}

func TestEveryNativeSoundPresetCompilesAndRendersAnAudition(t *testing.T) {
	m := nativeEditorFixture(t)
	for channel := 0; channel < 4; channel++ {
		for index, preset := range NativeSoundPresets(channel) {
			id, _, err := m.EnsureNativeSoundPreset(channel, index)
			if err != nil {
				t.Fatalf("CH%d preset %q did not compile: %v", channel+1, preset.Name, err)
			}
			note := 60
			if channel == 2 {
				note = 48
			}
			left, right, err := RenderNativeNotePreviewStereo(m.NativeAudio.Library, id, note, m.NativeAudio.Settings)
			if err != nil || len(left) == 0 || len(left) != len(right) {
				t.Fatalf("CH%d preset %q audition = %d/%d samples, err %v", channel+1, preset.Name, len(left), len(right), err)
			}
			audible := false
			for sample := range left {
				if left[sample] != 0 || right[sample] != 0 {
					audible = true
					break
				}
			}
			if !audible {
				t.Fatalf("CH%d preset %q rendered silence", channel+1, preset.Name)
			}
		}
	}
}

func TestGBStudioNoisePresetMacrosMatchHUGEDriverNR43Bytes(t *testing.T) {
	// Golden bytes were independently derived from template.uge's table notes
	// with hUGEDriver's get_note_poly routine. Do not compute these through the
	// production converter: this test is intended to catch that exact class of
	// conversion regression.
	want := [][]uint8{
		{0x07, 0x07},
		{0x14, 0x14},
		{0x15, 0x16, 0x17, 0x17},
		{0x07, 0x07},
		{0x27, 0x27},
		{0x47, 0x47},
		{0x27, 0x27},
		{0x27, 0x27},
		{0x47, 0x54},
		{0x07, 0x07},
		{0x47, 0x54},
		{0x24, 0x55, 0x46, 0x95, 0x87, 0x87},
		{0x56, 0x64, 0x66, 0x74, 0x74, 0x74},
		{0x67, 0x74, 0x75, 0x76, 0x77, 0x77},
	}
	presets := NativeSoundPresets(3)
	if len(presets) != len(want) {
		t.Fatalf("noise presets = %d, want %d", len(presets), len(want))
	}
	baseDivisor, baseShift, baseOK := audio.NativeGBStudioNoisePolynomial(60, 0)
	if !baseOK {
		t.Fatal("GB Studio C5 base trigger polynomial is invalid")
	}
	wantBase := baseShift<<4 | baseDivisor
	sevenBit := map[int]bool{7: true, 8: true, 9: true, 13: true}
	for presetIndex, preset := range presets {
		if preset.Sound.Macro == nil || len(preset.Sound.Macro.Steps) != len(want[presetIndex]) {
			t.Fatalf("%q macro = %#v, want %d rows", preset.Name, preset.Sound.Macro, len(want[presetIndex]))
		}
		for row, step := range preset.Sound.Macro.Steps {
			divisor, shift, ok := audio.NativeGBStudioNoisePolynomial(60, int8(step.NoiseNoteOffset))
			got := shift<<4 | divisor
			if step.HasNoise || !step.HasNoiseNote || !ok || got != want[presetIndex][row] {
				t.Fatalf("%q row %d NR43 = %#02x relative=%t valid=%t, want %#02x", preset.Name, row, got, step.HasNoiseNote, ok, want[presetIndex][row])
			}
		}
		initial := uint8(preset.Sound.Noise.Shift<<4 | preset.Sound.Noise.Divisor)
		if initial != wantBase {
			t.Fatalf("%q base-trigger NR43 = %#02x, want C5 %#02x", preset.Name, initial, wantBase)
		}
		if got := preset.Sound.Noise.Width == 7; got != sevenBit[presetIndex] {
			t.Fatalf("%q 7-bit counter = %t, want %t", preset.Name, got, sevenBit[presetIndex])
		}
	}
}

func TestNativePresetAuditionRemovesAsymmetricPulseDC(t *testing.T) {
	m := nativeEditorFixture(t)
	id, _, err := m.EnsureNativeSoundPreset(0, 6) // sustained 12.5% pulse
	if err != nil {
		t.Fatal(err)
	}
	samples, err := RenderNativeNotePreview(m.NativeAudio.Library, id, 60)
	if err != nil {
		t.Fatal(err)
	}
	start := len(samples) / 2
	sum := 0
	for _, sample := range samples[start:] {
		sum += int(sample)
	}
	mean := math.Abs(float64(sum) / float64(len(samples)-start))
	if mean > 1 {
		t.Fatalf("settled 12.5%% pulse DC mean = %.3f, want <= 1", mean)
	}
}

func TestNativePresetAuditionPreservesHardwareAttack(t *testing.T) {
	m := nativeEditorFixture(t)
	id, _, err := m.EnsureNativeSoundPreset(0, 6) // sustained 12.5% pulse
	if err != nil {
		t.Fatal(err)
	}
	samples, err := RenderNativeNotePreview(m.NativeAudio.Library, id, 60)
	if err != nil {
		t.Fatal(err)
	}
	firstNonzero := -1
	for index, sample := range samples[:NativePreviewSampleRate/1000] {
		if sample != 0 {
			firstNonzero = index
			break
		}
	}
	if firstNonzero < 0 {
		t.Fatal("audition suppressed the hardware attack for longer than one millisecond")
	}
}
