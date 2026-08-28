package main

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
	ebitaudio "github.com/hajimehoshi/ebiten/v2/audio"
)

// Keep enough decoded PCM queued to survive an expensive editor frame or a
// Go GC pause. The preview is fully rendered before playback, so a small
// low-latency buffer would only increase underrun risk.
const editorPlaybackBufferSize = 500 * time.Millisecond

// Desktop playback uses a conventional host-device rate. GBA synthesis and
// analysis stay at the target rate; only the final monitor stream is resampled.
const editorPlaybackSampleRate = 48_000

func editorAudioContext() (*ebitaudio.Context, error) {
	context := ebitaudio.CurrentContext()
	if context == nil {
		return ebitaudio.NewContext(editorPlaybackSampleRate), nil
	}
	if context.SampleRate() != editorPlaybackSampleRate {
		return nil, fmt.Errorf("audio context uses %d Hz, editor playback requires %d Hz", context.SampleRate(), editorPlaybackSampleRate)
	}
	return context, nil
}

func resampleEditorPCM(pcm []byte) io.Reader {
	resampled := ebitaudio.ResampleReader(
		bytes.NewReader(pcm), int64(len(pcm)),
		editor.SFXPreviewSampleRate, editorPlaybackSampleRate,
	)
	hostBytes := int64(len(pcm)) * editorPlaybackSampleRate / editor.SFXPreviewSampleRate
	hostBytes -= hostBytes % 4
	return io.LimitReader(resampled, hostBytes)
}

func newEditorAudioPlayer(context *ebitaudio.Context, pcm []byte) (*ebitaudio.Player, error) {
	player, err := context.NewPlayer(resampleEditorPCM(pcm))
	if err != nil {
		return nil, err
	}
	player.SetBufferSize(editorPlaybackBufferSize)
	return player, nil
}

func targetMonoToHostStereo(samples []int8) []byte {
	pcm := make([]byte, len(samples)*4)
	for index, sample := range samples {
		value := int16(sample) << 8
		low, high := byte(value), byte(uint16(value)>>8)
		offset := index * 4
		pcm[offset], pcm[offset+1] = low, high
		pcm[offset+2], pcm[offset+3] = low, high
	}
	return pcm
}

func targetStereoToHostStereo(left, right []int8) []byte {
	frames := min(len(left), len(right))
	pcm := make([]byte, frames*4)
	for index := 0; index < frames; index++ {
		leftValue := int16(left[index]) << 8
		rightValue := int16(right[index]) << 8
		offset := index * 4
		pcm[offset], pcm[offset+1] = byte(leftValue), byte(uint16(leftValue)>>8)
		pcm[offset+2], pcm[offset+3] = byte(rightValue), byte(uint16(rightValue)>>8)
	}
	return pcm
}

func (g *Game) stopAudioPreview() {
	if g == nil {
		return
	}
	g.nativeAudition = false
	if g.previewPlayer == nil {
		return
	}
	g.previewPlayer.Pause()
	_ = g.previewPlayer.Close()
	g.previewPlayer = nil
}
