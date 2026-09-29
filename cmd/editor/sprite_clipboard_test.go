package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
)

func clipboardTestPalette() assetsync.Palette {
	colors := make([]assetsync.RGB15, 16)
	colors[0] = assetsync.RGB15{R: 0, G: 0, B: 0}
	colors[1] = assetsync.RGB15{R: 31, G: 0, B: 0}
	colors[2] = assetsync.RGB15{R: 0, G: 31, B: 0}
	colors[3] = assetsync.RGB15{R: 0, G: 0, B: 31}
	for i := 4; i < len(colors); i++ {
		colors[i] = assetsync.RGB15{R: 31, G: 31, B: 31}
	}
	return assetsync.Palette{Name: "clipboard-test", Colors: colors}
}

func setClipboardTestPalette(g *Game) {
	g.model.ActivePalette = clipboardTestPalette()
	g.model.Project.ActivePalette = g.model.ActivePalette.Name
}

func TestPasteSpriteImageWritesSpritesheetAndSelectsImportedTiles(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	setClipboardTestPalette(g)
	g.currentSprite = 5 // tile (1,1) on a 4x4 sheet
	if err := g.model.SetSpriteFlag(5, 3, true); err != nil {
		t.Fatal(err)
	}

	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			clr := color.NRGBA{R: 250, G: 8, B: 4, A: 255}
			switch {
			case x >= 8 && y < 8:
				clr = color.NRGBA{R: 4, G: 250, B: 8, A: 255}
			case x < 8 && y >= 8:
				clr = color.NRGBA{R: 5, G: 9, B: 251, A: 255}
			case x >= 8 && y >= 8:
				clr = color.NRGBA{A: 0}
			}
			img.SetNRGBA(x, y, clr)
		}
	}

	undoBefore := len(g.undo.past)
	if err := g.pasteSpriteImage(img); err != nil {
		t.Fatal(err)
	}
	if g.currentSprite != 5 || g.gridCols != 2 || g.gridRows != 2 {
		t.Fatalf("paste selection = sprite %d, %dx%d; want sprite 5, 2x2", g.currentSprite, g.gridCols, g.gridRows)
	}
	if got := len(g.undo.past); got != undoBefore+1 {
		t.Fatalf("paste added %d undo states, want exactly 1", got-undoBefore)
	}
	if got := g.model.SpriteFlagByte(5); got&(1<<3) == 0 {
		t.Fatalf("paste cleared existing sprite flags: %#08b", got)
	}

	for _, check := range []struct {
		x, y int
		want int
	}{
		{x: 8, y: 8, want: 1},
		{x: 16, y: 8, want: 2},
		{x: 8, y: 16, want: 3},
		{x: 16, y: 16, want: 0},
	} {
		if got := g.pixelAt(check.x, check.y); got != check.want {
			t.Fatalf("sheet pixel (%d,%d) = %d, want %d", check.x, check.y, got, check.want)
		}
	}
	if !strings.Contains(g.statusMsg, "16x16") || !strings.Contains(g.statusMsg, "2x2 tiles") {
		t.Fatalf("paste status = %q", g.statusMsg)
	}
}

func TestImagePaletteMappingReservesColorZeroForTransparentPixels(t *testing.T) {
	palette := clipboardTestPalette().Colors
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})

	pixels := editor.QuantizeImageToPalette(img, palette)
	if pixels[0] != 0 {
		t.Fatalf("transparent pixel mapped to %d, want color 0", pixels[0])
	}
	if pixels[1] == 0 {
		t.Fatal("opaque pixel mapped to transparent color 0")
	}
}

func TestPasteSpriteImageRejectsInvalidDimensionsWithoutMutation(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	setClipboardTestPalette(g)
	g.currentSprite = 1
	before := g.snapshot().clone()
	undoBefore := len(g.undo.past)

	for _, size := range []image.Point{{X: 10, Y: 8}, {X: 136, Y: 8}} {
		err := g.pasteSpriteImage(image.NewNRGBA(image.Rect(0, 0, size.X, size.Y)))
		if err == nil {
			t.Fatalf("%dx%d image was accepted", size.X, size.Y)
		}
		if !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
			t.Fatalf("%dx%d rejection mutated the spritesheet", size.X, size.Y)
		}
	}
	if got := len(g.undo.past); got != undoBefore {
		t.Fatalf("rejected images added %d undo states", got-undoBefore)
	}
}

func TestPasteSpriteImageRejectsReservedAndOutOfBoundsAtomically(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	setClipboardTestPalette(g)
	img := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for x := 0; x < 16; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: 255, A: 255})
	}

	g.currentSprite = 0
	before := g.snapshot().clone()
	if err := g.pasteSpriteImage(img); err == nil || !strings.Contains(err.Error(), "sprite 0") {
		t.Fatalf("reserved paste error = %v", err)
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
		t.Fatal("reserved paste partially mutated the spritesheet")
	}

	g.currentSprite = 15 // last tile: a 2x1 block cannot fit
	before = g.snapshot().clone()
	if err := g.pasteSpriteImage(img); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("out-of-bounds paste error = %v", err)
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
		t.Fatal("out-of-bounds paste partially mutated the spritesheet")
	}
}

func TestPasteSpriteClipboardPrefersImageAndFallsBackToInternalBuffer(t *testing.T) {
	originalReader := readSpriteClipboardImagePNG
	t.Cleanup(func() { readSpriteClipboardImagePNG = originalReader })

	g := newTestGame(t, 32, 32, 2, 2)
	setClipboardTestPalette(g)
	g.currentSprite = 1
	g.hasCopied = true
	g.copiedSprite = filledCopyBuffer(8, 8, 2)
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	readSpriteClipboardImagePNG = func() ([]byte, error) { return encoded.Bytes(), nil }

	before := g.snapshot().clone()
	g.pasteSpriteClipboard()
	if g.spritePaste == nil {
		t.Fatal("system image did not start interactive paste placement")
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
		t.Fatal("starting paste placement mutated the spritesheet before click")
	}
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(8, 0); got != 1 {
		t.Fatalf("system image did not take priority: pixel = %d, want red index 1", got)
	}
	if g.spritePaste != nil {
		t.Fatal("successful click left paste placement active")
	}

	g.currentSprite = 2
	readSpriteClipboardImagePNG = func() ([]byte, error) { return nil, hostclipboard.ErrNoImage }
	g.pasteSpriteClipboard()
	if !g.dirty {
		t.Fatal("internal sprite fallback did not paint")
	}
	if got := g.model.Sheet.Sprites[len(g.model.Sheet.Sprites)-1].Pixels[0][0]; got != 2 {
		t.Fatalf("internal fallback pixel = %d, want 2", got)
	}

	readSpriteClipboardImagePNG = func() ([]byte, error) { return nil, errors.New("clipboard offline") }
	g.hasCopied = false
	g.pasteSpriteClipboard()
	if g.statusMsg != "clipboard offline" {
		t.Fatalf("clipboard failure status = %q", g.statusMsg)
	}
}

func TestSpriteImagePastePlacementMovesBeforeSingleAtomicCommit(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	setClipboardTestPalette(g)
	g.currentSprite = 5
	img := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for x := 0; x < 16; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: 255, A: 255})
	}

	before := g.snapshot().clone()
	undoBefore := len(g.undo.past)
	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	if g.spritePaste == nil || g.spritePaste.startCol != 1 || g.spritePaste.startRow != 1 {
		t.Fatalf("initial placement = %#v, want tile (1,1)", g.spritePaste)
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) || len(g.undo.past) != undoBefore {
		t.Fatal("paste preview changed project data or undo history")
	}

	g.moveSpritePastePlacement(2, 2)
	if g.spritePaste.startCol != 2 || g.spritePaste.startRow != 2 {
		t.Fatalf("moved placement = (%d,%d), want (2,2)", g.spritePaste.startCol, g.spritePaste.startRow)
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) || len(g.undo.past) != undoBefore {
		t.Fatal("moving paste preview changed project data or undo history")
	}

	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if g.currentSprite != 10 || g.gridCols != 2 || g.gridRows != 1 {
		t.Fatalf("committed selection = sprite %d, %dx%d; want sprite 10, 2x1", g.currentSprite, g.gridCols, g.gridRows)
	}
	if got := len(g.undo.past); got != undoBefore+1 {
		t.Fatalf("placement commit added %d undo states, want exactly 1", got-undoBefore)
	}
	for _, x := range []int{16, 24} {
		if got := g.pixelAt(x, 16); got != 1 {
			t.Fatalf("pasted pixel (%d,16) = %d, want red index 1", x, got)
		}
	}
}

func TestSpriteImagePastePlacementRejectsInvalidTargetAndCanCancel(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	setClipboardTestPalette(g)
	g.currentSprite = 5
	img := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	before := g.snapshot().clone()
	undoBefore := len(g.undo.past)

	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	g.moveSpritePastePlacement(3, 3)
	if err := g.commitSpritePastePlacement(); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("out-of-bounds placement error = %v", err)
	}
	if g.spritePaste == nil {
		t.Fatal("invalid click dismissed the placement instead of allowing recovery")
	}
	if !strings.Contains(g.statusMsg, "Cannot paste here") {
		t.Fatalf("invalid placement status = %q", g.statusMsg)
	}

	g.moveSpritePastePlacement(0, 0)
	if err := g.commitSpritePastePlacement(); err == nil || !strings.Contains(err.Error(), "sprite 0") {
		t.Fatalf("locked placement error = %v", err)
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) || len(g.undo.past) != undoBefore {
		t.Fatal("invalid paste placement mutated project data or undo history")
	}

	g.cancelSpritePastePlacement()
	if g.spritePaste != nil || g.statusMsg != "Paste canceled" {
		t.Fatalf("cancel left placement=%#v status=%q", g.spritePaste, g.statusMsg)
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) || len(g.undo.past) != undoBefore {
		t.Fatal("canceling paste placement mutated project data or undo history")
	}
}

func TestSpriteImagePastePlacementRejectsImageWithNoPossibleDestination(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	setClipboardTestPalette(g)
	before := g.snapshot().clone()

	err := g.beginSpritePasteImage(image.NewNRGBA(image.Rect(0, 0, 16, 16)))
	if err == nil || !strings.Contains(err.Error(), "no valid placement") {
		t.Fatalf("full-sheet placement error = %v", err)
	}
	if g.spritePaste != nil || !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
		t.Fatal("impossible paste entered placement mode or mutated the spritesheet")
	}
}
