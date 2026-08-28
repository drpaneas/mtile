package runtime

import "github.com/drpaneas/mtile/runtime/audio"

// AttachSFX installs the generated cartridge's sample-clocked SFX player.
// A nil player disables sfx() calls without affecting graphics-only carts.
func (c *Context) AttachSFX(player *audio.SFXPlayer) {
	if c != nil {
		c.sfxPlayer = player
	}
}

// AttachAudioStream installs the generated cartridge's production stream for
// diagnostics. A nil stream clears stream telemetry without affecting the
// attached SFX player.
func (c *Context) AttachAudioStream(stream *audio.MonoStream) {
	if c != nil {
		c.audioStream = stream
	}
}

// AttachNativeAudio installs the PSG-only Audio v2 controller. A native
// controller and the archived v1 software stream are mutually exclusive in
// generated cartridges.
func (c *Context) AttachNativeAudio(controller *audio.Controller) {
	if c != nil {
		c.nativeAudio = controller
	}
}

// AudioStats is one bounded snapshot of stream, mixer, and sequencer state.
// Reading it does not enable profiling or change audio production.
type AudioStats struct {
	Stream audio.StreamStats
	Mixer  audio.Stats
	SFX    audio.SFXStats
	Native audio.NativeStats
}

func (c *Context) AudioStats() AudioStats {
	if c == nil {
		return AudioStats{}
	}
	var stats AudioStats
	if c.audioStream != nil {
		stats.Stream = c.audioStream.Stats()
	}
	if c.sfxPlayer != nil {
		stats.Mixer = c.sfxPlayer.MixerStats()
		stats.SFX = c.sfxPlayer.Stats()
	}
	if c.nativeAudio != nil {
		stats.Native = c.nativeAudio.Stats()
	}
	return stats
}

// SFX implements the initial PICO-style call surface. Channel -1 selects an
// automatic logical channel. Length 0 means all remaining authored steps.
// ID -1 stops one channel, or all channels when channel is also -1.
func (c *Context) SFX(id, channel, offset, length int) bool {
	if c == nil || c.sfxPlayer == nil {
		return false
	}
	_, ok := c.sfxPlayer.Play(id, channel, offset, length)
	return ok
}

func (c *Context) SFXV(id, channel, offset, length Value) {
	if c != nil && c.nativeAudio != nil {
		nativeID := id.Int()
		if nativeID == -1 {
			c.nativeAudio.StopSFX()
			return
		}
		priority := channel.IntOr(100)
		if nativeID < 0 || nativeID > 255 || priority < 0 || priority > 255 {
			return
		}
		c.nativeAudio.PlaySFX(uint8(nativeID), uint8(priority))
		return
	}
	c.SFX(id.Int(), channel.IntOr(-1), offset.IntOr(0), length.IntOr(0))
}

// Music starts a PICO-style pattern graph at a stable pattern ID. ID -1
// stops music-owned channels while leaving manually triggered SFX alone.
// A positive fade length is measured in milliseconds: starts fade in and
// stops fade out before releasing music-owned channels.
func (c *Context) Music(id, fadeMS int) bool {
	if c == nil || c.sfxPlayer == nil {
		return false
	}
	return c.sfxPlayer.Music(id, fadeMS)
}

func (c *Context) MusicV(id, fadeMS Value) {
	if c != nil && c.nativeAudio != nil {
		nativeID := id.IntOr(-1)
		if nativeID == -1 {
			c.nativeAudio.StopSong()
		} else if nativeID >= 0 && nativeID <= 255 {
			c.nativeAudio.PlaySong(uint8(nativeID))
		}
		return
	}
	c.Music(id.IntOr(-1), fadeMS.IntOr(0))
}

// Sample starts a native PCM asset. Volume and priority use -1 for the
// manifest defaults. ID -1 stops all native PCM voices.
func (c *Context) Sample(id, volume, priority int) bool {
	if c == nil || c.sfxPlayer == nil {
		return false
	}
	return c.sfxPlayer.Sample(id, volume, priority)
}

func (c *Context) SampleV(id, volume, priority Value) {
	if c != nil && c.nativeAudio != nil {
		return
	}
	c.Sample(id.IntOr(-1), volume.IntOr(-1), priority.IntOr(-1))
}

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

func (c *Context) SFXStats() audio.SFXStats {
	if c == nil || c.sfxPlayer == nil {
		return audio.SFXStats{}
	}
	return c.sfxPlayer.Stats()
}
