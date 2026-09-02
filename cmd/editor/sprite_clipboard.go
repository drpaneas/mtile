package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
)

const spriteClipboardMaxPixels = 128

var readSpriteClipboardImagePNG = hostclipboard.ReadImagePNG

// spritePastePlacement is the transient, quantized image shown over the
// spritesheet while the user chooses its destination. It deliberately lives
// outside the model and undo snapshot: merely moving or cancelling a paste
// preview must never dirty the project.
type spritePastePlacement struct {
	pixels      []int
	pixelWidth  int
	pixelHeight int
	startCol    int
	startRow    int
}

func (p *spritePastePlacement) tileCols() int {
	if p == nil {
		return 0
	}
	return p.pixelWidth / 8
}

func (p *spritePastePlacement) tileRows() int {
	if p == nil {
		return 0
	}
	return p.pixelHeight / 8
}

// pasteSpriteClipboard gives a system image priority and starts interactive
// placement for it. The original internal 8x8 copy buffer remains an immediate
// fallback when the system clipboard has no image.
func (g *Game) pasteSpriteClipboard() {
	data, err := readSpriteClipboardImagePNG()
	if err == nil {
		if err := g.beginSpritePastePNG(data); err != nil {
			g.statusMsg = err.Error()
		}
		return
	}
	if errors.Is(err, hostclipboard.ErrNoImage) {
		if g.hasCopied {
			g.pasteCurrentSprite()
		} else {
			g.statusMsg = hostclipboard.ErrNoImage.Error()
		}
		return
	}
	if g.hasCopied {
		g.pasteCurrentSprite()
		return
	}
	g.statusMsg = err.Error()
}

func (g *Game) beginSpritePastePNG(data []byte) error {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("clipboard image is not valid PNG data: %w", err)
	}
	return g.beginSpritePasteImage(img)
}

func (g *Game) prepareSpritePasteImage(img image.Image) (*spritePastePlacement, error) {
	if g == nil || g.model == nil {
		return nil, fmt.Errorf("sprite editor is unavailable")
	}
	if img == nil {
		return nil, fmt.Errorf("clipboard image is empty")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width%8 != 0 || height%8 != 0 {
		return nil, fmt.Errorf("clipboard image must be a positive multiple of 8 pixels (got %dx%d)", width, height)
	}
	if width > spriteClipboardMaxPixels || height > spriteClipboardMaxPixels {
		return nil, fmt.Errorf("clipboard image must be at most %dx%d pixels (got %dx%d)", spriteClipboardMaxPixels, spriteClipboardMaxPixels, width, height)
	}
	if len(g.model.ActivePalette.Colors) < 2 {
		return nil, fmt.Errorf("active palette needs a transparent color plus at least one paint color")
	}

	startRow, startCol := g.baseSpriteRowCol()
	return &spritePastePlacement{
		pixels:      editor.QuantizeImageToPalette(img, g.model.ActivePalette.Colors),
		pixelWidth:  width,
		pixelHeight: height,
		startCol:    startCol,
		startRow:    startRow,
	}, nil
}

// beginSpritePasteImage enters placement mode without mutating the project.
// The initial target is the current selection, so feedback appears immediately
// even before the pointer reaches the spritesheet.
func (g *Game) beginSpritePasteImage(img image.Image) error {
	placement, err := g.prepareSpritePasteImage(img)
	if err != nil {
		return err
	}
	cols, rows := g.spriteCols(), g.spriteRows()
	if placement.tileCols() > cols || placement.tileRows() > rows {
		return fmt.Errorf("%dx%d tile clipboard image does not fit the %dx%d spritesheet", placement.tileCols(), placement.tileRows(), cols, rows)
	}
	if placement.tileCols() == cols && placement.tileRows() == rows {
		return fmt.Errorf("clipboard image covers locked sprite 0 and has no valid placement")
	}
	g.spritePaste = placement
	g.spriteShapeActive = false
	g.sheetSelectionDragging = false
	g.statusMsg = g.spritePastePlacementStatus()
	g.requestRender()
	return nil
}

func (g *Game) spritePasteTargetError(placement *spritePastePlacement, startCol, startRow int) error {
	if g == nil || g.model == nil || placement == nil {
		return fmt.Errorf("sprite paste preview is unavailable")
	}
	tileCols, tileRows := placement.tileCols(), placement.tileRows()
	cols, rows := g.spriteCols(), g.spriteRows()
	if startCol < 0 || startRow < 0 || startCol+tileCols > cols || startRow+tileRows > rows {
		return fmt.Errorf("%dx%d tile area at (%d,%d) exceeds the %dx%d spritesheet", tileCols, tileRows, startCol, startRow, cols, rows)
	}
	if startCol == 0 && startRow == 0 {
		return fmt.Errorf("sprite 0 is locked as the transparent map eraser")
	}
	return nil
}

func (g *Game) spritePastePlacementStatus() string {
	placement := g.spritePaste
	if placement == nil {
		return ""
	}
	if err := g.spritePasteTargetError(placement, placement.startCol, placement.startRow); err != nil {
		return "Cannot paste here: " + err.Error()
	}
	return fmt.Sprintf("Place %dx%d image: move over the spritesheet, click to paste, Esc to cancel", placement.pixelWidth, placement.pixelHeight)
}

func (g *Game) moveSpritePastePlacement(startCol, startRow int) {
	if g == nil || g.spritePaste == nil {
		return
	}
	if g.spritePaste.startCol == startCol && g.spritePaste.startRow == startRow {
		return
	}
	g.spritePaste.startCol = startCol
	g.spritePaste.startRow = startRow
	g.statusMsg = g.spritePastePlacementStatus()
	g.requestRender()
}

func (g *Game) cancelSpritePastePlacement() {
	if g == nil || g.spritePaste == nil {
		return
	}
	g.spritePaste = nil
	g.statusMsg = "Paste canceled"
	g.requestRender()
}

func (g *Game) commitSpritePastePlacement() error {
	if g == nil || g.spritePaste == nil {
		return fmt.Errorf("sprite paste preview is unavailable")
	}
	placement := g.spritePaste
	if err := g.spritePasteTargetError(placement, placement.startCol, placement.startRow); err != nil {
		g.statusMsg = "Cannot paste here: " + err.Error()
		return err
	}

	// A valid click consumes the transient placement. SetSpritePixelBlock is
	// atomic, so any model-level rejection cannot leave a partial paste behind.
	g.spritePaste = nil
	if err := g.applySpritePaste(placement, placement.startCol, placement.startRow); err != nil {
		g.statusMsg = "Paste failed: " + err.Error()
		return err
	}
	return nil
}

func (g *Game) applySpritePaste(placement *spritePastePlacement, startCol, startRow int) error {
	if err := g.spritePasteTargetError(placement, startCol, startRow); err != nil {
		return err
	}
	changed, err := g.model.SetSpritePixelBlock(startCol, startRow, placement.pixelWidth, placement.pixelHeight, placement.pixels)
	if err != nil {
		return err
	}

	g.currentSprite = startRow*g.spriteCols() + startCol
	g.gridCols, g.gridRows = placement.tileCols(), placement.tileRows()
	g.spriteShapeActive = false
	if changed {
		if err := g.refreshPreview(); err != nil {
			return fmt.Errorf("refresh pasted sprites: %w", err)
		}
		g.undo.forcePush(g.snapshot())
	}
	g.statusMsg = fmt.Sprintf("Pasted %dx%d image as %dx%d tiles", placement.pixelWidth, placement.pixelHeight, g.gridCols, g.gridRows)
	g.requestRender()
	return nil
}

// pasteSpriteImage is the non-interactive primitive retained for focused
// tests and callers that already chose a destination. User-facing clipboard
// paste goes through beginSpritePasteImage and waits for a click.
func (g *Game) pasteSpriteImage(img image.Image) error {
	placement, err := g.prepareSpritePasteImage(img)
	if err != nil {
		return err
	}
	return g.applySpritePaste(placement, placement.startCol, placement.startRow)
}
