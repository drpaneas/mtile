package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
)

var readSpriteClipboardImagePNG = hostclipboard.ReadImagePNG

// spritePastePlacement is the transient, quantized image shown over the
// spritesheet while the user chooses its destination. It deliberately lives
// outside the model and undo snapshot: merely moving or cancelling a paste
// preview must never dirty the project.
type spritePastePlacement struct {
	pixels      []int
	nearest     []int
	ordered     []int
	source      image.Image
	palette     []assetsync.RGB15
	dither      bool
	status      [2]string
	pixelWidth  int
	pixelHeight int
	startCol    int
	startRow    int
	selection   bool
	cut         *spriteCutSource
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

type spriteCopyBuffer struct {
	width, height  int
	pixels         []int
	preferInternal bool
	clipboardHash  [32]byte
	clipboardKnown bool
	cut            *spriteCutSource
}

// A cut remains non-destructive until the destination click. The captured
// source lets a stale clipboard fail safely if its artwork changes meanwhile.
type spriteCutSource struct {
	col, row, sheetCols int
	flags               []int
}

func (b spriteCopyBuffer) pixel(x, y int) int {
	if x < 0 || y < 0 || x >= b.width || y >= b.height || len(b.pixels) != b.width*b.height {
		return 0
	}
	return b.pixels[y*b.width+x]
}

// A newly copied selection wins over the unchanged desktop clipboard. A new
// desktop image wins again, so importing another PNG never needs a reset.
func (g *Game) pasteSpriteClipboard() {
	data, err := readSpriteClipboardImagePNG()
	b := g.copiedSprite
	clipboardUnchanged := !b.clipboardKnown || sha256.Sum256(data) == b.clipboardHash
	if g.objEditing && g.hasCopied && (err != nil || (b.preferInternal && clipboardUnchanged)) {
		// If the clipboard recovers after Copy, this first paste still uses
		// the selection. Remember the recovered content so later PNGs can win.
		if !b.clipboardKnown && (err == nil || errors.Is(err, hostclipboard.ErrNoImage)) {
			g.copiedSprite.clipboardHash = sha256.Sum256(data)
			g.copiedSprite.clipboardKnown = true
		}
		if err := g.beginCopiedSpritePaste(); err != nil {
			g.statusMsg = err.Error()
		}
		g.requestRender()
		return
	}
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
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("clipboard image is not valid PNG data: %w", err)
	}
	if err := assetsync.ValidateObjSheetDimensions(config.Width, config.Height); err != nil {
		return fmt.Errorf("clipboard image: %w", err)
	}
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
	if err := assetsync.ValidateObjSheetDimensions(width, height); err != nil {
		return nil, fmt.Errorf("clipboard image: %w", err)
	}
	if g.objEditing {
		page := g.currentObjPageRect()
		if width > page.W || height > page.H {
			return nil, fmt.Errorf("%dx%d image does not fit the current %dx%d page; crop the image or choose a larger page", width, height, page.W, page.H)
		}
	}
	if len(g.model.ActivePalette.Colors) < 2 {
		return nil, fmt.Errorf("active palette needs a transparent color plus at least one paint color")
	}

	startRow, startCol := g.baseSpriteRowCol()
	pixels := editor.QuantizeImageToPalette(img, g.model.ActivePalette.Colors)
	return &spritePastePlacement{
		pixels:      pixels,
		nearest:     pixels,
		source:      img,
		palette:     slices.Clone(g.model.ActivePalette.Colors),
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
	if !g.objEditing && placement.tileCols() == cols && placement.tileRows() == rows {
		return fmt.Errorf("clipboard image covers locked sprite 0 and has no valid placement")
	}
	g.copiedSprite.cut = nil
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
	if g.objEditing {
		page := g.currentObjPageRect()
		if startCol < page.X/8 || startRow < page.Y/8 || startCol+tileCols > (page.X+page.W)/8 || startRow+tileRows > (page.Y+page.H)/8 {
			return fmt.Errorf("%dx%d tile area at (%d,%d) exceeds the current page; move the image fully inside this page", tileCols, tileRows, startCol, startRow)
		}
	}
	for row := 0; row < tileRows; row++ {
		for col := 0; col < tileCols; col++ {
			id := (startRow+row)*cols + (startCol + col)
			if g.spriteIsReserved(id) {
				if id == editor.ReservedSpriteID {
					return fmt.Errorf("sprite 0 is locked as the transparent map eraser")
				}
				return fmt.Errorf("sprite %d is locked for Mode 0 print glyphs", id)
			}
		}
	}
	return nil
}

func (g *Game) spritePastePlacementStatus() string {
	placement := g.spritePaste
	if placement == nil {
		return ""
	}
	mode := 0
	if placement.dither {
		mode = 1
	}
	if placement.status[mode] == "" {
		kind, action := "image", "paste"
		if placement.selection {
			kind = "selection"
		}
		if placement.cut != nil {
			action = "move"
		}
		message := fmt.Sprintf("Place %dx%d %s: move, click to %s, Esc to cancel", placement.pixelWidth, placement.pixelHeight, kind, action)
		if err := g.spritePasteTargetError(placement, placement.startCol, placement.startRow); err != nil {
			message = "Cannot paste here: " + err.Error()
		}
		placement.status[0], placement.status[1] = message, message
		if !placement.selection {
			placement.status[0] += " | Dither off (D)"
			placement.status[1] += " | Dither on (D)"
		}
	}
	return placement.status[mode]
}

func (g *Game) toggleSpritePasteDither() {
	placement := g.spritePaste
	if placement == nil || placement.selection {
		return
	}
	if !placement.dither && placement.ordered == nil {
		placement.ordered = editor.QuantizeImageToPaletteOrdered(placement.source, placement.palette)
	}
	placement.dither = !placement.dither
	placement.pixels = placement.nearest
	if placement.dither {
		placement.pixels = placement.ordered
	}
	g.statusMsg = g.spritePastePlacementStatus()
	g.requestRender()
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
	g.spritePaste.status = [2]string{}
	g.statusMsg = g.spritePastePlacementStatus()
	g.requestRender()
}

func (g *Game) cancelSpritePastePlacement() {
	if g == nil || (g.spritePaste == nil && g.copiedSprite.cut == nil) {
		return
	}
	g.spritePaste = nil
	g.copiedSprite.cut = nil
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

	// Keep the preview on rejection so the user can correct the destination.
	if err := g.applySpritePaste(placement, placement.startCol, placement.startRow); err != nil {
		g.statusMsg = "Paste failed: " + err.Error()
		return err
	}
	g.spritePaste = nil
	if placement.cut != nil {
		g.copiedSprite.cut = nil
	}
	return nil
}

func (g *Game) setActiveSpritePixelBlock(startCol, startRow, width, height int, pixels []int) (bool, error) {
	if g != nil && g.objEditing {
		return g.model.SetObjSpritePixelBlock(startCol, startRow, width, height, pixels)
	}
	return g.model.SetSpritePixelBlock(startCol, startRow, width, height, pixels)
}

func (g *Game) applySpritePaste(placement *spritePastePlacement, startCol, startRow int) error {
	if err := g.spritePasteTargetError(placement, startCol, startRow); err != nil {
		return err
	}
	var changed bool
	var err error
	if placement.cut != nil {
		if err := g.validateSpriteCutSource(placement); err != nil {
			return err
		}
		cut := placement.cut
		err = g.model.MoveObjSpritePixelBlock(cut.col, cut.row, placement.tileCols(), placement.tileRows(), startCol, startRow)
		changed = startCol != cut.col || startRow != cut.row
	} else {
		changed, err = g.setActiveSpritePixelBlock(startCol, startRow, placement.pixelWidth, placement.pixelHeight, placement.pixels)
	}
	if err != nil {
		return err
	}

	g.currentSprite = startRow*g.spriteCols() + startCol
	g.gridCols, g.gridRows = placement.tileCols(), placement.tileRows()
	g.spriteShapeActive = false
	if placement.selection && placement.cut == nil {
		// Duplicating artwork does not silently replace an existing pose recipe.
		g.selectedPose = -1
		g.cancelPoseNameEdit()
	} else if placement.cut != nil {
		g.syncPoseSelectionFromSprite(g.currentSprite)
	}
	if changed {
		g.dirty = true
		g.objUsage = nil
		g.invalidateAnimationPreview()
		if err := g.refreshPreview(); err != nil {
			return fmt.Errorf("refresh pasted sprites: %w", err)
		}
		g.undo.forcePush(g.snapshot())
	}
	g.statusMsg = fmt.Sprintf("Pasted %dx%d image as %dx%d tiles", placement.pixelWidth, placement.pixelHeight, g.gridCols, g.gridRows)
	if placement.selection {
		g.statusMsg = "Copied tiles pasted independently · edit them, then Register Pose to create a variant"
		if placement.cut != nil {
			g.statusMsg = "Tiles moved · registered poses and animation clips follow the new location"
		}
	}
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
