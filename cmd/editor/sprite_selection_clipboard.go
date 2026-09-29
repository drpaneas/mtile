package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/hajimehoshi/ebiten/v2"
)

// Read just the selected rectangle, without searching every sprite for every
// pixel or quantizing palette indices through RGB.
func spriteSelectionPixels(sheet *assetsync.Spritesheet, col, row, width, height int) []int {
	pixels := make([]int, width*height)
	left, top := col*8, row*8
	for i := len(sheet.Sprites) - 1; i >= 0; i-- {
		s := sheet.Sprites[i]
		for y := max(top, s.Y); y < min(top+height, s.Y+s.Height); y++ {
			for x := max(left, s.X); x < min(left+width, s.X+s.Width); x++ {
				pixels[(y-top)*width+x-left] = s.Pixels[y-s.Y][x-s.X]
			}
		}
	}
	return pixels
}

func spriteSelectionFlags(sheet *assetsync.Spritesheet, col, row, cols, rows int) []int {
	flags := make([]int, cols*rows)
	stride := sheet.Width / 8
	for _, s := range sheet.Sprites {
		x, y := s.ID%stride-col, s.ID/stride-row
		if x >= 0 && y >= 0 && x < cols && y < rows {
			flags[y*cols+x] = s.Flags
		}
	}
	return flags
}

func (g *Game) captureSpriteCopyClipboard() {
	if !g.objEditing {
		return
	}
	g.copiedSprite.preferInternal = true
	data, err := readSpriteClipboardImagePNG()
	g.copiedSprite.clipboardHash = sha256.Sum256(data)
	g.copiedSprite.clipboardKnown = err == nil || errors.Is(err, hostclipboard.ErrNoImage)
}

func (g *Game) cutCurrentSprite() {
	if !g.objEditing || g.model == nil || g.model.ObjSheet == nil {
		return
	}
	g.copyCurrentSprite()
	if !g.hasCopied {
		return
	}
	row, col := g.baseSpriteRowCol()
	g.copiedSprite.cut = &spriteCutSource{
		col: col, row: row, sheetCols: g.spriteCols(),
		flags: spriteSelectionFlags(g.model.ObjSheet, col, row, g.copiedSprite.width/8, g.copiedSprite.height/8),
	}
	g.statusMsg = "Cut ready · Cmd/Ctrl+P or V to place · source moves on click · Esc cancels"
	g.requestRender()
}

func (g *Game) beginCopiedSpritePaste() error {
	b := &g.copiedSprite
	if !g.hasCopied || b.width <= 0 || b.height <= 0 || b.width%8 != 0 || b.height%8 != 0 || len(b.pixels) != b.width*b.height {
		return fmt.Errorf("select tiles and copy or cut them first")
	}
	if g.objEditing {
		p := g.currentObjPageRect()
		if b.width > p.W || b.height > p.H {
			return fmt.Errorf("%dx%d selection does not fit the current %dx%d page", b.width, b.height, p.W, p.H)
		}
	}
	row, col := g.baseSpriteRowCol()
	placement := &spritePastePlacement{
		pixels: slices.Clone(b.pixels), pixelWidth: b.width, pixelHeight: b.height,
		startCol: col, startRow: row, selection: true, cut: b.cut,
	}
	if placement.cut != nil {
		if err := g.validateSpriteCutSource(placement); err != nil {
			return err
		}
	}
	g.spritePaste = placement
	g.spriteShapeActive, g.sheetSelectionDragging = false, false
	g.statusMsg = g.spritePastePlacementStatus()
	g.requestRender()
	return nil
}

func (g *Game) validateSpriteCutSource(p *spritePastePlacement) error {
	c := p.cut
	if c == nil {
		return nil
	}
	if !g.objEditing || g.model == nil || g.model.ObjSheet == nil || c.sheetCols != g.spriteCols() ||
		c.col < 0 || c.row < 0 || c.col+p.tileCols() > g.spriteCols() || c.row+p.tileRows() > g.spriteRows() {
		return fmt.Errorf("cut source is no longer available; select and cut the tiles again")
	}
	sheet := g.model.ObjSheet
	if !slices.Equal(p.pixels, spriteSelectionPixels(sheet, c.col, c.row, p.pixelWidth, p.pixelHeight)) ||
		!slices.Equal(c.flags, spriteSelectionFlags(sheet, c.col, c.row, p.tileCols(), p.tileRows())) {
		return fmt.Errorf("cut source changed; select and cut the tiles again")
	}
	return nil
}

func (g *Game) drawSpriteCutSource(screen *ebiten.Image, canvas editor.Rect) {
	c := g.copiedSprite.cut
	if !g.objEditing || c == nil {
		return
	}
	region := sheetCameraRect(g.sheetCam, canvas, c.col, c.row, g.copiedSprite.width/8, g.copiedSprite.height/8)
	if outline, ok := spritesheetSelectionOutlineRect(region, g.sheetContentClip(canvas)); ok {
		strokeRect(screen, outline, colorHighlight)
	}
}
