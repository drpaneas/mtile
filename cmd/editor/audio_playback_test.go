package main

import (
	"io"
	"runtime"
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

func TestLowLatencyBuffersCoverOneHostMixChunk(t *testing.T) {
	if editorDeviceBufferSize > 48*time.Millisecond {
		t.Fatalf("device buffer = %s, want no more than 48ms for live MIDI", editorDeviceBufferSize)
	}
	liveBuffer := nativeLiveBufferSize()
	minimum := editorDeviceBufferSize
	switch runtime.GOOS {
	case "darwin":
		minimum = editorDeviceBufferSize / 2
	case "linux":
		minimum = editorDeviceBufferSize / 2
	}
	if liveBuffer < minimum {
		t.Fatalf("native live buffer = %s, want at least %s of host coverage", liveBuffer, minimum)
	}
	if runtime.GOOS == "darwin" && liveBuffer > 22*time.Millisecond {
		t.Fatalf("native live buffer = %s, want at most 22ms for playable latency", liveBuffer)
	}
}

func TestEditorAudioByteTimeConversions(t *testing.T) {
	bytes := editorAudioBufferBytes(time.Second, editorPlaybackSampleRate)
	if bytes != editorPlaybackSampleRate*editorAudioBytesPerFrame {
		t.Fatalf("one-second buffer = %d bytes, want %d", bytes, editorPlaybackSampleRate*editorAudioBytesPerFrame)
	}
	got := editorAudioDuration(int64(bytes), editorPlaybackSampleRate)
	if got != time.Second {
		t.Fatalf("one-second byte duration = %s, want 1s", got)
	}
}

func TestEditorPlaybackResamplesTargetPCMToHostRate(t *testing.T) {
	const targetFrames = 256
	pcm := targetMonoToHostStereo(make([]int8, targetFrames))
	host, err := io.ReadAll(resampleEditorPCMAtRate(pcm, 32_768))
	if err != nil {
		t.Fatal(err)
	}
	want := targetFrames * editorPlaybackSampleRate / 32_768 * 4
	if len(host) != want {
		t.Fatalf("resampled host bytes = %d, want %d", len(host), want)
	}
}

func TestEditorPlaybackKeepsNativePSGAtHostRate(t *testing.T) {
	const nativeFrames = 256
	pcm := targetMonoToHostStereo(make([]int8, nativeFrames))
	host, err := io.ReadAll(resampleEditorPCMAtRate(pcm, 48_000))
	if err != nil {
		t.Fatal(err)
	}
	if len(host) != len(pcm) {
		t.Fatalf("native host bytes = %d, want %d", len(host), len(pcm))
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
