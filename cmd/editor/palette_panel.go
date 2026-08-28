package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	paletteSwatchColumns      = 16
	paletteSelectionThickness = 2
	paletteColorHeaderLabel   = "COLOR"
	colorPickerTitle          = "CHANGE COLOR"
)

func swatchRect(index int) editor.Rect {
	row, col := index/paletteSwatchColumns, index%paletteSwatchColumns
	return editor.Rect{
		X: paletteRect.X + col*paletteSwatchWidth,
		Y: paletteRect.Y + row*paletteSwatchHeight,
		W: paletteSwatchWidth - 1,
		H: paletteSwatchHeight - 1,
	}
}

func paletteSelectionRect(index int) editor.Rect {
	r := swatchRect(index)
	return editor.Rect{X: r.X - 1, Y: r.Y - 1, W: r.W + 2, H: r.H + 2}
}

func drawPaletteSelectionRing(screen *ebiten.Image, r editor.Rect) {
	t := min(paletteSelectionThickness, min(r.W, r.H)/2)
	if t < 1 {
		return
	}
	// Four integer-aligned bars stay perfectly crisp. The second pixel falls
	// inward, strengthening contrast without covering neighbouring swatches.
	fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: r.W, H: t}, colorSprite)
	fillRect(screen, editor.Rect{X: r.X, Y: r.Y + r.H - t, W: r.W, H: t}, colorSprite)
	fillRect(screen, editor.Rect{X: r.X, Y: r.Y + t, W: t, H: r.H - 2*t}, colorSprite)
	fillRect(screen, editor.Rect{X: r.X + r.W - t, Y: r.Y + t, W: t, H: r.H - 2*t}, colorSprite)
}

// paintableColorCount is the number of swatches that are meaningfully
// selectable as a paint color: the palette's own stored entries, not the
// 256-wide expanded preview. For a 16-color palette, indices 16..255 are all
// aliases of color 0 (see assetsync.ExpandedPaletteColors) and are rejected
// by Model.SetSpritePixel, so they must not be offered as distinct choices.
func (g *Game) paintableColorCount() int {
	return len(g.model.ActivePalette.Colors)
}

func (g *Game) updatePaletteSwatches(mx, my int) {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	if g.preview == nil {
		return
	}
	for i := 0; i < g.paintableColorCount(); i++ {
		if rectContains(swatchRect(i), mx, my) {
			g.currentColor = i
			return
		}
	}
}

func (g *Game) drawPaletteSwatches(screen *ebiten.Image) {
	titleX, titleY := spritePanelTitlePosition(spritePalettePanelRect)
	headerY := spritePanelMetaY(spritePalettePanelRect)
	colorNumber := paletteColorNumberText(g.currentColor)
	drawText(screen, paletteColorHeaderLabel, titleX, titleY, colorText)
	colorNumberX := titleX + uiTextWidth(paletteColorHeaderLabel) + smallTextWidth(" ")
	drawSmallText(screen, colorNumber, colorNumberX, headerY, colorSprite)
	meta := paletteMetaText(g.paintableColorCount())
	metaX := paletteMetaX(meta)
	if metaX > colorNumberX+smallTextWidth(colorNumber)+12 {
		drawSmallText(screen, meta, metaX, headerY, colorDim)
	}
	if g.preview == nil {
		return
	}
	for i := 0; i < g.paintableColorCount(); i++ {
		r := swatchRect(i)
		if isEraserIndex(i) {
			drawEraserSwatch(screen, r)
		} else {
			fillRect(screen, r, g.swatch(i))
		}
	}
	// Draw selection last. Painting it inside the swatch loop lets the next
	// row/column overwrite its right and bottom edges, which makes the focus
	// state look broken. Cyan is reserved for real active/selected state.
	if g.currentColor >= 0 && g.currentColor < g.paintableColorCount() {
		drawPaletteSelectionRing(screen, paletteSelectionRect(g.currentColor))
	}
}

func paletteColorNumberText(colorID int) string {
	return fmt.Sprintf("%03d", colorID)
}

func paletteMetaText(colorCount int) string {
	return fmt.Sprintf("%d COLORS", colorCount)
}

func paletteMetaX(meta string) int {
	right := spritePalettePanelRect.X + spritePalettePanelRect.W - spritePanelTitleInsetX
	if spritePickerPanelRect.W == 0 {
		right = paletteRect.X + paletteRect.W
	}
	return right - smallTextWidth(meta)
}

// --- RGB15 color picker for the currently selected swatch ---

func pickerSliderRect(row int) editor.Rect {
	top := pickerRect.Y + 28
	if spritePickerPanelRect.W == 0 {
		preview := colorPickerPreviewRect()
		top = preview.Y + preview.H + 10
	}
	return editor.Rect{X: pickerRect.X + 24, Y: top + row*24, W: max(18, pickerRect.W-58), H: 14}
}

func colorPickerPreviewRect() editor.Rect {
	y := pickerRect.Y
	if spritePickerPanelRect.W == 0 {
		y += uiTextHeight + 8
	}
	return editor.Rect{X: pickerRect.X + pickerRect.W - 30, Y: y, W: 30, H: 18}
}

func colorPickerTitlePosition() (int, int) {
	if spritePickerPanelRect.W > 0 {
		return spritePanelTitlePosition(spritePickerPanelRect)
	}
	return pickerRect.X, pickerRect.Y
}

func colorPickerInfoY() int {
	// The standalone desktop panel gives its reclaimed vertical space to the
	// flags above it, then anchors hardware metadata to the content baseline.
	// Compact mode shares this panel with the palette, so preserve its tighter
	// top-aligned rhythm instead of stretching a three-row form arbitrarily.
	if spritePickerPanelRect.W > 0 {
		return pickerRect.Y + pickerRect.H - uiSmallTextHeight
	}
	lastSlider := pickerSliderRect(2)
	return lastSlider.Y + lastSlider.H + 12
}

func (g *Game) editableActiveColor() (assetsync.RGB15, bool) {
	idx := g.currentColor
	if idx < 0 || idx >= len(g.model.ActivePalette.Colors) {
		return assetsync.RGB15{}, false
	}
	return g.model.ActivePalette.Colors[idx], true
}

func (g *Game) updateColorPicker(mx, my int) {
	c, ok := g.editableActiveColor()
	if !ok {
		return
	}
	changed := false
	if v, ok := updateSlider(pickerSliderRect(0), mx, my, c.R); ok {
		c.R, changed = v, true
	}
	if v, ok := updateSlider(pickerSliderRect(1), mx, my, c.G); ok {
		c.G, changed = v, true
	}
	if v, ok := updateSlider(pickerSliderRect(2), mx, my, c.B); ok {
		c.B, changed = v, true
	}
	if !changed {
		return
	}
	if err := g.model.SetActiveColor(g.currentColor, c); err != nil {
		g.statusMsg = err.Error()
		return
	}
	// Dragging a slider is continuous like pixel painting, not a discrete
	// click like a flag toggle, so it follows paintPixel's model: mark dirty
	// and let Cmd/Ctrl+S (or a mode toggle) persist it, rather than writing
	// to disk every frame of the drag.
	g.dirty = true
}

// updateSlider returns the new 0..31 value and true if the mouse is
// dragging within the slider track, otherwise (0, false).
func updateSlider(r editor.Rect, mx, my, current int) (int, bool) {
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		return current, false
	}
	if my < r.Y-3 || my >= r.Y+r.H+3 || mx < r.X || mx > r.X+r.W {
		return current, false
	}
	frac := float64(mx-r.X) / float64(r.W)
	frac = math.Min(1, math.Max(0, frac))
	v := int(math.Round(frac * 31))
	if v == current {
		return current, false
	}
	return v, true
}

func (g *Game) drawColorPicker(screen *ebiten.Image) {
	titleX, titleY := colorPickerTitlePosition()
	drawText(screen, colorPickerTitle, titleX, titleY, colorText)
	c, ok := g.editableActiveColor()
	if !ok {
		drawSmallText(screen, fmt.Sprintf("ID %d · ALIAS OF 0", g.currentColor), pickerRect.X, titleY+uiTextHeight+8, colorDim)
	} else {
		labels := [3]string{"R", "G", "B"}
		values := [3]int{c.R, c.G, c.B}
		accents := [3]color.Color{colorDanger, colorRun, colorCode}
		preview := colorPickerPreviewRect()
		g.fillColor(screen, preview, g.currentColor)
		strokeRoundedRect(screen, preview, 3, colorBorder)
		for i := 0; i < 3; i++ {
			r := pickerSliderRect(i)
			drawSmallText(screen, labels[i], pickerRect.X, r.Y+(r.H-uiSmallTextHeight)/2, accents[i])
			fillRoundedRect(screen, r, 3, colorInk)
			strokeRoundedRect(screen, r, 3, rgba(colorBorder, 190))
			filled := max(2, r.W*values[i]/31)
			fillRoundedRect(screen, editor.Rect{X: r.X + 2, Y: r.Y + 2, W: max(1, filled-4), H: r.H - 4}, 2, accents[i])
			value := fmt.Sprintf("%02d", values[i])
			drawSmallText(screen, value, pickerRect.X+pickerRect.W-smallTextWidth(value), r.Y+(r.H-uiSmallTextHeight)/2, colorText)
		}
		infoY := colorPickerInfoY()
		if infoY+uiSmallTextHeight <= pickerRect.Y+pickerRect.H {
			drawSmallText(screen, colorPickerInfoText(g.currentColor, c.Packed()), pickerRect.X, infoY, colorDim)
		}
	}
}

const colorZeroHint = "id 0 is transparent (eraser)"
const spriteZeroHint = "sprite 0 is reserved (transparent eraser)"
const spriteZeroLockedMessage = "sprite 0 is locked: it is the transparent map eraser"

func colorPickerInfoText(id int, packed uint16) string {
	return fmt.Sprintf("BGR555 · 0x%04X · ID %d", packed, id)
}
