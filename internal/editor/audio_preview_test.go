package editor

import (
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
)

func previewFixture() assetsync.SFXLibrary {
	return assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 4, Name: "preview", Speed: 1, Loop: &assetsync.SFXLoop{Start: 1, End: 3},
		Steps: []assetsync.SFXStep{
			{Pitch: 57, Waveform: "square", Volume: 7, Effect: "none"},
			{Pitch: 60, Waveform: "pulse", Volume: 6, Effect: "none"},
			{Pitch: 62, Waveform: "noise", Volume: 5, Effect: "none"},
		},
	}}}
}

func TestRenderSFXPreviewSupportsEveryPICO8InstrumentAndFilter(t *testing.T) {
	waveforms := []string{"triangle", "tilted-saw", "saw", "square", "pulse", "organ", "noise", "phaser"}
	for id, waveform := range waveforms {
		library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
			ID: id, Name: waveform, Speed: 2,
			Filters: &assetsync.SFXFilters{Noiz: true, Buzz: true, Detune: 1, Reverb: 1, Dampen: 1},
			Steps:   []assetsync.SFXStep{{Pitch: 36, Waveform: waveform, Volume: 5, Effect: "none"}},
		}}}
		preview, err := RenderSFXPreview(library, id, 0, 0)
		if err != nil {
			t.Fatalf("%s preview failed: %v", waveform, err)
		}
		authored := int(audio.SFXSamplesForUnits(2))
		if len(preview.Samples) <= authored || len(preview.Samples) > authored+sfxPreviewFilterTailGuard {
			t.Fatalf("%s filtered preview length = %d (authored %d)", waveform, len(preview.Samples), authored)
		}
		if preview.Samples[len(preview.Samples)-1] == 0 {
			t.Fatalf("%s filtered preview retained trailing silence", waveform)
		}
	}
}

func TestRenderSFXPreviewKeepsOnlyMeasuredStatelessReleaseTail(t *testing.T) {
	library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 0, Name: "stateless", Speed: 2,
		Filters: &assetsync.SFXFilters{Noiz: true, Buzz: true, Detune: 2},
		Steps:   []assetsync.SFXStep{{Pitch: 36, Waveform: "saw", Volume: 5, Effect: "none"}},
	}}}
	preview, err := RenderSFXPreview(library, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	authored := int(audio.SFXSamplesForUnits(2))
	if len(preview.Samples) <= authored || len(preview.Samples) > authored+audio.SFXEdgeSamples ||
		preview.Samples[len(preview.Samples)-1] == 0 {
		t.Fatalf("stateless preview length/tail = %d/%d, authored %d", len(preview.Samples), preview.Samples[len(preview.Samples)-1], authored)
	}
}

func TestRenderSFXPreviewCarriesSlideIntoSharedRuntime(t *testing.T) {
	library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 2, Name: "slide", Speed: 4,
		Steps: []assetsync.SFXStep{
			{Pitch: 40, Waveform: "square", Volume: 7, Effect: "slide"},
			{Pitch: 63, Waveform: "square", Volume: 1, Effect: "none"},
		},
	}}}
	slide, err := RenderSFXPreview(library, 2, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	library.SFX[0].Steps[0].Effect = "none"
	plain, err := RenderSFXPreview(library, 2, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(slide.Samples[:136], plain.Samples[:136]) {
		t.Fatal("first-step slide did not begin from PICO-8's previous-note source")
	}
	if slices.Equal(slide.Samples[136:272], plain.Samples[136:272]) {
		t.Fatal("slide preview did not change target PCM after the first effect tick")
	}
}

func TestRenderSFXPreviewCarriesVibratoIntoSharedRuntime(t *testing.T) {
	library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 3, Name: "vibrato", Speed: 18,
		Steps: []assetsync.SFXStep{{Pitch: 40, Waveform: "square", Volume: 7, Effect: "vibrato"}},
	}}}
	vibrato, err := RenderSFXPreview(library, 3, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	library.SFX[0].Steps[0].Effect = "none"
	plain, err := RenderSFXPreview(library, 3, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(vibrato.Samples, plain.Samples) {
		t.Fatal("vibrato preview matches effect-free target PCM")
	}
}

func TestRenderSFXPreviewCarriesDropIntoSharedRuntime(t *testing.T) {
	library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 5, Name: "drop", Speed: 18,
		Steps: []assetsync.SFXStep{{Pitch: 48, Waveform: "square", Volume: 7, Effect: "drop"}},
	}}}
	drop, err := RenderSFXPreview(library, 5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	library.SFX[0].Steps[0].Effect = "none"
	plain, err := RenderSFXPreview(library, 5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(drop.Samples, plain.Samples) {
		t.Fatal("drop preview matches effect-free target PCM")
	}
}

func TestRenderSFXPreviewCarriesSampleContinuousFadeIn(t *testing.T) {
	library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 6, Name: "fade-in", Speed: 1,
		Steps: []assetsync.SFXStep{{Pitch: 48, Waveform: "square", Volume: 7, Effect: "fade-in"}},
	}}}
	fade, err := RenderSFXPreview(library, 6, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fade.Samples[0] != 0 || fade.Samples[135] == 0 {
		t.Fatalf("fade-in endpoints = %d,%d", fade.Samples[0], fade.Samples[135])
	}
	library.SFX[0].Steps[0].Effect = "none"
	plain, err := RenderSFXPreview(library, 6, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(fade.Samples, plain.Samples) {
		t.Fatal("fade-in preview matches effect-free target PCM")
	}
}

func TestRenderSFXPreviewCarriesSampleContinuousFadeOut(t *testing.T) {
	library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
		ID: 7, Name: "fade-out", Speed: 1,
		Steps: []assetsync.SFXStep{{Pitch: 48, Waveform: "square", Volume: 7, Effect: "fade-out"}},
	}}}
	fade, err := RenderSFXPreview(library, 7, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fade.Samples[0] != 0 || fade.Samples[47] == 0 || fade.Samples[135] < -1 || fade.Samples[135] > 1 {
		t.Fatalf("fade-out endpoints = %d,%d", fade.Samples[0], fade.Samples[135])
	}
	library.SFX[0].Steps[0].Effect = "none"
	plain, err := RenderSFXPreview(library, 7, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(fade.Samples, plain.Samples) {
		t.Fatal("fade-out preview matches effect-free target PCM")
	}
}

func TestRenderSFXPreviewCarriesFastAndSlowArpeggios(t *testing.T) {
	for _, effect := range []string{"arpeggio-fast", "arpeggio-slow"} {
		t.Run(effect, func(t *testing.T) {
			library := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{{
				ID: 8, Name: effect, Speed: 18,
				Steps: []assetsync.SFXStep{
					{Pitch: 36, Waveform: "square", Volume: 7, Effect: effect},
					{Pitch: 40, Waveform: "square", Volume: 7, Effect: effect},
					{Pitch: 43, Waveform: "square", Volume: 7, Effect: effect},
					{Pitch: 48, Waveform: "square", Volume: 7, Effect: effect},
				},
			}}}
			arpeggio, err := RenderSFXPreview(library, 8, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			for index := range library.SFX[0].Steps {
				library.SFX[0].Steps[index].Effect = "none"
			}
			plain, err := RenderSFXPreview(library, 8, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Equal(arpeggio.Samples, plain.Samples) {
				t.Fatalf("%s preview matches effect-free target PCM", effect)
			}
		})
	}
}

func TestRenderSFXPreviewUsesExactTargetClockAndLoopCursor(t *testing.T) {
	preview, err := RenderSFXPreview(previewFixture(), 4, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Samples) != SFXPreviewMaxSamples || preview.SpeedSamples != 136 {
		t.Fatalf("preview samples=%d speed=%d", len(preview.Samples), preview.SpeedSamples)
	}
	if preview.Samples[0] != 0 || preview.Samples[47] == 0 {
		t.Fatalf("anti-click target square edge = [%d,%d], want silence then audible", preview.Samples[0], preview.Samples[47])
	}
	for sample, want := range map[int]int{0: 0, 135: 0, 136: 1, 271: 1, 272: 2, 407: 2, 408: 1, 544: 2, 680: 1} {
		if got := preview.StepAtSample(sample); got != want {
			t.Errorf("StepAtSample(%d) = %d, want %d", sample, got, want)
		}
	}
}

func TestSFXAssetPlaybackPositionProvidesSmoothLoopingCursor(t *testing.T) {
	effect := previewFixture().SFX[0]
	for sample, want := range map[int]int{0: 0, 135: 0, 136: 1, 272: 2, 408: 1} {
		position, ok := SFXAssetPlaybackPosition(effect, sample)
		if !ok {
			t.Fatalf("sample %d has no playback position", sample)
		}
		if position.Step != want {
			t.Errorf("sample %d step = %d, want %d", sample, position.Step, want)
		}
		if position.StepSample < 0 || position.StepSample >= position.StepDurationSamples {
			t.Errorf("sample %d progress = %d/%d", sample, position.StepSample, position.StepDurationSamples)
		}
	}
	start, _ := SFXAssetPlaybackPosition(effect, 136)
	middle, _ := SFXAssetPlaybackPosition(effect, 200)
	if start.Step != middle.Step || start.StepSample >= middle.StepSample {
		t.Fatalf("cursor did not advance smoothly within step: start=%#v middle=%#v", start, middle)
	}
}

func TestRenderSFXPreviewAppliesProjectMasterVolume(t *testing.T) {
	library := previewFixture()
	full, err := RenderSFXPreviewVolume(library, 4, 0, 1, assetsync.AudioMasterVolumeMax)
	if err != nil {
		t.Fatal(err)
	}
	half, err := RenderSFXPreviewVolume(library, 4, 0, 1, assetsync.AudioMasterVolumeMax/2)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Samples) == 0 || len(half.Samples) != len(full.Samples) {
		t.Fatalf("preview lengths full=%d half=%d", len(full.Samples), len(half.Samples))
	}
	want := int(full.Samples[0]) / 2
	got := int(half.Samples[0])
	if got < want-1 || got > want+1 {
		t.Fatalf("half-master first sample = %d, want about %d (full %d)", got, want, full.Samples[0])
	}
	if _, err := RenderSFXPreviewVolume(library, 4, 0, 1, assetsync.AudioMasterVolumeMax+1); err == nil {
		t.Fatal("preview accepted invalid master volume")
	}
}

func TestRenderSelectedSFXStepIsFiniteAndRejectsBadRanges(t *testing.T) {
	preview, err := RenderSFXPreview(previewFixture(), 4, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Samples) != 136+audio.SFXEdgeSamples-1 || preview.LoopStart != -1 ||
		preview.StepAtSample(135) != 2 || preview.StepAtSample(136) != -1 {
		t.Fatalf("selected-step preview = %#v", preview)
	}
	for _, call := range []struct{ id, offset, length int }{{9, 0, 0}, {4, -1, 0}, {4, 3, 0}, {4, 2, 2}} {
		if _, err := RenderSFXPreview(previewFixture(), call.id, call.offset, call.length); err == nil {
			t.Fatalf("invalid preview range %+v succeeded", call)
		}
	}
}
