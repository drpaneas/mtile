package main

import (
	"io"
	"testing"
	"time"
)

func TestEditorPlaybackBufferSurvivesOrdinaryFrameAndGCPauses(t *testing.T) {
	if editorPlaybackBufferSize < 250*time.Millisecond {
		t.Fatalf("editor playback buffer = %s, want at least 250ms", editorPlaybackBufferSize)
	}
	if editorPlaybackSampleRate != 48_000 {
		t.Fatalf("editor playback sample rate = %d, want 48000", editorPlaybackSampleRate)
	}
}

func TestEditorPlaybackResamplesTargetPCMToHostRate(t *testing.T) {
	const targetFrames = 256
	pcm := targetMonoToHostStereo(make([]int8, targetFrames))
	host, err := io.ReadAll(resampleEditorPCM(pcm))
	if err != nil {
		t.Fatal(err)
	}
	want := targetFrames * editorPlaybackSampleRate / 16_384 * 4
	if len(host) != want {
		t.Fatalf("resampled host bytes = %d, want %d", len(host), want)
	}
}

func TestTargetMonoToHostStereoPreservesSignedAmplitude(t *testing.T) {
	got := targetMonoToHostStereo([]int8{-128, -1, 0, 1, 127})
	want := []byte{
		0x00, 0x80, 0x00, 0x80,
		0x00, 0xff, 0x00, 0xff,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x01,
		0x00, 0x7f, 0x00, 0x7f,
	}
	if len(got) != len(want) {
		t.Fatalf("host PCM length = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("host PCM byte %d = %#02x, want %#02x", index, got[index], want[index])
		}
	}
}

func TestTargetStereoToHostStereoPreservesIndependentChannels(t *testing.T) {
	got := targetStereoToHostStereo([]int8{-128, 64}, []int8{127, -32})
	want := []byte{0x00, 0x80, 0x00, 0x7f, 0x00, 0x40, 0x00, 0xe0}
	if len(got) != len(want) {
		t.Fatalf("stereo PCM length = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("stereo PCM byte %d = %#02x, want %#02x", index, got[index], want[index])
		}
	}
}
