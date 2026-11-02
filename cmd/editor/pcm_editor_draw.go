package main

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/drpaneas/mtile/internal/audioimport"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

func pcmBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%.2f MiB", float64(n)/(1<<20))
}
func pcmWaveDescription(w audioimport.WaveInfo) string {
	channels := "mono"
	if w.Channels == 2 {
		channels = "stereo"
	}
	kind := "PCM"
	if w.Float {
		kind = "float"
	}
	return fmt.Sprintf("%.2f s / %d Hz / %d-bit %s / %s", w.Seconds(), w.Rate, w.Bits, kind, channels)
}
func pcmWrapText(value string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(value) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if smallTextWidth(next) > width && line != "" {
			lines = append(lines, line)
			line = word
		} else {
			line = next
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
func pcmParagraph(screen *ebiten.Image, value string, r editor.Rect, clr color.Color) {
	for i, line := range pcmWrapText(value, r.W) {
		y := r.Y + i*(uiSmallTextHeight+6)
		if y+uiSmallTextHeight > r.Y+r.H {
			return
		}
		drawSmallText(screen, truncatePCMText(line, r.W), r.X, y, clr)
	}
}
func pcmInput(screen *ebiten.Image, r editor.Rect, value string, focused bool) {
	fillRect(screen, r, colorBackground)
	border := color.Color(colorBorder)
	if focused {
		border = colorSelection
		value += "_"
	}
	strokeRect(screen, r, border)
	drawText(screen, truncateUIText(value, r.W-20), r.X+10, r.Y+(r.H-uiTextHeight)/2, colorText)
}
func (g *Game) drawPCMEditor(screen *ebiten.Image) {
	s := &g.pcm
	l := newPCMLayout()
	p, b := l.Panel, l.Body
	fillRect(screen, editor.Rect{W: screenWidth, H: screenHeight}, color.RGBA{A: 205})
	fillRect(screen, p, colorPanel)
	strokeRect(screen, p, colorSectionBorder)
	drawText(screen, "PCM EDITOR", p.X+24, p.Y+20, colorText)
	drawSmallText(screen, "IMPORT / PREPARE / USE IN LUA", p.X+24, p.Y+48, colorDim)
	drawNativeButton(screen, l.Close, "CLOSE", false, colorMusic)
	drawNativeButton(screen, l.Converter, "MMUTIL…", s.converter != "", colorMusic)
	fillRect(screen, l.List, colorBackground)
	strokeRect(screen, l.List, colorSectionBorder)
	if s.batch != nil {
		g.drawPCMBatchList(screen, l)
	} else {
		drawSmallText(screen, "PROJECT AUDIO", l.List.X+12, l.List.Y+12, colorText)
		visible := max(1, (l.List.H-52)/54)
		for i := s.scroll; i < min(len(s.entries), s.scroll+visible); i++ {
			e := s.entries[i]
			r := editor.Rect{X: l.List.X + 8, Y: l.List.Y + 44 + (i-s.scroll)*54, W: l.List.W - 16, H: 48}
			active := (s.phase == "asset" || s.phase == "done") && s.command == e.command()
			if active {
				fillRect(screen, r, colorButtonFace)
				strokeRect(screen, r, colorSelection)
			}
			drawText(screen, truncateUIText(e.Item.Name, r.W-16), r.X+8, r.Y+3, colorText)
			drawSmallText(screen, e.command(), r.X+8, r.Y+26, colorDim)
		}
		if len(s.entries) == 0 {
			pcmParagraph(screen, "No imported audio yet. Choose IMPORT AUDIO to add your first sound.", editor.Rect{X: l.List.X + 14, Y: l.List.Y + 52, W: l.List.W - 28, H: 100}, colorDim)
		}
		drawNativeButton(screen, l.Add, "IMPORT AUDIO", s.phase == "choose", colorMusic)
		drawSmallText(screen, fmt.Sprintf("Bank: %s / 24 MiB limit", pcmBytes(s.bankBytes)), l.List.X, l.Add.Y-24, colorDim)
	}
	switch s.phase {
	case "review":
		title := "2 / PREPARE FOR GBA"
		if s.batch != nil {
			title = fmt.Sprintf("2 / REVIEW BATCH (%d/%d)", s.batchIndex+1, len(s.batch.Items))
		}
		drawText(screen, title, b.X, b.Y, colorText)
		drawSmallText(screen, truncatePCMText(s.path, b.W), b.X, b.Y+32, colorDim)
		if s.prepared.Music {
			message := "Tracker song: notes, instruments and song order stay intact. Maxmod converts the module into the cartridge bank. WAV quality presets do not rewrite tracker instruments."
			if s.batch != nil {
				message += " Select a WAV row to change the shared WAV settings."
			}
			pcmParagraph(screen, message, editor.Rect{X: b.X, Y: b.Y + 82, W: b.W, H: 145}, colorText)
			drawSmallText(screen, "Source: "+pcmBytes(len(s.prepared.Data)), b.X, b.Y+240, colorDim)
		} else {
			if s.batch != nil && s.batch.Items[s.batchIndex].Error != "" {
				pcmParagraph(screen, "This file needs attention. Choose another shared preset, or REMOVE it from the queue.", editor.Rect{X: b.X, Y: b.Y + 66, W: b.W, H: 40}, colorText)
			} else {
				drawSmallText(screen, truncatePCMText("Original: "+pcmWaveDescription(s.prepared.Before), b.W), b.X, b.Y+68, colorDim)
				drawSmallText(screen, truncatePCMText("Prepared: "+pcmWaveDescription(s.prepared.After), b.W), b.X, b.Y+96, colorText)
			}
			details := []string{"Default: mono, up to 16 kHz, 8-bit. Keeps more high-tone detail.", "Mono, up to 8 kHz, 8-bit. Smaller; high tones become softer.", "Original 8/16-bit PCM. Preserves timing and loop metadata; trimming off."}
			qualities := []audioimport.Quality{audioimport.QualityBalanced, audioimport.QualitySmall, audioimport.QualitySource}
			for i, r := range l.Profiles {
				fillRect(screen, r, colorBackground)
				border := color.Color(colorBorder)
				if s.quality == qualities[i] {
					border = colorSelection
				}
				strokeRect(screen, r, border)
				label := pcmProfileLabel(s.prepared, qualities[i])
				if s.prepared.Analysis.Info.Rate <= 0 {
					label = strings.ToUpper(pcmQualityName(qualities[i]))
				}
				if s.batch != nil {
					label = "ALL WAVs: " + label + " / selected file"
				}
				drawSmallText(screen, truncatePCMText(label, r.W-20), r.X+10, r.Y+4, colorText)
				drawSmallText(screen, truncatePCMText(details[i], r.W-20), r.X+10, r.Y+24, colorDim)
			}
		}
		drawSmallText(screen, "ASSET NAME  /  letters, numbers, underscores", b.X, l.Name.Y-24, colorDim)
		pcmInput(screen, l.Name, s.name, s.focus == 2)
		summary := fmt.Sprintf("Source %s -> prepared %s. Final bank size is shown after import.", pcmBytes(s.prepared.OriginalBytes), pcmBytes(len(s.prepared.Data)))
		if s.prepared.Music {
			summary = "Module import preserves its authored voices. More simultaneous voices need more mixing work."
		}
		if s.batch != nil && s.batch.Items[s.batchIndex].Error != "" {
			summary = "Fix or remove failed rows before importing. The project has not changed."
		}
		pcmParagraph(screen, summary, editor.Rect{X: b.X, Y: l.Name.Y + 48, W: b.W, H: 32}, colorText)
		if !s.prepared.Music {
			y := l.Advice.Y
			for _, message := range pcmAudioAdvice(s.prepared) {
				r := l.Advice
				r.Y = y
				r.H = l.Advice.Y + l.Advice.H - y
				pcmParagraph(screen, message, r, colorText)
				y += len(pcmWrapText(message, r.W))*(uiSmallTextHeight+6) + 4
			}
			if s.prepared.Analysis.CanTrim() || s.batch != nil && s.batch.TrimEligibleCount() > 0 {
				label := "TRIM QUIET EDGES / optional, changes timing"
				if s.batch != nil {
					label = fmt.Sprintf("TRIM ELIGIBLE WAVs (%d) / shared, optional", s.batch.TrimEligibleCount())
				}
				if s.trimSilence {
					label = "RESTORE QUIET EDGES / undo trim"
					if s.batch != nil {
						label = "RESTORE QUIET EDGES / all queued WAVs"
					}
				}
				if s.quality == audioimport.QualitySource {
					label = "CHOOSE BALANCED OR SMALLEST TO ENABLE TRIM"
				}
				drawNativeButton(screen, l.Trim, label, s.trimSilence, colorMusic)
			}
		}
		label := "IMPORT + SAVE"
		if s.batch != nil {
			label = fmt.Sprintf("IMPORT %d FILES", len(s.batch.Items))
		}
		if s.confirmReplace {
			label = "REPLACE + SAVE"
			if s.batch != nil {
				label = "REPLACE + IMPORT"
			}
		}
		if s.batch != nil && s.batch.ErrorCount() > 0 {
			drawDisabledTactileButton(screen, l.Commit, fmt.Sprintf("FIX %d FILE(S)", s.batch.ErrorCount()), false)
		} else {
			drawNativeButton(screen, l.Commit, label, true, colorMusic)
		}
		if !s.prepared.Music {
			if len(s.prepared.Data) > 0 {
				g.drawPCMAuditionButton(screen, l.Preview, false)
			} else {
				drawDisabledTactileButton(screen, l.Preview, "PREPARED WAV", false)
			}
			if s.prepared.Analysis.Info.Rate > 0 {
				g.drawPCMAuditionButton(screen, l.Original, true)
			} else {
				drawDisabledTactileButton(screen, l.Original, "ORIGINAL WAV", false)
			}
		}
	case "asset", "done":
		title := "READY TO USE"
		if s.phase == "done" {
			title = "3 / IMPORT COMPLETE"
			if s.batch != nil {
				title = fmt.Sprintf("3 / IMPORTED %d FILES", len(s.batch.Items))
			}
		}
		drawText(screen, title, b.X, b.Y, colorText)
		drawSmallText(screen, truncatePCMText(s.name, b.W), b.X, b.Y+38, colorDim)
		code := editor.Rect{X: b.X, Y: b.Y + 82, W: b.W, H: 60}
		fillRect(screen, code, colorBackground)
		strokeRect(screen, code, colorBorder)
		drawText(screen, s.command, code.X+16, code.Y+18, colorText)
		drawNativeButton(screen, l.Copy, "COPY LUA CALL", false, colorMusic)
		if s.batch != nil {
			drawNativeButton(screen, l.CopyAll, "COPY ALL CALLS", false, colorMusic)
		}
		explanation := "Call this once when the event happens (for example, inside if btnp(5) then ... end). Each call starts a new WAV voice."
		if s.prepared.Music {
			explanation = "Call this in _init() to start and loop the song. pcm_music(-1) stops it. Calling it every update would restart the song."
		}
		pcmParagraph(screen, explanation, editor.Rect{X: b.X, Y: b.Y + 226, W: b.W, H: 98}, colorText)
		pcmParagraph(screen, "Saved under assets/audio/direct/. The next build embeds the bank in the ROM; no file loading is needed in Lua.", editor.Rect{X: b.X, Y: b.Y + 340, W: b.W, H: 76}, colorDim)
		if s.phase == "asset" {
			label := "DELETE ASSET"
			if s.confirmDelete {
				label = "CONFIRM DELETE"
			}
			drawNativeButton(screen, l.Delete, label, s.confirmDelete, colorDanger)
			if !s.prepared.Music && len(s.prepared.Data) > 0 {
				g.drawPCMAuditionButton(screen, l.Preview, false)
			}
		}
	default:
		drawText(screen, "1 / CHOOSE AUDIO", b.X, b.Y, colorText)
		drawSmallText(screen, "Browse for files, or paste one full path per line.", b.X, b.Y+32, colorDim)
		pathLabel := s.path
		if paths := pcmPathLines(s.path); len(paths) > 1 {
			pathLabel = fmt.Sprintf("%d paths ready / Next prepares the complete selection", len(paths))
		}
		pcmInput(screen, l.Path, pathLabel, s.focus == 1)
		drawNativeButton(screen, l.Browse, "BROWSE…", false, colorMusic)
		drawNativeButton(screen, l.Prepare, "NEXT: PREPARE", true, colorMusic)
		pcmParagraph(screen, "WAV: recorded effects, speech or a full recording. Integer PCM 8/16/24/32-bit and float 32/64-bit input, mono or stereo. Balanced preparation happens here before import.", editor.Rect{X: b.X, Y: b.Y + 200, W: b.W, H: 110}, colorText)
		pcmParagraph(screen, "MOD / XM / S3M / IT: tracker music with reusable instruments. These play through pcm_music(id), with looping by default.", editor.Rect{X: b.X, Y: b.Y + 320, W: b.W, H: 90}, colorText)
		pcmParagraph(screen, "The Maxmod converter (mmutil) is needed to create or update the bank. Existing imported projects build without it. MP3/OGG/FLAC: export as WAV first.", editor.Rect{X: b.X, Y: b.Y + 420, W: b.W, H: 88}, colorDim)
	}
	message := s.message
	clr := color.Color(colorText)
	if s.busy != "" {
		message = s.busy
		clr = colorSelection
	}
	if message != "" {
		r := editor.Rect{X: b.X, Y: min(b.Y+b.H-110, p.Y+p.H-176), W: b.W, H: 98}
		if s.phase == "review" {
			r.Y = b.Y + 560
			r.H = 70
		}
		fillRect(screen, r, colorPanel)
		pcmParagraph(screen, message, r, clr)
	}
}
func (g *Game) drawPCMAuditionButton(screen *ebiten.Image, r editor.Rect, original bool) {
	variant, label := "prepared", "PREPARED WAV"
	if original {
		variant, label = "original", "ORIGINAL WAV"
	}
	if g.previewPlayer != nil && g.previewPlayer.IsPlaying() && g.pcm.previewSource == variant {
		label = "STOP " + strings.ToUpper(variant)
	}
	drawNativeButton(screen, r, label, false, colorMusic)
}

func truncatePCMText(value string, width int) string {
	return truncateTextToWidth(value, width, uiSmallFace)
}
