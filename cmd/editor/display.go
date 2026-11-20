package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Layout and hit testing use logical window coordinates. Only the window
// adapter and drawing helpers cross into physical display pixels.
var (
	editorInputScale = 1.0
	editorDrawScale  = 1.0
)

type editorWindow struct {
	*Game
	scale         float64
	width, height float64
}

var (
	_ ebiten.Game      = (*editorWindow)(nil)
	_ ebiten.LayoutFer = (*editorWindow)(nil)
)

func validDisplayScale(scale float64) float64 {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 1
	}
	return scale
}

func (w *editorWindow) Layout(outsideWidth, outsideHeight int) (int, int) {
	width, height := w.LayoutF(float64(outsideWidth), float64(outsideHeight))
	return int(math.Ceil(width)), int(math.Ceil(height))
}

func (w *editorWindow) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	scale := 1.0
	if monitor := ebiten.Monitor(); monitor != nil {
		scale = monitor.DeviceScaleFactor()
	}
	return w.layoutAtScale(outsideWidth, outsideHeight, scale)
}

func (w *editorWindow) layoutAtScale(width, height, scale float64) (float64, float64) {
	scale = validDisplayScale(scale)
	width, height = math.Max(1, width), math.Max(1, height)
	w.Game.Layout(int(width), int(height))
	if w.scale != scale || w.width != width || w.height != height {
		w.scale, w.width, w.height = scale, width, height
		w.requestRender()
	}
	editorInputScale = scale
	// Preserve fractional window dimensions until after converting to physical
	// pixels; truncating them first can introduce whole-frame resampling.
	return width * scale, height * scale
}

func (w *editorWindow) Draw(screen *ebiten.Image) {
	w.drawAtScale(screen, w.scale)
}

func (g *Game) drawAtScale(screen *ebiten.Image, scale float64) {
	previous := editorDrawScale
	editorDrawScale = validDisplayScale(scale)
	defer func() { editorDrawScale = previous }()
	g.Draw(screen)
}

func logicalCursorPosition(x, y, scale float64) (int, int) {
	scale = validDisplayScale(scale)
	return int(math.Floor(x / scale)), int(math.Floor(y / scale))
}

func editorCursorPosition() (int, int) {
	// Divide before rounding: CursorPosition's integer coordinates would lose
	// precision at fractional scale factors and near character boundaries.
	x, y := ebiten.CursorPositionF()
	return logicalCursorPosition(x, y, editorInputScale)
}
