package editor

import (
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func musicPreviewFixture(loop bool) (assetsync.SFXLibrary, assetsync.MusicLibrary) {
	sfx := assetsync.SFXLibrary{Version: assetsync.SFXSchemaVersion, SFX: []assetsync.SFXAsset{
		{ID: 0, Name: "square", Speed: 1, Steps: []assetsync.SFXStep{{Pitch: 36, Waveform: "square", Volume: 7, Effect: "none"}}},
		{ID: 1, Name: "triangle", Speed: 1, Steps: []assetsync.SFXStep{{Pitch: 40, Waveform: "triangle", Volume: 7, Effect: "none"}}},
	}}
	next0, next1 := 1, assetsync.MusicStopPattern
	if loop {
		next1 = 0
	}
	music := assetsync.MusicLibrary{Version: assetsync.MusicSchemaVersion, Patterns: []assetsync.MusicPattern{
		{ID: 0, Name: "a", Channels: []int{0, -1, -1, -1}, Next: &next0},
		{ID: 1, Name: "b", Channels: []int{1, -1, -1, -1}, Next: &next1},
	}}
	return sfx, music
}

func TestRenderMusicPreviewUsesExactPatternTimelineAndLoopCap(t *testing.T) {
	sfx, music := musicPreviewFixture(true)
	preview, err := RenderMusicPreview(sfx, music, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Samples) != MusicPreviewMaxSamples || len(preview.Segments) != (MusicPreviewMaxSamples+135)/136 {
		t.Fatalf("loop preview samples=%d segments=%d", len(preview.Samples), len(preview.Segments))
	}
	for sample, want := range map[int]int{0: 0, 135: 0, 136: 1, 271: 1, 272: 0} {
		if got := preview.PatternAtSample(sample); got != want {
			t.Errorf("PatternAtSample(%d) = %d, want %d", sample, got, want)
		}
	}
}

func TestRenderMusicPreviewStopsAndSupportsHostMute(t *testing.T) {
	sfx, music := musicPreviewFixture(false)
	full, err := RenderMusicPreview(sfx, music, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Samples) != 272 || full.PatternAtSample(271) != 1 {
		t.Fatalf("finite preview samples=%d tail pattern=%d", len(full.Samples), full.PatternAtSample(271))
	}
	muted, err := RenderMusicPreview(sfx, music, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	for index, sample := range muted.Samples {
		if sample != 0 {
			t.Fatalf("muted sample %d = %d", index, sample)
		}
	}
	if full.Samples[0] != 0 || !slices.ContainsFunc(full.Samples[:48], func(sample int8) bool { return sample != 0 }) {
		t.Fatal("unmuted preview does not contain the expected anti-click attack")
	}
}

func TestRenderMusicPreviewPlaysFiniteSongPastLoopCap(t *testing.T) {
	sfx, music := musicPreviewFixture(false)
	for effect := range sfx.SFX {
		sfx.SFX[effect].Speed = 20
		step := sfx.SFX[effect].Steps[0]
		for len(sfx.SFX[effect].Steps) < assetsync.MaxSFXSteps {
			sfx.SFX[effect].Steps = append(sfx.SFX[effect].Steps, step)
		}
	}
	preview, err := RenderMusicPreview(sfx, music, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Samples) <= MusicPreviewMaxSamples {
		t.Fatalf("finite preview samples = %d, want past cyclic cap %d", len(preview.Samples), MusicPreviewMaxSamples)
	}
	if len(preview.Segments) != 2 || preview.PatternAtSample(len(preview.Samples)-1) != 1 {
		t.Fatalf("finite preview segments=%#v tail=%d", preview.Segments, preview.PatternAtSample(len(preview.Samples)-1))
	}
}

func TestRenderMusicPreviewRejectsMissingEntry(t *testing.T) {
	sfx, music := musicPreviewFixture(false)
	if _, err := RenderMusicPreview(sfx, music, 9, 0); err == nil {
		t.Fatal("missing music entry preview succeeded")
	}
}

func TestRenderMusicPreviewLimitCanRenderPastInteractiveCap(t *testing.T) {
	sfx, music := musicPreviewFixture(true)
	limit := MusicPreviewMaxSamples + 123
	preview, err := RenderMusicPreviewLimit(sfx, music, 0, 0, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Samples) != limit {
		t.Fatalf("limited preview samples = %d, want %d", len(preview.Samples), limit)
	}
	if _, err := RenderMusicPreviewLimit(sfx, music, 0, 0, MusicRenderMaxSamples+1); err == nil {
		t.Fatal("oversized music render limit accepted")
	}
}

func TestPackedMusicPreviewMatchesBytePreview(t *testing.T) {
	sfx, music := musicPreviewFixture(true)
	limit := MusicPreviewMaxSamples + 123
	bytes, err := RenderMusicPreviewLimit(sfx, music, 0, 0, limit)
	if err != nil {
		t.Fatal(err)
	}
	words, err := RenderMusicPreviewWordsLimit(sfx, music, 0, 0, limit)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(bytes.Samples, words.Samples) {
		t.Fatal("packed music preview differs from byte preview")
	}
}

func TestRenderMusicPreviewFadeUsesTargetMusicGainClock(t *testing.T) {
	sfx, music := musicPreviewFixture(false)
	full, err := RenderMusicPreview(sfx, music, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	faded, err := RenderMusicPreviewFade(sfx, music, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if faded.FadeMS != 1 || faded.Samples[0] != 0 || faded.Samples[20] != full.Samples[20] {
		t.Fatalf("faded preview metadata/sample boundary = %d/%d/%d, full=%d", faded.FadeMS, faded.Samples[0], faded.Samples[20], full.Samples[20])
	}
	if _, err := RenderMusicPreviewFade(sfx, music, 0, 0, -1); err == nil {
		t.Fatal("negative preview fade succeeded")
	}
}
