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

// pasteSpriteClipboard gives a system image priority and keeps the original
// internal 8x8 copy buffer as a fallback when the clipboard has no image.
func (g *Game) pasteSpriteClipboard() {
	data, err := readSpriteClipboardImagePNG()
	if err == nil {
		if err := g.pasteSpritePNG(data); err != nil {
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

func (g *Game) pasteSpritePNG(data []byte) error {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("clipboard image is not valid PNG data: %w", err)
	}
	return g.pasteSpriteImage(img)
}

// pasteSpriteImage writes an image into the spritesheet starting at the
// currently selected tile, then selects the complete imported tile block.
func (g *Game) pasteSpriteImage(img image.Image) error {
	if g == nil || g.model == nil {
		return fmt.Errorf("sprite editor is unavailable")
	}
	if img == nil {
		return fmt.Errorf("clipboard image is empty")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width%8 != 0 || height%8 != 0 {
		return fmt.Errorf("clipboard image must be a positive multiple of 8 pixels (got %dx%d)", width, height)
	}
	if width > spriteClipboardMaxPixels || height > spriteClipboardMaxPixels {
		return fmt.Errorf("clipboard image must be at most %dx%d pixels (got %dx%d)", spriteClipboardMaxPixels, spriteClipboardMaxPixels, width, height)
	}
	if len(g.model.ActivePalette.Colors) < 2 {
		return fmt.Errorf("active palette needs a transparent color plus at least one paint color")
	}

	pixels := editor.QuantizeImageToPalette(img, g.model.ActivePalette.Colors)
	startRow, startCol := g.baseSpriteRowCol()
	changed, err := g.model.SetSpritePixelBlock(startCol, startRow, width, height, pixels)
	if err != nil {
		return err
	}

	g.gridCols, g.gridRows = width/8, height/8
	g.spriteShapeActive = false
	if changed {
		if err := g.refreshPreview(); err != nil {
			return fmt.Errorf("refresh pasted sprites: %w", err)
		}
		g.undo.forcePush(g.snapshot())
	}
	g.statusMsg = fmt.Sprintf("Pasted %dx%d image as %dx%d tiles", width, height, g.gridCols, g.gridRows)
	return nil
}
