package main

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) drawPCMBatchList(screen *ebiten.Image, l pcmLayout) {
	s := &g.pcm
	b := s.batch
	title := fmt.Sprintf("QUEUE (%d)", len(b.Items))
	if s.phase == "done" {
		title = "IMPORTED"
	}
	drawSmallText(screen, title, l.List.X+12, l.List.Y+12, colorText)
	if s.phase == "review" {
		drawNativeMiniButton(screen, l.RemoveQueued, "REMOVE", false, colorDanger)
	}
	visible := max(1, (l.List.H-52)/54)
	for i := s.batchScroll; i < min(len(b.Items), s.batchScroll+visible); i++ {
		item := b.Items[i]
		r := editor.Rect{X: l.List.X + 8, Y: l.List.Y + 44 + (i-s.batchScroll)*54, W: l.List.W - 16, H: 48}
		if i == s.batchIndex {
			fillRect(screen, r, colorButtonFace)
			strokeRect(screen, r, colorSelection)
		}
		drawText(screen, truncateUIText(item.Name, r.W-16), r.X+8, r.Y+3, colorText)
		status := "WAV / " + pcmBytes(item.Bytes)
		if item.Prepared.Music {
			status = "TRACKER / " + pcmBytes(item.Bytes)
		} else if item.Prepared.Trimmed {
			status = "WAV / trimmed / " + pcmBytes(item.Bytes)
		}
		if s.phase == "done" && i < len(s.batchCommands) {
			status = s.batchCommands[i]
		}
		if item.Error != "" {
			drawSmallText(screen, "NEEDS ATTENTION", r.X+8, r.Y+26, colorDanger)
		} else {
			drawSmallText(screen, truncatePCMText(status, r.W-16), r.X+8, r.Y+26, colorDim)
		}
	}
	label := "ADD FILES"
	if s.phase == "done" {
		label = "IMPORT AUDIO"
	}
	drawNativeButton(screen, l.Add, label, false, colorMusic)
	size := fmt.Sprintf("Prepared total: %s / %d files", pcmBytes(b.PreparedBytes()), len(b.Items))
	if s.phase == "done" {
		size = fmt.Sprintf("Bank: %s / 24 MiB limit", pcmBytes(s.bankBytes))
	}
	drawSmallText(screen, truncatePCMText(size, l.List.W), l.List.X, l.Add.Y-24, colorDim)
}
