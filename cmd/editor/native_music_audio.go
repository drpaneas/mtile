package main

import (
	"fmt"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
)

func (g *Game) refreshNativeMusicPreview() {
	if g == nil || g.model == nil {
		return
	}
	song, ok := g.model.NativeSong()
	if g.nativeSFXMode {
		tempo := 120
		if ok {
			tempo = song.Tempo
		}
		preview, err := editor.RenderNativeSFXPreview(g.model.NativeAudio.Library, g.nativeSFXID, tempo, g.nativeEffectiveMuteMask(), g.model.NativeAudio.Settings)
		if err != nil {
			g.nativePreview = nil
			return
		}
		g.nativePreview = preview
		return
	}
	if !ok {
		g.nativePreview = nil
		return
	}
	preview, err := editor.RenderNativeMusicPreviewSettings(g.model.NativeAudio.Library, song.ID, g.nativeEffectiveMuteMask(), g.model.NativeAudio.Settings)
	if err != nil {
		g.nativePreview = nil
		g.statusMsg = "preview failed: " + err.Error()
		return
	}
	g.nativePreview = preview
}

func (g *Game) playNativeMusicPreview() {
	g.playNativeMusicPreviewFrom(0)
}

func (g *Game) playNativeMusicPreviewFrom(sampleOffset int) {
	song, ok := g.model.NativeSong()
	if !ok && !g.nativeSFXMode {
		g.statusMsg = "Place a note first"
		return
	}
	var preview *editor.NativeMusicPreview
	var err error
	if g.nativeSFXMode {
		tempo := 120
		if ok {
			tempo = song.Tempo
		}
		preview, err = editor.RenderNativeSFXPreview(g.model.NativeAudio.Library, g.nativeSFXID, tempo, g.nativeEffectiveMuteMask(), g.model.NativeAudio.Settings)
	} else {
		preview, err = editor.RenderNativeMusicPreviewSettings(g.model.NativeAudio.Library, song.ID, g.nativeEffectiveMuteMask(), g.model.NativeAudio.Settings)
	}
	if err != nil {
		g.statusMsg = "preview failed: " + err.Error()
		return
	}
	stats := preview.AudioStats()
	if stats.Peak == 0 {
		g.nativePreview = preview
		if g.nativeEffectiveMuteMask() == 0x0f {
			g.statusMsg = "All four channels are muted"
		} else {
			g.statusMsg = "Nothing to play — place a note or drum hit"
		}
		return
	}
	context, err := editorAudioContext()
	if err != nil {
		g.statusMsg = "preview failed: " + err.Error()
		return
	}
	g.stopAudioPreview()
	g.nativePreview = preview
	sampleOffset = min(max(sampleOffset, 0), max(0, len(preview.Samples)-1))
	g.nativePreviewOffset = sampleOffset
	g.previewPlayer, err = newEditorAudioPlayerAtRate(context, targetStereoToHostStereo(preview.LeftSamples[sampleOffset:], preview.RightSamples[sampleOffset:]), editor.NativePreviewSampleRate)
	if err != nil {
		g.statusMsg = "preview failed: " + err.Error()
		return
	}
	g.setPreviewPlayerVolume(g.previewPlayer)
	g.previewPlayer.Play()
	if g.nativeSFXMode {
		g.statusMsg = fmt.Sprintf("Playing sfx(%d) — %.2fs, peak %d/127", g.nativeSFXID, stats.Duration, stats.Peak)
	} else {
		g.statusMsg = fmt.Sprintf("Playing PSG preview — %.2fs, peak %d/127", stats.Duration, stats.Peak)
	}
}

func (g *Game) playNativeNotePreview(step, note int) {
	sound, ok := g.nativeSoundAtStep(step)
	if !ok {
		g.statusMsg = "note preview failed: the cell has no hardware sound"
		return
	}
	g.playNativeSoundPreview(sound.ID, note)
}

func (g *Game) playNativeSoundPreview(soundID, note int) {
	if g.nativeEffectiveMuteMask()&(1<<g.nativeChannel) != 0 {
		g.stopAudioPreview()
		g.statusMsg = fmt.Sprintf("CH%d is muted in the monitor mix", g.nativeChannel+1)
		return
	}
	left, right, err := editor.RenderNativeNotePreviewStereo(g.model.NativeAudio.Library, soundID, note, g.model.NativeAudio.Settings)
	if err != nil {
		g.statusMsg = "note preview failed: " + err.Error()
		return
	}
	context, err := editorAudioContext()
	if err != nil {
		g.statusMsg = "note preview failed: " + err.Error()
		return
	}
	g.stopAudioPreview()
	// An audition is not song transport. The explicit flag prevents the short
	// player from moving the arrangement playhead or lighting PLAY, including
	// when releasing a drag refreshes nativePreview while this note still rings.
	g.nativePreview = nil
	g.nativePreviewOffset = 0
	g.nativeAudition = true
	g.previewPlayer, err = newEditorAudioPlayerAtRate(context, targetStereoToHostStereo(left, right), editor.NativePreviewSampleRate)
	if err != nil {
		g.nativeAudition = false
		g.statusMsg = "note preview failed: " + err.Error()
		return
	}
	g.setPreviewPlayerVolume(g.previewPlayer)
	g.previewPlayer.Play()
}

func (g *Game) nativeMusicPreviewPlaying() bool {
	return g != nil && !g.nativeAudition && g.previewPlayer != nil && g.nativePreview != nil && g.previewPlayer.IsPlaying()
}

func (g *Game) toggleNativeMusicPreview() {
	if g.nativeMusicPreviewPlaying() {
		g.stopAudioPreview()
		return
	}
	g.playNativeMusicPreview()
}

func (g *Game) nativePreviewPosition() (sequence, step int, ok bool) {
	if !g.nativeMusicPreviewPlaying() {
		return 0, 0, false
	}
	sample := g.nativePreviewCurrentSample()
	return g.nativePreview.Position(sample)
}

func (g *Game) nativePreviewCurrentSample() int {
	if g == nil || g.previewPlayer == nil {
		return 0
	}
	played := int(g.previewPlayer.Position().Nanoseconds() * editor.NativePreviewSampleRate / int64(time.Second))
	return g.nativePreviewOffset + played
}

func (g *Game) nativeEffectiveMuteMask() uint8 {
	if g == nil {
		return 0
	}
	return editor.NativeMonitorMuteMask(g.nativeMuteMask, g.nativeSoloMask)
}

func (g *Game) toggleNativeChannelMute(channel int) {
	if channel < 0 || channel > 3 {
		return
	}
	g.applyNativeMonitorChange(func() { g.nativeMuteMask ^= 1 << channel })
	g.statusMsg = fmt.Sprintf("CH%d mute %s", channel+1, nativeToggleState(g.nativeMuteMask&(1<<channel) != 0))
}

func (g *Game) toggleNativeChannelSolo(channel int) {
	if channel < 0 || channel > 3 {
		return
	}
	g.applyNativeMonitorChange(func() { g.nativeSoloMask ^= 1 << channel })
	g.statusMsg = fmt.Sprintf("CH%d solo %s", channel+1, nativeToggleState(g.nativeSoloMask&(1<<channel) != 0))
}

func (g *Game) applyNativeMonitorChange(change func()) {
	wasPlaying := g.nativeMusicPreviewPlaying()
	sample := g.nativePreviewCurrentSample()
	g.stopAudioPreview()
	change()
	if wasPlaying {
		g.playNativeMusicPreviewFrom(sample)
		return
	}
	g.refreshNativeMusicPreview()
}

func nativeToggleState(active bool) string {
	if active {
		return "ON"
	}
	return "OFF"
}

func (g *Game) nativePreviewStep() (int, bool) {
	if g != nil && g.nativeCaptureVisible && g.nativeCaptureSequence == g.nativeSequence {
		return max(0, g.nativeCaptureTick/editor.NativeTicksPerStep), true
	}
	sequence, step, ok := g.nativePreviewPosition()
	if !ok || !g.nativeSFXMode && sequence != g.nativeSequence {
		return 0, false
	}
	return step, true
}

func (g *Game) updateNativeMusicPreviewLifecycle() {
	if g == nil || g.previewPlayer == nil {
		return
	}
	if !g.previewPlayer.IsPlaying() {
		g.stopAudioPreview()
		return
	}
	if sequence, step, ok := g.nativePreviewPosition(); ok {
		if g.nativeSFXMode {
			return
		}
		g.nativeSequence = sequence
		barSteps := g.model.NativePartBarStepCount(sequence)
		g.nativeBar = max(0, step/barSteps)
	}
}
