package main

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"
	ebitaudio "github.com/hajimehoshi/ebiten/v2/audio"
)

// Keep enough decoded PCM queued to survive an expensive editor frame or a
// Go GC pause. The preview is fully rendered before playback, so a small
// low-latency buffer would only increase underrun risk.
const editorPlaybackBufferSize = 500 * time.Millisecond

// Ebitengine's audio.Context leaves Oto's host buffer at the platform
// default. On macOS that is four 32 ms AudioQueue buffers, so a MIDI note can
// wait roughly 128 ms before reaching the speakers even though the PSG stream
// renders in a fraction of a millisecond. The editor owns a low-latency Oto
// context directly so the device queue is short while cartridge audio remains
// completely unchanged.
const editorDeviceBufferSize = 40 * time.Millisecond

// Desktop PSG previews render at the host-device rate to preserve pulse
// edges and high-frequency noise.
const editorPlaybackSampleRate = 48_000

const editorAudioBytesPerFrame = 4 // signed 16-bit stereo

var (
	editorAudioOnce       sync.Once
	editorAudioContextRef *editorHostAudioContext
	editorAudioContextErr error
)

// editorHostAudioContext is deliberately small: the editor already produces
// signed 16-bit stereo PCM, and Oto only needs a single process-wide context.
// Keeping this adapter local avoids changing any Game Boy PSG rendering code.
type editorHostAudioContext struct {
	context    *oto.Context
	sampleRate int
}

func editorAudioContext() (*editorHostAudioContext, error) {
	editorAudioOnce.Do(func() {
		context, _, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   editorPlaybackSampleRate,
			ChannelCount: 2,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   editorDeviceBufferSize,
		})
		if err != nil {
			editorAudioContextErr = fmt.Errorf("create low-latency audio context: %w", err)
			return
		}
		editorAudioContextRef = &editorHostAudioContext{context: context, sampleRate: editorPlaybackSampleRate}
	})
	if editorAudioContextErr != nil {
		return nil, editorAudioContextErr
	}
	if editorAudioContextRef == nil || editorAudioContextRef.context == nil {
		return nil, fmt.Errorf("low-latency audio context is unavailable")
	}
	if err := editorAudioContextRef.context.Err(); err != nil {
		return nil, fmt.Errorf("audio device: %w", err)
	}
	return editorAudioContextRef, nil
}

// editorAudioReader tracks how many source bytes Oto has requested. Subtracting
// the bytes still buffered by the player gives a stable transport position
// without adding another queue or clock to the playback path.
type editorAudioReader struct {
	reader io.Reader
	read   atomic.Int64
}

func (r *editorAudioReader) Read(target []byte) (int, error) {
	n, err := r.reader.Read(target)
	r.read.Add(int64(n))
	return n, err
}

type editorAudioPlayer struct {
	player     *oto.Player
	reader     *editorAudioReader
	sampleRate int
	closed     atomic.Bool
}

func (c *editorHostAudioContext) NewPlayer(source io.Reader) (*editorAudioPlayer, error) {
	if c == nil || c.context == nil {
		return nil, fmt.Errorf("audio context is unavailable")
	}
	if err := c.context.Err(); err != nil {
		return nil, fmt.Errorf("audio device: %w", err)
	}
	tracked := &editorAudioReader{reader: source}
	return &editorAudioPlayer{
		player: c.context.NewPlayer(tracked), reader: tracked, sampleRate: c.sampleRate,
	}, nil
}

func (p *editorAudioPlayer) Play() {
	if p != nil && p.player != nil && !p.closed.Load() {
		p.player.Play()
	}
}

func (p *editorAudioPlayer) Pause() {
	if p != nil && p.player != nil && !p.closed.Load() {
		p.player.Pause()
	}
}

func (p *editorAudioPlayer) IsPlaying() bool {
	return p != nil && p.player != nil && !p.closed.Load() && p.player.IsPlaying()
}

func (p *editorAudioPlayer) SetVolume(volume float64) {
	if p != nil && p.player != nil && !p.closed.Load() {
		p.player.SetVolume(volume)
	}
}

func (p *editorAudioPlayer) SetBufferSize(bufferSize time.Duration) {
	if p == nil || p.player == nil || p.closed.Load() {
		return
	}
	p.player.SetBufferSize(editorAudioBufferBytes(bufferSize, p.sampleRate))
}

func (p *editorAudioPlayer) Position() time.Duration {
	if p == nil || p.player == nil || p.reader == nil || p.sampleRate <= 0 {
		return 0
	}
	playedBytes := p.reader.read.Load() - int64(p.player.BufferedSize())
	if playedBytes <= 0 {
		return 0
	}
	return editorAudioDuration(playedBytes, p.sampleRate)
}

func (p *editorAudioPlayer) Close() error {
	if p == nil || p.player == nil || p.closed.Swap(true) {
		return nil
	}
	// Oto releases resources when the player becomes unreachable. Pause first
	// so closing this adapter also stops playback immediately.
	p.player.Pause()
	return nil
}

func editorAudioBufferBytes(bufferSize time.Duration, sampleRate int) int {
	bytes := int(bufferSize * time.Duration(sampleRate*editorAudioBytesPerFrame) / time.Second)
	bytes -= bytes % editorAudioBytesPerFrame
	return max(editorAudioBytesPerFrame, bytes)
}

func editorAudioDuration(byteCount int64, sampleRate int) time.Duration {
	if byteCount <= 0 || sampleRate <= 0 {
		return 0
	}
	frames := byteCount / editorAudioBytesPerFrame
	return time.Duration(frames) * time.Second / time.Duration(sampleRate)
}

func resampleEditorPCMAtRate(pcm []byte, sourceRate int) io.Reader {
	if sourceRate <= 0 {
		sourceRate = editorPlaybackSampleRate
	}
	resampled := ebitaudio.ResampleReader(
		bytes.NewReader(pcm), int64(len(pcm)),
		sourceRate, editorPlaybackSampleRate,
	)
	hostBytes := int64(len(pcm)) * editorPlaybackSampleRate / int64(sourceRate)
	hostBytes -= hostBytes % 4
	return io.LimitReader(resampled, hostBytes)
}

func newEditorAudioPlayerAtRate(context *editorHostAudioContext, pcm []byte, sourceRate int) (*editorAudioPlayer, error) {
	player, err := context.NewPlayer(resampleEditorPCMAtRate(pcm, sourceRate))
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
