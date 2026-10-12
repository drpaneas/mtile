package main

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type poseVariantEditor struct {
	open                     bool
	selected, source, scroll int
	colorScroll              int
	picking                  bool
	action, draft, message   string
}

func poseVariantsButton(panel editor.Rect) editor.Rect {
	return editor.Rect{X: panel.X + 392, Y: panel.Y + panel.H - 44, W: min(152, max(0, panel.W-472)), H: 32}
}
func poseVariantPanel() editor.Rect {
	w, h := min(960, screenWidth-32), min(560, screenHeight-64)
	return editor.Rect{X: (screenWidth - w) / 2, Y: (screenHeight - h) / 2, W: w, H: h}
}
func variantButton(p editor.Rect, index int) editor.Rect {
	return editor.Rect{X: p.X + 16 + index*112, Y: p.Y + 44, W: 104, H: 30}
}
func variantSwatch(p editor.Rect, index int) editor.Rect {
	const size = 21
	return editor.Rect{X: p.X + 220 + (index%16)*size, Y: p.Y + 162 + (index/16)*size, W: size - 2, H: size - 2}
}
func variantColorRow(p editor.Rect, row int) editor.Rect {
	return editor.Rect{X: p.X + 204, Y: p.Y + 162 + row*34, W: 264, H: 28}
}
func posePreviewColors(preview editor.PosePreview) []int {
	var used [256]bool
	for _, c := range preview.Pixels {
		if c > 0 && c < 256 {
			used[c] = true
		}
	}
	colors := []int{}
	for c := 1; c < 256; c++ {
		if used[c] {
			colors = append(colors, c)
		}
	}
	return colors
}
func (g *Game) poseVariantColors() []int {
	if g.selectedPose < 0 || g.selectedPose >= len(g.authoredPoses()) {
		return nil
	}
	return posePreviewColors(g.animationPoseThumbnail(g.selectedPose))
}
func variantReplacement(v *assetsync.PoseColorVariant, source int) int {
	if v != nil {
		for _, swap := range v.Colors {
			if swap.From == source {
				return swap.To
			}
		}
	}
	return source
}
func (g *Game) selectedColorVariant() *assetsync.PoseColorVariant {
	id := g.poseVariants.selected
	if id < 0 || id >= len(g.model.Poses.Variants) {
		return nil
	}
	return &g.model.Poses.Variants[id]
}
func (g *Game) commitColorVariant(err error) {
	if err != nil {
		g.poseVariants.message = err.Error()
		g.requestRender()
		return
	}
	g.poseVariants.message = "Updated · Cmd/Ctrl+S saves · Cmd/Ctrl+Z undoes"
	g.dirty = true
	g.undo.forcePush(g.snapshot())
	g.requestRender()
}
func (g *Game) updatePoseVariants() {
	s := &g.poseVariants
	p := poseVariantPanel()
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if s.picking {
			s.picking = false
		} else if s.action != "" {
			s.action = ""
		} else {
			s.open = false
		}
		g.requestRender()
		return
	}
	if s.action != "" {
		for _, r := range ebiten.AppendInputChars(nil) {
			if !unicode.IsControl(r) && len(s.draft)+utf8.RuneLen(r) <= assetsync.MaxPoseNameBytes {
				s.draft += string(r)
			}
		}
		if codeKeyRepeat(ebiten.KeyBackspace) && s.draft != "" {
			_, n := utf8.DecodeLastRuneInString(s.draft)
			s.draft = s.draft[:len(s.draft)-n]
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			v := g.selectedColorVariant()
			var err error
			switch s.action {
			case "NEW":
				err = g.model.CreatePoseColorVariant(s.draft)
			case "COPY":
				if v != nil {
					err = g.model.DuplicatePoseColorVariant(v.Name, s.draft)
				}
			case "RENAME":
				if v != nil {
					err = g.model.RenamePoseColorVariant(v.Name, s.draft)
				}
			}
			if err == nil {
				if s.action != "RENAME" {
					s.selected = len(g.model.Poses.Variants) - 1
				}
				s.action = ""
			}
			g.commitColorVariant(err)
		}
		g.requestRender()
		return
	}
	mx, my := ebiten.CursorPosition()
	colors := g.poseVariantColors()
	s.colorScroll = min(max(0, s.colorScroll), max(0, len(colors)-9))
	if _, dy := ebiten.Wheel(); dy != 0 && !s.picking {
		if mx < p.X+192 {
			s.scroll = min(max(0, s.scroll-int(dy)), max(0, len(g.model.Poses.Variants)-10))
		} else {
			s.colorScroll = min(max(0, s.colorScroll-int(dy)), max(0, len(colors)-9))
		}
		g.requestRender()
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	if s.picking {
		for i := 1; i < 256; i++ {
			if rectContains(variantSwatch(p, i), mx, my) {
				if v := g.selectedColorVariant(); v != nil {
					g.commitColorVariant(g.model.SetPoseColorReplacement(v.Name, s.source, i))
				}
				s.picking = false
				g.requestRender()
				return
			}
		}
		s.picking = false
		g.requestRender()
		return
	}
	for i, label := range []string{"NEW", "COPY", "RENAME", "DELETE", "SAVE", "CLOSE"} {
		if !rectContains(variantButton(p, i), mx, my) {
			continue
		}
		v := g.selectedColorVariant()
		switch label {
		case "NEW":
			s.action = label
			s.draft = ""
		case "COPY", "RENAME":
			if v != nil {
				s.action = label
				s.draft = ""
			}
		case "DELETE":
			if v != nil {
				name := v.Name
				g.commitColorVariant(g.model.DeletePoseColorVariant(name))
				s.selected = min(s.selected, len(g.model.Poses.Variants)-1)
				s.message = "Deleted " + name + " · update any pvariant calls; unknown literal names fail builds"
			}
		case "SAVE":
			g.persist()
			s.message = g.statusMsg
			if s.message == "" {
				s.message = "Saved color variants"
			}
		case "CLOSE":
			s.open = false
		}
		g.requestRender()
		return
	}
	for i := s.scroll; i < len(g.model.Poses.Variants) && i < s.scroll+10; i++ {
		row := editor.Rect{X: p.X + 16, Y: p.Y + 110 + (i-s.scroll)*30, W: 172, H: 28}
		if rectContains(row, mx, my) {
			s.selected = i
			g.requestRender()
			return
		}
	}
	for row := 0; row < 9 && row+s.colorScroll < len(colors); row++ {
		if rectContains(variantColorRow(p, row), mx, my) && g.selectedColorVariant() != nil {
			s.source = colors[row+s.colorScroll]
			s.picking = true
			g.requestRender()
			return
		}
	}
}

func (g *Game) drawPoseVariants(screen *ebiten.Image) {
	p := poseVariantPanel()
	s := &g.poseVariants
	fillRect(screen, editor.Rect{W: screenWidth, H: screenHeight}, rgba(colorInk, 190))
	fillRect(screen, p, colorPanel)
	strokeRect(screen, p, colorBorder)
	drawText(screen, "POSE COLOR VARIANTS", p.X+16, p.Y+14, colorSprite)
	for i, label := range []string{"NEW", "COPY", "RENAME", "DELETE", "SAVE", "CLOSE"} {
		drawTactileButton(screen, variantButton(p, i), label, false, colorSprite, true)
	}
	for i := s.scroll; i < len(g.model.Poses.Variants) && i < s.scroll+10; i++ {
		row := editor.Rect{X: p.X + 16, Y: p.Y + 110 + (i-s.scroll)*30, W: 172, H: 28}
		drawTactileButton(screen, row, truncateUIText(g.model.Poses.Variants[i].Name, row.W-12), i == s.selected, colorSprite, true)
	}
	colors := g.poseVariantColors()
	v := g.selectedColorVariant()
	poseName := "Select a pose before opening color variants"
	if g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()) {
		poseName = g.authoredPoses()[g.selectedPose].Name
	}
	drawText(screen, truncateUIText(poseName, p.W-236), p.X+204, p.Y+104, colorText)
	drawSmallText(screen, "ORIGINAL", p.X+204, p.Y+140, colorDim)
	drawSmallText(screen, "REPLACEMENT", p.X+340, p.Y+140, colorDim)
	for row := 0; row < 9 && row+s.colorScroll < len(colors); row++ {
		source := colors[row+s.colorScroll]
		replacement := variantReplacement(v, source)
		r := variantColorRow(p, row)
		fillRect(screen, editor.Rect{X: r.X, Y: r.Y + 2, W: 24, H: 24}, g.swatch(source))
		drawSmallText(screen, fmt.Sprintf("%03d", source), r.X+34, r.Y+8, colorText)
		drawSmallText(screen, "->", r.X+98, r.Y+8, colorDim)
		button := editor.Rect{X: r.X + 136, Y: r.Y, W: 128, H: r.H}
		drawTactileButton(screen, button, fmt.Sprintf("%03d  CHANGE", replacement), false, colorSprite, v != nil)
		fillRect(screen, editor.Rect{X: button.X + 4, Y: button.Y + 5, W: 18, H: 18}, g.swatch(replacement))
	}
	if len(colors) == 0 {
		drawSmallText(screen, "This pose has no opaque colors.", p.X+204, p.Y+170, colorDim)
	}
	if len(colors) > 9 {
		drawSmallText(screen, "Scroll for more pose colors", p.X+204, p.Y+478, colorDim)
	}
	if v == nil {
		drawSmallText(screen, "Create or select a variant to edit replacements.", p.X+204, p.Y+448, colorText)
	}
	if g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()) {
		preview := g.animationPoseThumbnail(g.selectedPose)
		for side := 0; side < 2; side++ {
			label := "ORIGINAL POSE"
			frame := preview
			if side == 1 {
				label = "WITH VARIANT"
				frame.Pixels = append([]int(nil), preview.Pixels...)
				for i, c := range frame.Pixels {
					frame.Pixels[i] = variantReplacement(v, c)
				}
			}
			r := editor.Rect{X: p.X + p.W - 204, Y: p.Y + 166 + side*158, W: 184, H: 128}
			drawSmallText(screen, label, r.X, r.Y-20, colorDim)
			drawCheckerboardColors(screen, r, colorSheetCheckerA, colorSheetCheckerB)
			g.drawAnimationFrame(screen, frame, r)
		}
	}
	if s.picking {
		picker := editor.Rect{X: p.X + 204, Y: p.Y + 130, W: 370, H: 372}
		fillRect(screen, picker, colorPanel)
		strokeRect(screen, picker, colorBorder)
		drawSmallText(screen, fmt.Sprintf("Replace color %d · Esc cancels", s.source), picker.X+16, picker.Y+12, colorText)
		for i := 1; i < 256; i++ {
			r := variantSwatch(p, i)
			fillRect(screen, r, g.swatch(i))
			if i == variantReplacement(v, s.source) {
				strokeRect(screen, r, colorText)
			}
		}
	}

	text := s.message
	if s.action != "" {
		text = s.action + " name: " + s.draft + "_   Enter saves · Esc cancels"
	}
	drawSmallText(screen, truncateUIText(text, p.W-32), p.X+16, p.Y+p.H-48, colorText)
	drawSmallText(screen, "Only this pose’s colors are shown. Click a replacement to change it; choose the original to reset.", p.X+16, p.Y+p.H-24, colorDim)
}
