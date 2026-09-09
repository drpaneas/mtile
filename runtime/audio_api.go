package runtime

import "github.com/drpaneas/mtile/runtime/audio"

// AttachNativeAudio installs the four-channel PSG controller.
func (c *Context) AttachNativeAudio(controller *audio.Controller) {
	if c != nil {
		c.nativeAudio = controller
	}
}

// AudioStats is one bounded snapshot of the PSG scheduler and channels.
// Reading it does not enable profiling or change audio production.
type AudioStats struct {
	Native audio.NativeStats
}

func (c *Context) AudioStats() AudioStats {
	if c == nil {
		return AudioStats{}
	}
	var stats AudioStats
	if c.nativeAudio != nil {
		stats.Native = c.nativeAudio.Stats()
	}
	return stats
}

// SFXV plays a PSG clip with optional priority. The final operands retain
// the asset-free call ABI; project validation rejects offset/length arguments.
func (c *Context) SFXV(id, priorityValue, _, _ Value) {
	if c != nil && c.nativeAudio != nil {
		nativeID := id.Int()
		if nativeID == -1 {
			c.nativeAudio.StopSFX()
			return
		}
		priority := priorityValue.IntOr(int(audio.NativeDefaultSFXPriority))
		if nativeID < 0 || nativeID > 255 || priority < 0 || priority > 255 {
			return
		}
		c.nativeAudio.PlaySFX(uint8(nativeID), uint8(priority))
		return
	}
}

// MusicV starts or stops a PSG song. Project validation rejects fade arguments.
func (c *Context) MusicV(id, _ Value) {
	if c != nil && c.nativeAudio != nil {
		nativeID := id.IntOr(-1)
		if nativeID == -1 {
			c.nativeAudio.StopSong()
		} else if nativeID >= 0 && nativeID <= 255 {
			c.nativeAudio.PlaySong(uint8(nativeID))
		}
		return
	}
}

// SampleV preserves the asset-free compatibility no-op. Project builds
// reject sample() because PSG channels cannot play PCM assets.
func (c *Context) SampleV(id, volume, priority Value) {}

// GBANoteV queues one direct note on a fixed PSG channel. CH4 ignores note but
// still requires a channel-compatible sound ID.
func (c *Context) GBANoteV(channel, note, soundID, duration Value) {
	if c == nil || c.nativeAudio == nil {
		return
	}
	ch, pitch, sound, ticks := channel.Int(), note.Int(), soundID.Int(), duration.IntOr(0)
	if ch < 1 || ch > 4 || pitch < 0 || pitch > 127 || sound < 0 || sound > 255 || ticks < 0 || ticks > 65535 {
		return
	}
	c.nativeAudio.NoteOn(uint8(ch), uint8(pitch), uint8(sound), uint16(ticks))
}

func (c *Context) GBAStopV(channel Value) {
	if c == nil || c.nativeAudio == nil {
		return
	}
	ch := channel.Int()
	if ch >= 1 && ch <= 4 {
		c.nativeAudio.StopChannel(uint8(ch))
	}
}

func (c *Context) NativeAudioPreview() audio.NativePreview {
	var preview audio.NativePreview
	c.NativeAudioPreviewInto(&preview)
	return preview
}

// NativeAudioPreviewInto writes target telemetry into caller-owned memory.
// GBA code that polls every frame should use this form to avoid a large
// aggregate return crossing an interrupt boundary.
func (c *Context) NativeAudioPreviewInto(preview *audio.NativePreview) bool {
	if c == nil || c.nativeAudio == nil || preview == nil {
		return false
	}
	return c.nativeAudio.PreviewInto(preview)
}
