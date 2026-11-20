package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

// newTestGame builds a *Game against a fresh temp project, exercising the
// exact same newGame/editor.Open path main.go uses - but without touching
// ebiten's rendering or input, so it can run in `go test` (no display, no
// running game loop).
func newTestGame(t *testing.T, sheetW, sheetH, mapW, mapH int) *Game {
	t.Helper()
	dir := t.TempDir()
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: sheetW, Height: sheetH}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: mapW, Height: mapH, Cells: make([]int, mapW*mapH)}); err != nil {
		t.Fatal(err)
	}
	writeTestEntry(t, dir)
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func writeTestEntry(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "game.lua"), []byte("-- test project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func filledCopyBuffer(width, height, color00 int) spriteCopyBuffer {
	pixels := make([]int, width*height)
	if len(pixels) > 0 {
		pixels[0] = color00
	}
	return spriteCopyBuffer{width: width, height: height, pixels: pixels}
}

func wideColors(n int) []assetsync.RGB15 {
	colors := make([]assetsync.RGB15, n)
	for i := range colors {
		colors[i] = assetsync.RGB15{R: i % 32, G: (i * 2) % 32, B: (i * 3) % 32}
	}
	return colors
}

// TestPaintableColorCountMatchesStoredPaletteLength guards the swatch-picker
// bug found during manual smoke testing: with a 16-color palette, indices
// 16..255 in the expanded 256-wide preview are all aliases of color 0 (see
// assetsync.ExpandedPaletteColors) and Model.SetSpritePixel rejects any
// color index >= len(ActivePalette.Colors). If paintableColorCount ever
// drifted back to len(preview.Colors) (256), clicking a swatch below row 1
// would select an index that every paint attempt then fails on.
func TestPaintableColorCountMatchesStoredPaletteLength(t *testing.T) {
	g := newTestGame(t, 16, 16, 10, 10)
	if got := g.paintableColorCount(); got != 256 {
		t.Fatalf("canonical palette: paintableColorCount() = %d, want 256", got)
	}

	narrow := assetsync.Palette{Name: "narrow", Colors: wideColors(16)}
	if err := g.model.CreatePalette(narrow); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SelectPalette("narrow"); err != nil {
		t.Fatal(err)
	}
	if got := g.paintableColorCount(); got != 16 {
		t.Fatalf("16-color palette: paintableColorCount() = %d, want 16", got)
	}

	wide := assetsync.Palette{Name: "wide", Colors: wideColors(256)}
	if err := g.model.CreatePalette(wide); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SelectPalette("wide"); err != nil {
		t.Fatal(err)
	}
	if got := g.paintableColorCount(); got != 256 {
		t.Fatalf("256-color palette: paintableColorCount() = %d, want 256", got)
	}

	// Every paintable index must actually be an accepted color for
	// SetSpritePixel - the property paintableColorCount exists to guarantee.
	for _, idx := range []int{0, 255} {
		if err := g.model.SetSpritePixel(1, 0, 0, idx); err != nil {
			t.Fatalf("SetSpritePixel with paintable index %d: %v", idx, err)
		}
	}
}

func TestMapCellGetSetRoundTripsAndClampsOutOfBounds(t *testing.T) {
	g := newTestGame(t, 16, 16, 10, 8) // 2x2 sprites: valid ids are 0..3
	if g.mapActiveLayer != 0 {
		t.Fatalf("fresh map active layer = BG%d, want BG0", g.mapActiveLayer)
	}
	for layer, visible := range g.mapLayerVisible {
		if !visible {
			t.Fatalf("fresh map BG%d is hidden, want all layers visible", layer)
		}
	}

	if got := g.mapCellAt(0, 0); got != 0 {
		t.Fatalf("fresh cell = %d, want 0", got)
	}
	if !g.setMapCell(3, 4, 2) {
		t.Fatal("setMapCell on a real change reported no change")
	}
	if got := g.mapCellAt(3, 4); got != 2 {
		t.Fatalf("mapCellAt(3,4) = %d, want 2", got)
	}
	if g.setMapCell(3, 4, 2) {
		t.Fatal("setMapCell with an unchanged value reported a change")
	}

	for _, out := range [][2]int{{-1, 0}, {0, -1}, {10, 0}, {0, 8}} {
		if got := g.mapCellAt(out[0], out[1]); got != 0 {
			t.Fatalf("out-of-bounds mapCellAt%v = %d, want 0", out, got)
		}
		if g.setMapCell(out[0], out[1], 2) {
			t.Fatalf("out-of-bounds setMapCell%v reported success", out)
		}
	}
}

func TestMapLayerSelectionStartsAtBG0AndRejectsInvalidLayers(t *testing.T) {
	g := newTestGame(t, 16, 16, 10, 8)
	if g.mapActiveLayer != 0 {
		t.Fatalf("default active layer = BG%d, want BG0", g.mapActiveLayer)
	}
	if !g.selectMapLayer(3) || g.mapActiveLayer != 3 {
		t.Fatalf("select BG3 left active layer at BG%d", g.mapActiveLayer)
	}
	if g.selectMapLayer(4) || g.mapActiveLayer != 3 {
		t.Fatalf("invalid BG4 changed active layer to BG%d", g.mapActiveLayer)
	}
	if !g.selectMapLayer(0) || g.mapActiveLayer != 0 {
		t.Fatalf("select BG0 left active layer at BG%d", g.mapActiveLayer)
	}
}

func TestBackgroundLayersExampleOpensWithBG0Active(t *testing.T) {
	g, err := newGame(filepath.Join("..", "..", "examples", "background-layers"))
	if err != nil {
		t.Fatal(err)
	}
	if !g.model.Map.IsLayered() {
		t.Fatal("background-layers example is not a versioned layered map")
	}
	if g.mapActiveLayer != 0 {
		t.Fatalf("background-layers active layer = BG%d, want BG0", g.mapActiveLayer)
	}
	for layer, visible := range g.mapLayerVisible {
		if !visible {
			t.Fatalf("background-layers BG%d starts hidden", layer)
		}
	}
}

func TestMapLayerEditingIsolatedAndUndoRestoresLegacyShape(t *testing.T) {
	g := newTestGame(t, 16, 16, 4, 3)
	legacy := g.snapshot()
	g.mapActiveLayer = 2
	if !g.setMapCell(1, 1, 3) {
		t.Fatal("BG2 edit reported no change")
	}
	if !g.model.Map.IsLayered() || g.mapCellAtLayer(2, 1, 1) != 3 {
		t.Fatalf("BG2 edit did not promote/store: %#v", g.model.Map)
	}
	if g.mapCellAtLayer(0, 1, 1) != 0 || g.mapCellAtLayer(1, 1, 1) != 0 || g.mapCellAtLayer(3, 1, 1) != 0 {
		t.Fatalf("BG2 edit leaked into another plane: %#v", g.model.Map)
	}
	g.restore(legacy)
	if g.model.Map.IsLayered() || g.mapCellAtLayer(2, 1, 1) != 0 {
		t.Fatalf("undo snapshot did not restore legacy map: %#v", g.model.Map)
	}
}

func TestMapLayerToolbarUsesBackToFrontHorizontalOrderAndSeparateEyeTargets(t *testing.T) {
	layout := newMapLayout(1400, 1000)
	if layout.Layers.X < layout.Strip.X || layout.Layers.Y < layout.Strip.Y ||
		layout.Layers.X+layout.Layers.W > layout.Strip.X+layout.Strip.W ||
		layout.Layers.Y+layout.Layers.H > layout.Strip.Y+layout.Strip.H {
		t.Fatalf("layer toolbar escapes bottom strip: strip=%#v layers=%#v", layout.Strip, layout.Layers)
	}
	if layout.Canvas.X+layout.Canvas.W != 1400-mapPadding {
		t.Fatalf("map canvas does not use full workspace width: %#v", layout.Canvas)
	}
	if layout.Coordinates.X+layout.Coordinates.W > layout.Layers.X {
		t.Fatalf("map information overlaps layer toolbar: coordinates=%#v layers=%#v", layout.Coordinates, layout.Layers)
	}
	if !rectInsideIDE(layout.MoveArea, layout.Strip) || !rectInsideIDE(layout.CopyArea, layout.Strip) ||
		rectsOverlapIDE(layout.MoveArea, layout.SelectedTile) || rectsOverlapIDE(layout.MoveArea, layout.CopyArea) ||
		rectsOverlapIDE(layout.CopyArea, layout.Coordinates) {
		t.Fatalf("area tools are not isolated in the strip: move=%#v copy=%#v selected=%#v coordinates=%#v", layout.MoveArea, layout.CopyArea, layout.SelectedTile, layout.Coordinates)
	}
	if layout.LayerRows[0].X >= layout.LayerRows[1].X || layout.LayerRows[1].X >= layout.LayerRows[2].X || layout.LayerRows[2].X >= layout.LayerRows[3].X {
		t.Fatalf("horizontal order is not BG0..BG3: %#v", layout.LayerRows)
	}
	for bg := 1; bg < 4; bg++ {
		if layout.LayerRows[bg].Y != layout.LayerRows[0].Y {
			t.Fatalf("BG%d is not on the shared toolbar row: %#v", bg, layout.LayerRows)
		}
	}
	for bg := 0; bg < 4; bg++ {
		eye := layout.LayerEyes[bg]
		layer, visibility, ok := mapLayerPanelHit(layout, eye.X+eye.W/2, eye.Y+eye.H/2)
		if !ok || !visibility || layer != bg {
			t.Fatalf("BG%d eye hit = layer %d visibility=%v ok=%v", bg, layer, visibility, ok)
		}
		row := layout.LayerRows[bg]
		layer, visibility, ok = mapLayerPanelHit(layout, row.X+row.W-8, row.Y+row.H/2)
		if !ok || visibility || layer != bg {
			t.Fatalf("BG%d row hit = layer %d visibility=%v ok=%v", bg, layer, visibility, ok)
		}
	}
}

func TestEnsureEditableMapCreatesNativeDefaultWhenMapIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 16, Height: 16}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: 0, Height: 0, Cells: []int{}}); err != nil {
		t.Fatal(err)
	}
	writeTestEntry(t, dir)
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	g.prepareMapView()
	w, h := g.mapBounds()
	if w != gbaScreenWidthTiles || h != gbaScreenHeightTiles {
		t.Fatalf("mapBounds() = (%d,%d), want (%d,%d)", w, h, gbaScreenWidthTiles, gbaScreenHeightTiles)
	}
	if len(g.model.Map.Cells) != w*h {
		t.Fatalf("map cells = %d, want %d", len(g.model.Map.Cells), w*h)
	}
	if !g.dirty {
		t.Fatal("creating a default map did not mark the project dirty")
	}

	layout := newMapLayout(screenWidth, screenHeight)
	sx, sy := g.mapCam.WorldToScreen(0, 0, layout.Canvas)
	tx, ty, ok := g.mapTargetTile(layout, int(sx)+1, int(sy)+1)
	if !ok || tx != 0 || ty != 0 {
		t.Fatalf("click on fitted map origin = (%d,%d ok=%v), want (0,0 true)", tx, ty, ok)
	}
	g.currentSprite = 1
	g.placeGridSprites(0, 0)
	if got := g.mapCellAt(0, 0); got != 1 {
		t.Fatalf("placed sprite = %d, want 1", got)
	}
}

func TestEnsureEditableMapLeavesZeroSpriteSheetsEmpty(t *testing.T) {
	g := leftoverEdgeGame(t)
	g.ensureEditableMap()
	if w, h := g.mapBounds(); w != 0 || h != 0 {
		t.Fatalf("SpriteN=0 map became %dx%d, want 0x0", w, h)
	}
}

func TestNudgeMapSizeGrowsAndShrinksByOneGBAScreen(t *testing.T) {
	g := newTestGame(t, 32, 32, 10, 10)
	if !g.setMapCell(0, 0, 1) {
		t.Fatal("setup: setMapCell(0,0,1) failed")
	}
	g.nudgeMapSize(1, 0)
	w, h := g.mapBounds()
	if w != 10+gbaScreenWidthTiles || h != 10 {
		t.Fatalf("after ] = %dx%d, want %dx%d", w, h, 10+gbaScreenWidthTiles, 10)
	}
	if g.mapCellAt(0, 0) != 1 {
		t.Fatal("growing the map dropped the existing tile")
	}
	g.nudgeMapSize(0, 1)
	w, h = g.mapBounds()
	if w != 10+gbaScreenWidthTiles || h != 10+gbaScreenHeightTiles {
		t.Fatalf("after [ = %dx%d, want %dx%d", w, h, 10+gbaScreenWidthTiles, 10+gbaScreenHeightTiles)
	}
	g.nudgeMapSize(-1, 0)
	g.nudgeMapSize(0, -1)
	w, h = g.mapBounds()
	if w != 10 || h != 10 {
		t.Fatalf("after Cmd shrink = %dx%d, want 10x10", w, h)
	}
	if g.mapCellAt(0, 0) != 1 {
		t.Fatal("shrinking the map dropped the overlapping tile")
	}
}

func TestMapScreenCountRoundsUp(t *testing.T) {
	tests := []struct {
		name          string
		tiles, screen int
		want          int
	}{
		{name: "empty", tiles: 0, screen: 30, want: 0},
		{name: "one screen", tiles: 30, screen: 30, want: 1},
		{name: "partial extra screen", tiles: 31, screen: 30, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapScreenCount(tt.tiles, tt.screen); got != tt.want {
				t.Fatalf("mapScreenCount(%d, %d) = %d, want %d", tt.tiles, tt.screen, got, tt.want)
			}
		})
	}
}

func TestMapGuideClipRejectsEmptyMap(t *testing.T) {
	g := leftoverEdgeGame(t)
	_, _, _, _, ok := g.mapGuideClip(newMapLayout(screenWidth, screenHeight))
	if ok {
		t.Fatal("empty map must not report a guide clip rect")
	}
}

// TestPlaceGridSpritesStampsIndependentSpriteIDsPerCell guards the map
// editor's core placement rule: a 2x2 grid selection places four distinct
// sprite ids across four adjacent map cells (one per constituent sprite),
// not the same id repeated four times.
func TestPlaceGridSpritesStampsIndependentSpriteIDsPerCell(t *testing.T) {
	g := newTestGame(t, 32, 32, 10, 10) // 4x4 sprites (ids 0..15)
	g.gridCols = 2
	g.gridRows = 2
	g.currentSprite = 0 // top-left of the sheet, so the 2x2 fits

	g.placeGridSprites(2, 3)

	want := map[[2]int]int{
		{2, 3}: 0, {3, 3}: 1,
		{2, 4}: 4, {3, 4}: 5,
	}
	for pos, id := range want {
		if got := g.mapCellAt(pos[0], pos[1]); got != id {
			t.Fatalf("mapCellAt%v = %d, want %d", pos, got, id)
		}
	}
}

// TestPlaceGridSpritesSkipsSelectionsThatRunOffTheSheet ensures a
// multi-sprite selection anchored near the sheet's edge only stamps the
// sprite ids that actually exist, rather than wrapping or panicking.
func TestPlaceGridSpritesSkipsSelectionsThatRunOffTheSheet(t *testing.T) {
	g := newTestGame(t, 16, 16, 10, 10) // 2x2 sprites (ids 0..3)
	g.gridCols = 2
	g.gridRows = 2
	g.currentSprite = 3 // bottom-right sprite; block runs off-sheet

	g.placeGridSprites(0, 0)

	if got := g.mapCellAt(0, 0); got != 3 {
		t.Fatalf("mapCellAt(0,0) = %d, want 3 (the only in-bounds constituent sprite)", got)
	}
	for _, pos := range [][2]int{{1, 0}, {0, 1}, {1, 1}} {
		if got := g.mapCellAt(pos[0], pos[1]); got != 0 {
			t.Fatalf("mapCellAt%v = %d, want 0 (out-of-sheet constituent must not be placed)", pos, got)
		}
	}
}

func TestColorZeroHintMentionsTransparentEraser(t *testing.T) {
	if !strings.Contains(colorZeroHint, "0") || !strings.Contains(colorZeroHint, "transparent") || !strings.Contains(colorZeroHint, "eraser") {
		t.Fatalf("colorZeroHint = %q, want it to say id 0 is transparent (eraser)", colorZeroHint)
	}
}

func TestSpriteZeroHintMentionsTransparentEraser(t *testing.T) {
	if !strings.Contains(spriteZeroHint, "0") || !strings.Contains(spriteZeroHint, "transparent") || !strings.Contains(spriteZeroHint, "eraser") {
		t.Fatalf("spriteZeroHint = %q, want it to say sprite 0 is transparent (eraser)", spriteZeroHint)
	}
}

func TestReservedSpriteZeroCannotBePaintedOrPasted(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.paintPixel(editor.ReservedSpriteID, 0, 0, 9)
	if g.statusMsg != spriteZeroLockedMessage {
		t.Fatalf("paint status = %q, want %q", g.statusMsg, spriteZeroLockedMessage)
	}
	if g.dirty {
		t.Fatal("painting reserved sprite 0 marked the project dirty")
	}

	g.hasCopied = true
	g.copiedSprite = filledCopyBuffer(8, 8, 7)
	g.currentSprite = editor.ReservedSpriteID
	g.statusMsg = ""
	g.pasteCurrentSprite()
	if g.statusMsg != spriteZeroLockedMessage {
		t.Fatalf("paste status = %q, want %q", g.statusMsg, spriteZeroLockedMessage)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(0, 0); got != 0 {
		t.Fatalf("reserved sprite pixel = %d, want transparent color 0", got)
	}
}

func TestReservedMode0TextSpriteCannotBePaintedOrPasted(t *testing.T) {
	g := newTestGame(t, 240, 160, 2, 2)
	id := 480
	g.paintPixel(id, 0, 0, 9)
	if !strings.Contains(g.statusMsg, "480") || !strings.Contains(g.statusMsg, "locked") {
		t.Fatalf("paint status = %q, want locked Mode 0 font bank", g.statusMsg)
	}
	if g.dirty {
		t.Fatal("painting reserved text tile marked the project dirty")
	}

	g.hasCopied = true
	g.copiedSprite = filledCopyBuffer(8, 8, 7)
	g.currentSprite = id
	g.statusMsg = ""
	g.pasteCurrentSprite()
	if !strings.Contains(g.statusMsg, "480") || !strings.Contains(g.statusMsg, "locked") {
		t.Fatalf("paste status = %q, want locked Mode 0 font bank", g.statusMsg)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	cols := g.spriteCols()
	px, py := (id%cols)*8, (id/cols)*8
	if got := g.pixelAt(px, py); got != 0 {
		t.Fatalf("reserved text-tile pixel = %d, want unchanged 0", got)
	}
}

func TestEraserIndexIsZeroOnly(t *testing.T) {
	if !isEraserIndex(0) {
		t.Fatal("index 0 must be the eraser")
	}
	for _, id := range []int{-1, 1, 2, 255} {
		if isEraserIndex(id) {
			t.Fatalf("index %d must not be the eraser", id)
		}
	}
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	g := newTestGame(t, 16, 16, 10, 10)
	g.currentSprite = 2
	g.currentColor = 5
	g.gridCols = 2
	g.gridRows = 1
	if err := g.model.SetSpritePixel(2, 1, 1, 5); err != nil {
		t.Fatal(err)
	}
	// snapshot() aliases g.model.Sheet's slice backing arrays (by design - see
	// undoStack.push/reset, which clone before storing); a test that compares
	// state across a later mutation must clone explicitly, exactly as the
	// real undo stack does, or it would silently observe the mutation too.
	before := g.snapshot().clone()

	if err := g.model.SetSpritePixel(2, 1, 1, 9); err != nil {
		t.Fatal(err)
	}
	g.currentSprite, g.currentColor, g.gridCols, g.gridRows = 0, 0, 1, 1

	g.restore(before)

	if g.currentSprite != 2 || g.currentColor != 5 || g.gridCols != 2 || g.gridRows != 1 {
		t.Fatalf("restore did not reapply UI state: sprite=%d color=%d grid=%dx%d", g.currentSprite, g.currentColor, g.gridCols, g.gridRows)
	}
	// Sprite 2 on this 16x16 (2x2-sprite) sheet sits at col=2%2=0, row=2/2=1.
	if got := g.pixelAt(0*8+1, 1*8+1); got != 5 {
		t.Fatalf("restore did not reapply sheet pixel: got %d, want 5", got)
	}
}

func TestMapRestorePreservesPaintingSpriteSelection(t *testing.T) {
	g := newTestGame(t, 32, 32, 10, 10)
	g.mode = modeMap
	g.currentSprite = 6
	g.currentColor = 5
	g.gridCols = 2
	g.gridRows = 1

	previous := g.snapshot().clone()
	previous.CurrentSprite = 1
	previous.CurrentColor = 2
	previous.GridCols = 1
	previous.GridRows = 2

	g.restore(previous)

	if g.currentSprite != 6 || g.currentColor != 5 || g.gridCols != 2 || g.gridRows != 1 {
		t.Fatalf("map undo changed painting selection: sprite=%d color=%d grid=%dx%d", g.currentSprite, g.currentColor, g.gridCols, g.gridRows)
	}
}

func TestSpriteCountHelpersMatchSheetDimensions(t *testing.T) {
	g := newTestGame(t, 32, 16, 5, 5) // 4x2 sprites
	if got := g.spriteCols(); got != 4 {
		t.Fatalf("spriteCols() = %d, want 4", got)
	}
	if got := g.spriteRows(); got != 2 {
		t.Fatalf("spriteRows() = %d, want 2", got)
	}
	if got := g.spriteN(); got != 8 {
		t.Fatalf("spriteN() = %d, want 8", got)
	}
	if w, h := g.mapBounds(); w != 5 || h != 5 {
		t.Fatalf("mapBounds() = (%d,%d), want (5,5)", w, h)
	}
}

func TestZeroSpriteColumnsRemainSafeInEditor(t *testing.T) {
	dir := t.TempDir()
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 7, Height: 16}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: 0, Height: 0}); err != nil {
		t.Fatal(err)
	}
	writeTestEntry(t, dir)
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.spriteCols(); got != 0 {
		t.Fatalf("spriteCols() = %d, want 0", got)
	}
	if got := g.spriteN(); got != 0 {
		t.Fatalf("spriteN() = %d, want 0", got)
	}
	if g.setMapCell(0, 0, 99) {
		t.Fatal("setMapCell accepted an id when SpriteN is zero")
	}
	g.copyCurrentSprite()
	if g.hasCopied {
		t.Fatal("copyCurrentSprite marked an unavailable sprite as copied")
	}
	// These paths are called by normal map-mode drawing and keyboard handling.
	// They must no-op rather than divide by zero on a valid leftover-edge sheet.
	g.paintPixel(0, 0, 0, 1)
	g.drawSpriteThumb(nil, 0, 0, 0, 8, 8)
	// drawMapTiles is the map-mode loop that invokes drawSpriteThumb for each
	// cell. A nil screen is safe here because the zero-column guard returns
	// before any rendering primitive is reached.
	g.drawMapTiles(nil, newMapLayout(screenWidth, screenHeight))
}

func TestMapCellRejectsOutOfRangeSpriteID(t *testing.T) {
	g := newTestGame(t, 16, 16, 10, 8)
	if g.setMapCell(0, 0, 4) {
		t.Fatal("setMapCell accepted id 4 for a four-sprite sheet")
	}
	if g.setMapCell(0, 0, -1) {
		t.Fatal("setMapCell accepted a negative sprite id")
	}
	if !g.setMapCell(0, 0, 3) || g.mapCellAt(0, 0) != 3 {
		t.Fatal("setMapCell rejected the maximum valid sprite id")
	}
}

func TestUndoRestorePersistsPaletteAndProjectTogether(t *testing.T) {
	g := newTestGame(t, 16, 16, 4, 4)
	before := g.snapshot().clone()
	original := g.model.ActivePalette.Colors[1]
	edited := assetsync.RGB15{R: 31, G: 0, B: 0}
	if err := g.model.SetActiveColor(1, edited); err != nil {
		t.Fatal(err)
	}
	if err := g.persist(); err != nil {
		t.Fatal(err)
	}
	if g.statusMsg != "" {
		t.Fatalf("persist of edited palette failed: %s", g.statusMsg)
	}
	g.restore(before)
	if err := g.persist(); err != nil {
		t.Fatal(err)
	}
	if g.statusMsg != "" {
		t.Fatalf("persist after palette undo failed: %s", g.statusMsg)
	}
	p, err := assetsync.LoadPalette(g.model.Dir, assetsync.CanonicalPaletteName)
	if err != nil {
		t.Fatal(err)
	}
	if p.Colors[1] != original {
		t.Fatalf("palette undo was not persisted: got %#v want %#v", p.Colors[1], original)
	}
}

func TestPersistWritesEditedPaletteFileAndReloadsIt(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	path := filepath.Join(g.model.Dir, "assets", "palettes", assetsync.CanonicalPaletteName+".json")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("canonical palette file existed before any save")
	}

	want := assetsync.RGB15{R: 31, G: 0, B: 16}
	if err := g.model.SetActiveColor(1, want); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("color edit wrote a palette file before persist")
	}

	if err := g.persist(); err != nil {
		t.Fatal(err)
	}
	if g.statusMsg != "" {
		t.Fatalf("persist after color edit failed: %s", g.statusMsg)
	}
	disk, err := assetsync.LoadPalette(g.model.Dir, assetsync.CanonicalPaletteName)
	if err != nil {
		t.Fatal(err)
	}
	if disk.Colors[1] != want {
		t.Fatalf("saved palette color[1] = %#v, want %#v", disk.Colors[1], want)
	}

	reopened, err := newGame(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.model.ActivePalette.Colors[1] != want {
		t.Fatalf("reopened editor color[1] = %#v, want %#v", reopened.model.ActivePalette.Colors[1], want)
	}
}

func TestFlagToggleDoesNotWriteSheetUntilPersist(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.toggleFlag(0)
	if g.model.SpriteFlagByte(0)&1 == 0 {
		t.Fatal("toggleFlag did not set bit 0 in memory")
	}
	sheet, err := assetsync.LoadSpritesheet(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Sprites) != 0 {
		t.Fatal("toggleFlag wrote spritesheet.json before persist")
	}
	if err := g.persist(); err != nil {
		t.Fatal(err)
	}
	sheet, err = assetsync.LoadSpritesheet(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sprite := range sheet.Sprites {
		if sprite.ID == 0 {
			found = true
			if sprite.Flags&1 == 0 {
				t.Fatal("persist did not write flag bit 0")
			}
		}
	}
	if !found {
		t.Fatal("persist did not write sprite 0")
	}
}

func TestSelectPaletteClampsCurrentColorToStoredLength(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	wide := assetsync.Palette{Name: "wide", Colors: wideColors(256)}
	if err := g.model.CreatePalette(wide); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SelectPalette("wide"); err != nil {
		t.Fatal(err)
	}
	g.currentColor = 255
	g.clampCurrentColor()
	if g.currentColor != 255 {
		t.Fatalf("256-color clamp = %d, want 255", g.currentColor)
	}
	narrow := assetsync.Palette{Name: "narrow", Colors: wideColors(16)}
	if err := g.model.CreatePalette(narrow); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SelectPalette("narrow"); err != nil {
		t.Fatal(err)
	}
	g.clampCurrentColor()
	if g.currentColor < 0 || g.currentColor >= g.paintableColorCount() {
		t.Fatalf("after switching to 16-color palette, currentColor=%d is not paintable", g.currentColor)
	}
}

func leftoverEdgeGame(t *testing.T) *Game {
	t.Helper()
	dir := t.TempDir()
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 7, Height: 16}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: 0, Height: 0}); err != nil {
		t.Fatal(err)
	}
	writeTestEntry(t, dir)
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestFlagBitsStayClearWhenSpriteNIsZero(t *testing.T) {
	g := leftoverEdgeGame(t)
	all, any := g.flagBitState(0)
	if all || any {
		t.Fatalf("leftover-edge sheet drew flag 0 as all=%v any=%v, want both false", all, any)
	}
}

func TestToggleFlagNoOpDoesNotRecordUndo(t *testing.T) {
	g := leftoverEdgeGame(t)
	past := len(g.undo.past)
	g.toggleFlag(0)
	if len(g.undo.past) != past {
		t.Fatal("no-op flag toggle on a SpriteN==0 sheet recorded an undo step")
	}
}

func TestToggleFlagIsIndependentlyUndoable(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.toggleFlag(0)
	g.toggleFlag(1)
	if g.model.SpriteFlagByte(0)&0b11 != 0b11 {
		t.Fatal("expected both flag bits 0 and 1 set before undo")
	}

	s, ok := g.undo.undo()
	if !ok {
		t.Fatal("undo after second flag toggle was empty")
	}
	g.restore(s)
	if got := g.model.SpriteFlagByte(0); got != 1 {
		t.Fatalf("undo of bit 1 left flags=%#b, want bit 0 only", got)
	}

	s, ok = g.undo.undo()
	if !ok {
		t.Fatal("undo after first flag toggle was empty")
	}
	g.restore(s)
	if got := g.model.SpriteFlagByte(0); got != 0 {
		t.Fatalf("undo of bit 0 left flags=%#b, want 0", got)
	}
}

func TestToggleFlagMixedSelectionSetsAll(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.gridCols = 2
	g.gridRows = 2
	g.currentSprite = 0
	if err := g.model.SetSpriteFlag(0, 0, true); err != nil {
		t.Fatal(err)
	}
	// Sprite 0 is set while sprite 1 is clear. The tri-state selection must
	// become all-set; it must not use the top-left sprite as the toggle value
	// and clear both.
	g.toggleFlag(0)
	for _, id := range []int{0, 1, 2, 3} {
		if got := g.model.SpriteFlagByte(id) & 1; got == 0 {
			t.Fatalf("mixed selection left sprite %d flag 0 clear", id)
		}
	}

	// Once every selected sprite is set, the next toggle clears every one.
	g.toggleFlag(0)
	for _, id := range []int{0, 1, 2, 3} {
		if got := g.model.SpriteFlagByte(id) & 1; got != 0 {
			t.Fatalf("all-set selection left sprite %d flag 0 set", id)
		}
	}
}

// TestCopyPasteCopiesPixelsToAnotherSprite is the copy/paste happy path:
// paint a distinctive pattern on one sprite, copy it, select a different
// sprite, paste, and confirm every pixel landed - the failure-path test
// below only exercises the rejection branch, not that a normal paste
// actually writes pixels.
func TestCopyPasteCopiesPixelsToAnotherSprite(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2) // 4x4 sprites (ids 0..15)
	g.currentSprite = 1
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			color := (x + y) % 16
			if err := g.model.SetSpritePixel(1, x, y, color); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	g.copyCurrentSprite()
	if !g.hasCopied {
		t.Fatal("copyCurrentSprite did not record the source sprite")
	}

	g.currentSprite = 5
	g.pasteCurrentSprite()
	if g.statusMsg != "" {
		t.Fatalf("paste of a valid buffer reported an error: %s", g.statusMsg)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	// Sprite 5 on this 4x4-sprite sheet sits at col=5%4=1, row=5/4=1.
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want := (x + y) % 16
			if got := g.pixelAt(1*8+x, 1*8+y); got != want {
				t.Fatalf("pasted pixel (%d,%d) = %d, want %d", x, y, got, want)
			}
		}
	}
}

func TestCopyPasteCopiesMultiTileSelection(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2) // 4x4 sprites (ids 0..15)
	colors := map[int]int{1: 3, 2: 4, 5: 5, 6: 6}
	for id, color := range colors {
		if err := g.model.SetSpritePixel(id, 0, 0, color); err != nil {
			t.Fatal(err)
		}
		if err := g.model.SetSpritePixel(id, 7, 7, color); err != nil {
			t.Fatal(err)
		}
	}
	g.currentSprite = 1
	g.gridCols, g.gridRows = 2, 2
	g.copyCurrentSprite()
	if !g.hasCopied || g.copiedSprite.width != 16 || g.copiedSprite.height != 16 {
		t.Fatalf("copy buffer = %dx%d, want 16x16", g.copiedSprite.width, g.copiedSprite.height)
	}
	if g.copiedSprite.pixel(0, 0) != 3 || g.copiedSprite.pixel(8, 0) != 4 || g.copiedSprite.pixel(0, 8) != 5 || g.copiedSprite.pixel(8, 8) != 6 {
		t.Fatalf("copy missed the 2x2 selection: (0,0)=%d (8,0)=%d (0,8)=%d (8,8)=%d",
			g.copiedSprite.pixel(0, 0), g.copiedSprite.pixel(8, 0), g.copiedSprite.pixel(0, 8), g.copiedSprite.pixel(8, 8))
	}

	g.currentSprite = 5
	g.pasteCurrentSprite()
	if g.statusMsg != "" {
		t.Fatalf("paste of a 2x2 selection reported an error: %s", g.statusMsg)
	}
	if g.gridCols != 2 || g.gridRows != 2 {
		t.Fatalf("paste selection = %dx%d, want 2x2", g.gridCols, g.gridRows)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		x, y, want int
	}{
		{8, 8, 3},
		{16, 8, 4},
		{8, 16, 5},
		{16, 16, 6},
		{15, 15, 3},
		{23, 15, 4},
	} {
		if got := g.pixelAt(check.x, check.y); got != check.want {
			t.Fatalf("pasted pixel (%d,%d) = %d, want %d", check.x, check.y, got, check.want)
		}
	}
}

func TestEraseSpriteSelectionClearsTilesAndSkipsReserved(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	if err := g.model.SetSpritePixel(1, 0, 0, 9); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetSpritePixel(2, 0, 0, 8); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 1
	g.gridCols, g.gridRows = 2, 1
	g.eraseCurrentSpriteSelection()
	if g.statusMsg != "" {
		t.Fatalf("erase reported %q", g.statusMsg)
	}
	if !g.dirty {
		t.Fatal("erase did not mark the project dirty")
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(8, 0); got != 0 {
		t.Fatalf("erased sprite 1 pixel = %d, want 0", got)
	}
	if got := g.pixelAt(16, 0); got != 0 {
		t.Fatalf("erased sprite 2 pixel = %d, want 0", got)
	}

	if err := g.model.SetSpritePixel(1, 0, 0, 9); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 0
	g.gridCols, g.gridRows = 2, 1
	g.statusMsg = ""
	g.eraseCurrentSpriteSelection()
	if g.statusMsg != "" {
		t.Fatalf("mixed erase reported %q, want skip of reserved sprite 0", g.statusMsg)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(0, 0); got != 0 {
		t.Fatalf("sprite 0 pixel = %d, want unchanged 0", got)
	}
	if got := g.pixelAt(8, 0); got != 0 {
		t.Fatalf("mixed erase left sprite 1 = %d, want 0", got)
	}

	g.currentSprite = 0
	g.gridCols, g.gridRows = 1, 1
	g.statusMsg = ""
	g.dirty = false
	g.eraseCurrentSpriteSelection()
	if g.statusMsg != spriteZeroLockedMessage {
		t.Fatalf("reserved erase status = %q, want %q", g.statusMsg, spriteZeroLockedMessage)
	}
	if g.dirty {
		t.Fatal("reserved-only erase marked the project dirty")
	}
}

func TestCopyPasteAndEraseWorkOnObjectSheetIncludingTileZero(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetObjSpritePixel(0, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetObjSpritePixel(1, 0, 0, 5); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 0
	g.gridCols, g.gridRows = 2, 1
	g.copyCurrentSprite()
	if !g.hasCopied || g.copiedSprite.width != 16 || g.copiedSprite.pixel(0, 0) != 4 || g.copiedSprite.pixel(8, 0) != 5 {
		t.Fatalf("OBJ copy buffer = %dx%d pixels %d,%d", g.copiedSprite.width, g.copiedSprite.height, g.copiedSprite.pixel(0, 0), g.copiedSprite.pixel(8, 0))
	}

	g.currentSprite = 2
	g.pasteCurrentSprite()
	if g.statusMsg != "" {
		t.Fatalf("OBJ paste error: %s", g.statusMsg)
	}
	if got := spriteSheetPixel(*g.model.ObjSheet, 16, 0); got != 4 {
		t.Fatalf("pasted OBJ tile 2 = %d, want 4", got)
	}
	if got := spriteSheetPixel(*g.model.ObjSheet, 24, 0); got != 5 {
		t.Fatalf("pasted OBJ tile 3 = %d, want 5", got)
	}

	g.gridCols, g.gridRows = 2, 1
	g.eraseCurrentSpriteSelection()
	if g.statusMsg != "" {
		t.Fatalf("OBJ erase error: %s", g.statusMsg)
	}
	if spriteSheetPixel(*g.model.ObjSheet, 16, 0) != 0 || spriteSheetPixel(*g.model.ObjSheet, 24, 0) != 0 {
		t.Fatal("OBJ erase left pixels behind")
	}
}

// TestPasteRejectsColorsOutsideNarrowerActivePalette guards a gap where
// pasteCurrentSprite applied its buffer pixel-by-pixel with no preflight.
// The copy buffer is plain Game UI state, not part of the validated sheet,
// so it can go stale: copy a pixel painted under a wide (256-color) palette,
// repaint over it so the sheet is compatible with a 16-color palette again
// (SelectPalette itself refuses an incompatible switch, so the sheet must
// already fit before switching), switch palettes, then paste the still-wide
// buffer. Before this fix, SetSpritePixel would silently reject just the
// out-of-range pixels mid-loop, leaving the sprite partially overwritten
// with no clear signal that the paste was incomplete.
func TestPasteRejectsColorsOutsideNarrowerActivePalette(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	wide := assetsync.Palette{Name: "wide", Colors: wideColors(256)}
	if err := g.model.CreatePalette(wide); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SelectPalette("wide"); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetSpritePixel(1, 0, 0, 200); err != nil {
		t.Fatal(err)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 1
	g.copyCurrentSprite()
	if !g.hasCopied || g.copiedSprite.pixel(0, 0) != 200 {
		t.Fatalf("copy did not capture color 200: hasCopied=%v pixel=%d", g.hasCopied, g.copiedSprite.pixel(0, 0))
	}

	// Undo the out-of-range paint so the sheet is compatible with a
	// 16-color palette again; the stale copy buffer still holds color 200.
	// Canonical aurora is 256 colors, so switching back to it would still
	// accept index 200. A narrower preset is required to make the paste invalid.
	if err := g.model.SetSpritePixel(1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	narrow := assetsync.Palette{Name: "narrow", Colors: wideColors(16)}
	if err := g.model.CreatePalette(narrow); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SelectPalette("narrow"); err != nil {
		t.Fatal(err)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	g.pasteCurrentSprite()
	if g.statusMsg == "" {
		t.Fatal("paste of an out-of-range color did not report an error")
	}
	if got := g.pixelAt(8, 0); got != 0 {
		t.Fatalf("paste partially applied an invalid buffer: pixelAt(8,0)=%d, want unchanged 0", got)
	}
}

func TestGridSizeZeroDoesNotDivideByZero(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.gridCols = 0
	g.gridRows = 0
	if g.gridPixelsW() <= 0 || g.gridPixelsH() <= 0 {
		t.Fatalf("gridPixels = %dx%d, want a positive fallback", g.gridPixelsW(), g.gridPixelsH())
	}
	_ = g.canvasPixelScale()
	g.paintPixel(0, 0, 0, 1)
}

func TestBracketKeysCycleWidthAndHeightIndependently(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	if g.gridPixelsW() != 8 || g.gridPixelsH() != 8 {
		t.Fatalf("default grid = %dx%d, want 8x8", g.gridPixelsW(), g.gridPixelsH())
	}

	g.nudgeGridRows(1)
	if g.gridPixelsW() != 8 || g.gridPixelsH() != 16 {
		t.Fatalf("after [ = %dx%d, want 8x16", g.gridPixelsW(), g.gridPixelsH())
	}
	g.nudgeGridRows(1)
	if g.gridPixelsW() != 8 || g.gridPixelsH() != 24 {
		t.Fatalf("after second [ = %dx%d, want 8x24", g.gridPixelsW(), g.gridPixelsH())
	}
	g.nudgeGridRows(-1)
	if g.gridPixelsW() != 8 || g.gridPixelsH() != 16 {
		t.Fatalf("after Cmd/Ctrl+[ = %dx%d, want 8x16", g.gridPixelsW(), g.gridPixelsH())
	}

	g.nudgeGridCols(1)
	if g.gridPixelsW() != 16 || g.gridPixelsH() != 16 {
		t.Fatalf("after ] = %dx%d, want 16x16", g.gridPixelsW(), g.gridPixelsH())
	}
	g.nudgeGridCols(-1)
	if g.gridPixelsW() != 8 || g.gridPixelsH() != 16 {
		t.Fatalf("after Cmd/Ctrl+] = %dx%d, want 8x16", g.gridPixelsW(), g.gridPixelsH())
	}

	g.gridCols = gridCellsMax
	g.nudgeGridCols(1)
	if g.gridPixelsW() != 8 {
		t.Fatalf("width wrap past 128 = %d, want 8", g.gridPixelsW())
	}
	g.gridRows = gridCellsMax
	g.nudgeGridRows(1)
	if g.gridPixelsH() != 8 {
		t.Fatalf("height wrap past 128 = %d, want 8", g.gridPixelsH())
	}

	g.gridCols = gridCellsMin
	g.nudgeGridCols(-1)
	if g.gridPixelsW() != 128 {
		t.Fatalf("width wrap below 8 = %d, want 128", g.gridPixelsW())
	}
	g.gridRows = gridCellsMin
	g.nudgeGridRows(-1)
	if g.gridPixelsH() != 128 {
		t.Fatalf("height wrap below 8 = %d, want 128", g.gridPixelsH())
	}
}

func TestSpritesheetDragSelectionBuildsRectangleAroundAnchor(t *testing.T) {
	tests := []struct {
		name                                       string
		anchorCol, anchorRow, cursorCol, cursorRow int
		wantCol, wantRow, wantCols, wantRows       int
	}{
		{name: "single tile", anchorCol: 4, anchorRow: 5, cursorCol: 4, cursorRow: 5, wantCol: 4, wantRow: 5, wantCols: 1, wantRows: 1},
		{name: "down and right", anchorCol: 3, anchorRow: 4, cursorCol: 5, cursorRow: 6, wantCol: 3, wantRow: 4, wantCols: 3, wantRows: 3},
		{name: "up and left", anchorCol: 3, anchorRow: 4, cursorCol: 1, cursorRow: 2, wantCol: 1, wantRow: 2, wantCols: 3, wantRows: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, row, cols, rows, ok := spritesheetDragSelection(tt.anchorCol, tt.anchorRow, tt.cursorCol, tt.cursorRow, 30, 20)
			if !ok || col != tt.wantCol || row != tt.wantRow || cols != tt.wantCols || rows != tt.wantRows {
				t.Fatalf("selection = (%d,%d %dx%d ok=%t), want (%d,%d %dx%d true)", col, row, cols, rows, ok, tt.wantCol, tt.wantRow, tt.wantCols, tt.wantRows)
			}
		})
	}
}

func TestSpritesheetDragSelectionClampsToMaximumAndSheetEdges(t *testing.T) {
	tests := []struct {
		name                                       string
		anchorCol, anchorRow, cursorCol, cursorRow int
		wantCol, wantRow, wantCols, wantRows       int
	}{
		{name: "far up and left", anchorCol: 20, anchorRow: 10, cursorCol: -100, cursorRow: -100, wantCol: 5, wantRow: 0, wantCols: 16, wantRows: 11},
		{name: "far down and right", anchorCol: 2, anchorRow: 2, cursorCol: 100, cursorRow: 100, wantCol: 2, wantRow: 2, wantCols: 16, wantRows: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, row, cols, rows, ok := spritesheetDragSelection(tt.anchorCol, tt.anchorRow, tt.cursorCol, tt.cursorRow, 30, 20)
			if !ok || col != tt.wantCol || row != tt.wantRow || cols != tt.wantCols || rows != tt.wantRows {
				t.Fatalf("selection = (%d,%d %dx%d ok=%t), want (%d,%d %dx%d true)", col, row, cols, rows, ok, tt.wantCol, tt.wantRow, tt.wantCols, tt.wantRows)
			}
		})
	}
	if _, _, _, _, ok := spritesheetDragSelection(-1, 0, 0, 0, 30, 20); ok {
		t.Fatal("selection accepted an anchor outside the spritesheet")
	}
}

func TestSpriteNumberUsesThreeDigits(t *testing.T) {
	for _, tt := range []struct {
		id   int
		want string
	}{{0, "000"}, {1, "001"}, {37, "037"}, {599, "599"}} {
		if got := spriteNumberText(tt.id); got != tt.want {
			t.Errorf("spriteNumberText(%d) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestSpriteCanvasTitleShowsSSprSourceForLargerSelections(t *testing.T) {
	tests := []struct {
		name                    string
		id, cols, width, height int
		wantText                string
		wantHighlights          string
		wantSection             string
	}{
		{name: "single tile", id: 33, cols: 30, width: 8, height: 8, wantText: "SPRITE 033", wantHighlights: "033", wantSection: "SPRITE"},
		{name: "sixteen square", id: 33, cols: 30, width: 16, height: 16, wantText: "METASPRITE (24,8,16,16)", wantHighlights: "24|8|16|16", wantSection: "METASPRITE"},
		{name: "tall selection", id: 33, cols: 30, width: 8, height: 24, wantText: "METASPRITE (24,8,8,24)", wantHighlights: "24|8|8|24", wantSection: "METASPRITE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := spriteCanvasTitleParts(tt.id, tt.cols, tt.width, tt.height)
			text := ""
			highlights := []string{}
			section := ""
			for _, part := range parts {
				text += part.text
				if part.highlight {
					highlights = append(highlights, part.text)
				}
				if !part.secondary {
					section += part.text
				}
			}
			if text != tt.wantText {
				t.Errorf("title text = %q, want %q", text, tt.wantText)
			}
			if got := strings.Join(highlights, "|"); got != tt.wantHighlights {
				t.Errorf("highlighted values = %q, want %q", got, tt.wantHighlights)
			}
			if section != tt.wantSection {
				t.Errorf("section title = %q, want %q", section, tt.wantSection)
			}
		})
	}
}

func TestCanvasPixelScaleKeepsSquarePixelsForNonSquareGrid(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	g.gridCols = 4
	g.gridRows = 1
	scale := g.canvasPixelScale()
	want := math.Min(float64(canvasRect.W)/32, float64(canvasRect.H)/8)
	if diff(scale, want) > 0.001 {
		t.Fatalf("canvasPixelScale for 32x8 = %g, want %g", scale, want)
	}
}

func TestSquarePixelSelectionConsumesEntireCanvas(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := &Game{gridCols: 4, gridRows: 4}

	if got := g.canvasPixelBounds(0, 0, g.gridPixelsW(), g.gridPixelsH()); got != canvasRect {
		t.Fatalf("32x32 canvas bounds = %#v, want full canvas %#v", got, canvasRect)
	}
	col, row, ok := g.spriteCanvasCellAt(canvasRect.X+canvasRect.W-1, canvasRect.Y+canvasRect.H-1, false)
	if !ok || col != 31 || row != 31 {
		t.Fatalf("bottom-right canvas pixel = (%d,%d ok=%t), want (31,31 true)", col, row, ok)
	}
}

func TestSpriteCanvasZoomKeepsCenterPixelStableAndReturnsToFit(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := &Game{gridCols: 4, gridRows: 4, spriteTool: spriteToolLine, spriteShapeActive: true}

	centerX, centerY := canvasRect.X+canvasRect.W/2, canvasRect.Y+canvasRect.H/2
	centerCol, centerRow, ok := g.spriteCanvasCellAt(centerX, centerY, false)
	if !ok {
		t.Fatal("fit canvas center is outside the sprite")
	}
	if !g.changeSpriteCanvasZoom(1) {
		t.Fatal("zoom in did not change the canvas view")
	}
	if g.spriteTool != spriteToolMove || g.spriteShapeActive {
		t.Fatalf("zoom selected tool %s with shapeActive=%t, want MOVE with no active shape", g.spriteTool, g.spriteShapeActive)
	}
	zoomedCol, zoomedRow, ok := g.spriteCanvasCellAt(centerX, centerY, false)
	if !ok || zoomedCol != centerCol || zoomedRow != centerRow {
		t.Fatalf("zoom moved center pixel from (%d,%d) to (%d,%d ok=%t)", centerCol, centerRow, zoomedCol, zoomedRow, ok)
	}
	for g.changeSpriteCanvasZoom(1) {
	}
	if g.spriteCanvasZoomLevel != spriteCanvasZoomMax || spriteCanvasZoomFactor(g.spriteCanvasZoomLevel) != 4 {
		t.Fatalf("maximum zoom = level %d factor %g, want level %d factor 4", g.spriteCanvasZoomLevel, spriteCanvasZoomFactor(g.spriteCanvasZoomLevel), spriteCanvasZoomMax)
	}
	for g.changeSpriteCanvasZoom(-1) {
	}
	if g.spriteCanvasZoomLevel != spriteCanvasZoomMin || g.spriteCanvasPanX != 0 || g.spriteCanvasPanY != 0 {
		t.Fatalf("fit view = level %d pan (%g,%g), want level 0 pan (0,0)", g.spriteCanvasZoomLevel, g.spriteCanvasPanX, g.spriteCanvasPanY)
	}
	if got := g.canvasPixelBounds(0, 0, 32, 32); got != canvasRect {
		t.Fatalf("restored fit bounds = %#v, want %#v", got, canvasRect)
	}
}

func TestSpriteCanvasZoomPanCannotExposeOutsideSelection(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := &Game{gridCols: 4, gridRows: 4}
	g.spriteCanvasZoomLevel = spriteCanvasZoomMax
	g.spriteCanvasPanX, g.spriteCanvasPanY = 1e6, 1e6
	g.clampSpriteCanvasPan()
	scale := g.canvasPixelScale()
	wantMaxX := float64(g.gridPixelsW()) - float64(canvasRect.W)/scale
	wantMaxY := float64(g.gridPixelsH()) - float64(canvasRect.H)/scale
	if diff(g.spriteCanvasPanX, wantMaxX) > 0.001 || diff(g.spriteCanvasPanY, wantMaxY) > 0.001 {
		t.Fatalf("clamped pan = (%g,%g), want (%g,%g)", g.spriteCanvasPanX, g.spriteCanvasPanY, wantMaxX, wantMaxY)
	}
	col, row, ok := g.spriteCanvasCellAt(canvasRect.X+canvasRect.W-1, canvasRect.Y+canvasRect.H-1, false)
	if !ok || col != 31 || row != 31 {
		t.Fatalf("panned bottom-right pixel = (%d,%d ok=%t), want (31,31 true)", col, row, ok)
	}
	g.spriteCanvasPanX, g.spriteCanvasPanY = -100, -100
	g.clampSpriteCanvasPan()
	if g.spriteCanvasPanX != 0 || g.spriteCanvasPanY != 0 {
		t.Fatalf("negative pan survived clamp: (%g,%g)", g.spriteCanvasPanX, g.spriteCanvasPanY)
	}
}

func TestMoveToolPanUsesScreenDragAndStaysBounded(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := &Game{gridCols: 4, gridRows: 4, spriteTool: spriteToolMove, spriteCanvasZoomLevel: spriteCanvasZoomMax}

	g.panSpriteCanvasBy(-100, -80)
	if g.spriteCanvasPanX <= 0 || g.spriteCanvasPanY <= 0 {
		t.Fatalf("left/up drag produced pan (%g,%g), want positive world offset", g.spriteCanvasPanX, g.spriteCanvasPanY)
	}
	g.panSpriteCanvasBy(1_000_000, 1_000_000)
	if g.spriteCanvasPanX != 0 || g.spriteCanvasPanY != 0 {
		t.Fatalf("large right/down drag escaped start bound: (%g,%g)", g.spriteCanvasPanX, g.spriteCanvasPanY)
	}
}

func TestPlaceGridSpritesNonSquareSelection(t *testing.T) {
	g := newTestGame(t, 32, 32, 10, 10) // 4x4 sprites
	g.gridCols = 2
	g.gridRows = 1
	g.currentSprite = 0
	g.placeGridSprites(1, 1)
	if got := g.mapCellAt(1, 1); got != 0 {
		t.Fatalf("mapCellAt(1,1) = %d, want 0", got)
	}
	if got := g.mapCellAt(2, 1); got != 1 {
		t.Fatalf("mapCellAt(2,1) = %d, want 1", got)
	}
	if got := g.mapCellAt(1, 2); got != 0 {
		t.Fatalf("mapCellAt(1,2) = %d, want 0 (1-row selection must not stamp the next row)", got)
	}
}

func TestSheetCameraFillsEntireAvailableCanvas(t *testing.T) {
	g := newTestGame(t, 240, 160, 10, 10)
	canvas := g.sheetGridCanvas()
	cols, rows := g.spriteCols(), g.spriteRows()
	sheetW := g.sheetCam.TileSize() * float64(cols)
	sheetH := g.sheetCam.TileSizeY() * float64(rows)
	if diff(sheetW, float64(canvas.W)) > 0.001 || diff(sheetH, float64(canvas.H)) > 0.001 {
		t.Fatalf("30x20 sheet %gx%g, want exact grid canvas %dx%d", sheetW, sheetH, canvas.W, canvas.H)
	}
	wantCanvas := editor.Rect{
		X: sheetPanelRect.X + 10,
		Y: spritePanelContentY(sheetPanelRect),
		W: sheetPanelRect.W - 20,
		H: sheetPanelRect.H - spritePanelContentOffset() - 10,
	}
	if canvas != wantCanvas {
		t.Fatalf("grid canvas = %#v, want full panel interior %#v", canvas, wantCanvas)
	}
	sheetX, sheetY := g.sheetCam.WorldToScreen(0, 0, canvas)
	if diff(sheetX, float64(canvas.X)) > 1 || diff(sheetY, float64(canvas.Y)) > 1 {
		t.Fatalf("fitted sheet starts at %g,%g, want panel origin %d,%d", sheetX, sheetY, canvas.X, canvas.Y)
	}
}

func TestSpriteSheetRefitsWhenInterfaceTextSizeChanges(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := newTestGame(t, 240, 160, 10, 10)
	g.preferences = defaultEditorPreferences()
	for _, size := range []interfaceTextSize{textCompact, textComfort, textLarge} {
		g.preferences.TextSize = size
		g.applyPreferences()
		canvas := g.sheetGridCanvas()
		if diff(g.sheetCam.TileSizeY()*float64(g.spriteRows()), float64(canvas.H)) > 0.001 {
			t.Fatalf("%s text left the spritesheet fitted to the old content height", size)
		}
	}
	// Preferences unrelated to layout must not reset the user's sheet view.
	g.sheetCam.Zoom = 2
	g.sheetCam.ZoomY = 2
	g.preferences.PreviewVolume = 50
	g.applyPreferences()
	if g.sheetCam.Zoom != 2 || g.sheetCam.ZoomY != 2 {
		t.Fatal("changing preview volume refitted the spritesheet")
	}
}

func TestTinySheetHonorsComfortableZoomCeiling(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	if diff(g.sheetCam.TileSize(), g.sheetCam.TileSizeY()) > 0.001 {
		t.Fatalf("tile %gx%g, want square", g.sheetCam.TileSize(), g.sheetCam.TileSizeY())
	}
	canvas := g.sheetGridCanvas()
	sheetW := g.sheetCam.TileSize() * float64(g.spriteCols())
	sheetH := g.sheetCam.TileSizeY() * float64(g.spriteRows())
	if sheetW > float64(canvas.W)+0.001 || sheetH > float64(canvas.H)+0.001 {
		t.Fatalf("fitted sheet %gx%g overflowed panel %dx%d", sheetW, sheetH, canvas.W, canvas.H)
	}
	if diff(sheetW, 256) > 0.001 || diff(sheetH, 256) > 0.001 {
		t.Fatalf("compact 2x2 sheet %gx%g, want a comfortable 256x256 editing area", sheetW, sheetH)
	}
}

// TestGraphicsAndAudioExamplesKeepTheSameNativeSheetLayout guards the exact
// regression in graphics and audio projects: adding audio assets must
// not collapse an otherwise empty 240x160 spritesheet to one giant tile.
func TestGraphicsAndAudioExamplesKeepTheSameNativeSheetLayout(t *testing.T) {
	for _, example := range []string{"mapdemo", "native-audio"} {
		t.Run(example, func(t *testing.T) {
			dir := filepath.Join("..", "..", "examples", example)
			if example == "native-audio" {
				dir = newNativeMusicTestGame(t).model.Dir
				if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 240, Height: 160}); err != nil {
					t.Fatal(err)
				}
			}
			g, err := newGame(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := g.spriteCols(), gbaScreenWidthTiles; got != want {
				t.Fatalf("sprite columns = %d, want %d", got, want)
			}
			if got, want := g.spriteRows(), gbaScreenHeightTiles; got != want {
				t.Fatalf("sprite rows = %d, want %d", got, want)
			}
			canvas := g.sheetGridCanvas()
			sheetW := g.sheetCam.TileSize() * float64(g.spriteCols())
			sheetH := g.sheetCam.TileSizeY() * float64(g.spriteRows())
			if diff(sheetW, float64(canvas.W)) > 0.001 || diff(sheetH, float64(canvas.H)) > 0.001 {
				t.Fatalf("native sheet %gx%g, want full canvas %dx%d", sheetW, sheetH, canvas.W, canvas.H)
			}
		})
	}
}

func TestMapLayoutSitsAboveStatusBar(t *testing.T) {
	layout := newMapLayout(screenWidth, screenHeight)
	if bottom := layout.Strip.Y + layout.Strip.H; bottom > statusBarRect.Y {
		t.Fatalf("map strip bottom %d overlaps status bar Y=%d", bottom, statusBarRect.Y)
	}
	if bottom := layout.SelectedTile.Y + layout.SelectedTile.H; bottom > statusBarRect.Y {
		t.Fatalf("selected-tile preview bottom %d overlaps status bar Y=%d", bottom, statusBarRect.Y)
	}
	if bottom := layout.MoveArea.Y + layout.MoveArea.H; bottom > statusBarRect.Y {
		t.Fatalf("move-area tool bottom %d overlaps status bar Y=%d", bottom, statusBarRect.Y)
	}
	if bottom := layout.CopyArea.Y + layout.CopyArea.H; bottom > statusBarRect.Y {
		t.Fatalf("copy-area tool bottom %d overlaps status bar Y=%d", bottom, statusBarRect.Y)
	}
	if layout.Canvas.Y+layout.Canvas.H > layout.Strip.Y {
		t.Fatalf("map canvas bottom %d overlaps strip Y=%d", layout.Canvas.Y+layout.Canvas.H, layout.Strip.Y)
	}
}

func TestGraphicsWorkspaceKeepsDesktopEditingScale(t *testing.T) {
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	if screenWidth < 1920 || screenHeight < 1200 {
		t.Fatalf("default editor surface = %dx%d, want at least 1920x1200", screenWidth, screenHeight)
	}
	if canvasRect.W < 320 || canvasRect.H < 320 {
		t.Fatalf("sprite canvas = %dx%d, want at least 320x320", canvasRect.W, canvasRect.H)
	}
	if paletteSwatchWidth < 16 || paletteSwatchHeight < 16 {
		t.Fatalf("palette swatch = %dx%dpx, want at least 16px per axis", paletteSwatchWidth, paletteSwatchHeight)
	}
	wantPaletteRight := spritePalettePanelRect.X + spritePalettePanelRect.W - spritePanelPadding
	if got := paletteRect.X + paletteRect.W; got != wantPaletteRight {
		t.Fatalf("palette right edge = %d, want panel content edge %d", got, wantPaletteRight)
	}
}

func TestPaletteMetaShowsColorCountWithoutDuplicateSpriteID(t *testing.T) {
	if got, want := paletteMetaText(256), "256 COLORS"; got != want {
		t.Fatalf("paletteMetaText() = %q, want %q", got, want)
	}
}

func TestPaletteColorNumberTracksSelectedColor(t *testing.T) {
	tests := []struct {
		colorID int
		want    string
	}{
		{colorID: 0, want: "000"},
		{colorID: 1, want: "001"},
		{colorID: 37, want: "037"},
		{colorID: 255, want: "255"},
	}
	for _, tt := range tests {
		if got := paletteColorNumberText(tt.colorID); got != tt.want {
			t.Errorf("paletteColorNumberText(%d) = %q, want %q", tt.colorID, got, tt.want)
		}
	}
}

func TestDesktopSpriteInspectorBalancesFlagsAndColorPicker(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)

	if colorPickerTitle != "CHANGE COLOR" {
		t.Fatalf("color picker title = %q, want CHANGE COLOR", colorPickerTitle)
	}
	if want := spritePanelContentOffset() + spriteButtonHeight + spritePanelPadding + 2*(uiSmallTextHeight+6); spriteFlagsPanelRect.H != want {
		t.Fatalf("flags panel height = %d, want %d", spriteFlagsPanelRect.H, want)
	}
	lastFlag := flagCheckboxRect(7)
	if bottomSpace := spriteFlagsPanelRect.Y + spriteFlagsPanelRect.H - (lastFlag.Y + lastFlag.H); bottomSpace < 12 {
		t.Fatalf("flags panel bottom space = %dpx, want at least 12px", bottomSpace)
	}
	preview := colorPickerPreviewRect()
	titleX, _ := colorPickerTitlePosition()
	if titleRight := titleX + uiTextWidth(colorPickerTitle); titleRight+8 > preview.X {
		t.Fatalf("color title ends at %d and crowds preview at %d", titleRight, preview.X)
	}
	commonTitleX := spriteRailRect.X + spritePanelTitleInsetX
	for name, panel := range map[string]editor.Rect{
		"sprite": spriteCanvasPanelRect, "tools": spriteToolsPanelRect, "palette": spritePalettePanelRect,
		"flags": spriteFlagsPanelRect, "color": spritePickerPanelRect,
	} {
		x, y := spritePanelTitlePosition(panel)
		if x != commonTitleX || y != panel.Y+spritePanelTitleInsetY {
			t.Fatalf("%s title origin = (%d,%d), want (%d,%d)", name, x, y, commonTitleX, panel.Y+spritePanelTitleInsetY)
		}
	}
	if uiTextHeight <= uiSmallTextHeight {
		t.Fatalf("section height %d must exceed secondary height %d", uiTextHeight, uiSmallTextHeight)
	}

	setEditorLayout(minimumScreenWidth, minimumScreenHeight)
	lastFlag = flagCheckboxRect(7)
	if bottomSpace := spriteFlagsPanelRect.Y + spriteFlagsPanelRect.H - (lastFlag.Y + lastFlag.H); bottomSpace < 12 {
		t.Fatalf("compact flags panel bottom space = %dpx, want at least 12px", bottomSpace)
	}
	preview = colorPickerPreviewRect()
	compactTitleX, compactTitleY := colorPickerTitlePosition()
	if titleRight := compactTitleX + uiTextWidth(colorPickerTitle); titleRight > pickerRect.X+pickerRect.W {
		t.Fatalf("compact color title ends at %d outside picker right edge %d", titleRight, pickerRect.X+pickerRect.W)
	}
	if preview.Y < compactTitleY+uiTextHeight+8 {
		t.Fatalf("compact preview Y = %d, want below title ending at %d", preview.Y, compactTitleY+uiTextHeight)
	}
	paletteMeta := paletteMetaText(256)
	if metaRight := paletteMetaX(paletteMeta) + smallTextWidth(paletteMeta); metaRight > paletteRect.X+paletteRect.W {
		t.Fatalf("compact palette metadata ends at %d outside palette column %d", metaRight, paletteRect.X+paletteRect.W)
	}
}

func TestPaletteSelectionRingSurroundsEntireSwatch(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	swatch := swatchRect(17)
	ring := paletteSelectionRect(17)
	if ring.X != swatch.X-1 || ring.Y != swatch.Y-1 ||
		ring.X+ring.W != swatch.X+swatch.W+1 || ring.Y+ring.H != swatch.Y+swatch.H+1 {
		t.Fatalf("selection ring %#v does not fully surround swatch %#v", ring, swatch)
	}
	if paletteSelectionThickness != 2 {
		t.Fatalf("selection thickness = %d, want 2px", paletteSelectionThickness)
	}
}

func TestEditorLayoutUsesAdditionalLargeWindowSpace(t *testing.T) {
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := newTestGame(t, 240, 160, 30, 20)
	baseline := newNativeMusicLayout()

	w, h := g.Layout(2560, 1440)
	if w != 2560 || h != 1440 || screenWidth != w || screenHeight != h {
		t.Fatalf("expanded layout = returned %dx%d globals %dx%d, want 2560x1440", w, h, screenWidth, screenHeight)
	}
	expanded := newNativeMusicLayout()
	if expanded.Grid.W <= baseline.Grid.W || expanded.Grid.H <= baseline.Grid.H {
		t.Fatalf("expanded piano roll = %dx%d, baseline %dx%d", expanded.Grid.W, expanded.Grid.H, baseline.Grid.W, baseline.Grid.H)
	}
	if expanded.Arrangement.W <= baseline.Arrangement.W {
		t.Fatalf("expanded arrangement width = %d, baseline %d", expanded.Arrangement.W, baseline.Arrangement.W)
	}
	if sheetPanelRect.Y+sheetPanelRect.H > statusBarRect.Y {
		t.Fatalf("expanded spritesheet bottom %d overlaps status bar %d", sheetPanelRect.Y+sheetPanelRect.H, statusBarRect.Y)
	}
}

func TestEditorLayoutNeverFractionallyDownscalesWindow(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := &Game{}
	for _, size := range [][2]int{{1031, 1080}, {1280, 900}, {1512, 982}} {
		width, height := g.Layout(size[0], size[1])
		if width != size[0] || height != size[1] {
			t.Fatalf("Layout(%d, %d) = %d, %d; whole-frame scaling would blur text", size[0], size[1], width, height)
		}
	}
}

func TestMapOpensAtReadableScaleAndDoesNotRefitLongLevels(t *testing.T) {
	g := newTestGame(t, 240, 160, 4095, 20)
	g.prepareMapView()
	if got := g.mapCam.Zoom; got != editor.MaxZoom {
		t.Fatalf("map zoom = %g, want readable default %g", got, editor.MaxZoom)
	}
	if got := g.mapCam.TileSize(); got != 32 {
		t.Fatalf("map tile = %gpx, want 32px", got)
	}

	// Re-entering the map editor must preserve a user's zoom instead of
	// fitting all 4095 columns into the viewport.
	g.mapCam.Zoom = 2
	g.mapCam.ZoomY = 0
	g.prepareMapView()
	if got := g.mapCam.Zoom; got != 2 {
		t.Fatalf("map re-entry changed zoom to %g, want 2", got)
	}
}

func TestColorToolsSitUnderSpriteCanvas(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1541, 1058}, {defaultScreenWidth, defaultScreenHeight}} {
		setEditorLayout(size[0], size[1])
		panels := []editor.Rect{spriteCanvasPanelRect, spriteToolsPanelRect, spritePalettePanelRect, spriteFlagsPanelRect}
		if spritePickerPanelRect.W > 0 {
			panels = append(panels, spritePickerPanelRect)
		}
		for i, panel := range panels {
			if !rectInsideIDE(panel, spriteRailRect) {
				t.Fatalf("%dx%d panel %d escapes sprite rail: %#v in %#v", size[0], size[1], i, panel, spriteRailRect)
			}
			if i > 0 && rectsOverlapIDE(panels[i-1], panel) {
				t.Fatalf("%dx%d sprite panels overlap: %#v and %#v", size[0], size[1], panels[i-1], panel)
			}
		}

		for name, pair := range map[string][2]editor.Rect{
			"canvas":  {canvasRect, spriteCanvasPanelRect},
			"tools":   {spriteToolsRect, spriteToolsPanelRect},
			"palette": {paletteRect, spritePalettePanelRect},
			"flags":   {flagsRect, spriteFlagsPanelRect},
		} {
			if !rectInsideIDE(pair[0], pair[1]) {
				t.Fatalf("%dx%d %s escapes its panel: %#v in %#v", size[0], size[1], name, pair[0], pair[1])
			}
		}
		pickerParent := spritePickerPanelRect
		if pickerParent.W == 0 {
			pickerParent = spritePalettePanelRect
			if rectsOverlapIDE(paletteRect, pickerRect) {
				t.Fatalf("%dx%d compact palette overlaps color picker", size[0], size[1])
			}
		}
		if !rectInsideIDE(pickerRect, pickerParent) {
			t.Fatalf("%dx%d picker escapes its panel: %#v in %#v", size[0], size[1], pickerRect, pickerParent)
		}
		lastTool := spriteToolButtonRect(spriteToolButtonCount - 1)
		if !rectInsideIDE(lastTool, spriteToolsRect) {
			t.Fatalf("%dx%d last sprite tool escapes toolbar: %#v in %#v", size[0], size[1], lastTool, spriteToolsRect)
		}
		if sheetPanelRect.X < spriteRailRect.X+spriteRailRect.W || rectsOverlapIDE(sheetPanelRect, spriteRailRect) {
			t.Fatalf("%dx%d spritesheet overlaps inspector rail", size[0], size[1])
		}
	}
}

func TestIntermediateHeightUsesCompactSpriteInspector(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	setEditorLayout(1541, 1058)
	if spritePickerPanelRect.W != 0 {
		t.Fatalf("1058px-high inspector kept standalone picker panel %#v; want compact palette composition", spritePickerPanelRect)
	}
	if pickerRect.H < 100 {
		t.Fatalf("compact color picker height = %d, want usable content area", pickerRect.H)
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestScaledRunAbutsWithoutGaps(t *testing.T) {
	origin := 10.0
	scale := 15.0 / 8.0
	prevEnd := 0
	for i := 0; i < 8; i++ {
		start, size := scaledRun(origin, i, scale)
		if size < 1 {
			t.Fatalf("run %d has size %d", i, size)
		}
		if i > 0 && start != prevEnd {
			t.Fatalf("run %d starts at %d, want %d (gap or overlap)", i, start, prevEnd)
		}
		prevEnd = start + size
	}
}

func TestSpriteThumbRectsShareFractionalTileEdgesWithoutOverlap(t *testing.T) {
	canvas := editor.Rect{X: 101, Y: 73, W: 1596, H: 952}
	camera := editor.Camera{X: 0.125, Y: 0.25, Zoom: 6.65, ZoomY: 5.95}
	topLeft := sheetCameraRect(camera, canvas, 1, 1, 1, 1)
	right := sheetCameraRect(camera, canvas, 2, 1, 1, 1)
	below := sheetCameraRect(camera, canvas, 1, 2, 1, 1)
	if topLeft.X+topLeft.W != right.X {
		t.Fatalf("horizontal thumbnail bounds overlap or gap: %#v then %#v", topLeft, right)
	}
	if topLeft.Y+topLeft.H != below.Y {
		t.Fatalf("vertical thumbnail bounds overlap or gap: %#v then %#v", topLeft, below)
	}
}

func TestSpritesheetSelectionOutlineStaysInsideCanvasAndSelection(t *testing.T) {
	canvas := editor.Rect{X: 100, Y: 70, W: 300, H: 200}
	selection := editor.Rect{X: 95, Y: 65, W: 120, H: 90}
	outline, ok := spritesheetSelectionOutlineRect(selection, canvas)
	if !ok {
		t.Fatal("visible selection did not produce an outline")
	}
	visible, _ := intersectEditorRects(selection, canvas)
	if outline.X <= visible.X || outline.Y <= visible.Y || outline.X+outline.W >= visible.X+visible.W || outline.Y+outline.H >= visible.Y+visible.H {
		t.Fatalf("outline %#v is not strictly inside visible selection %#v", outline, visible)
	}
}

func TestObjEditingPaintsTileZeroAndPersists(t *testing.T) {
	g := newTestGame(t, 128, 64, 16, 16)
	if g.objEditing || g.model.ObjSheet != nil {
		t.Fatal("new project should start on the BG sheet")
	}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if !g.objEditing || g.model.ObjSheet == nil {
		t.Fatal("OBJ toggle should create the object sheet")
	}
	if g.spriteCols() != assetsync.DefaultObjSheetW/8 || g.spriteRows() != assetsync.DefaultObjSheetH/8 {
		t.Fatalf("object sheet grid = %dx%d", g.spriteCols(), g.spriteRows())
	}
	if g.spriteIsReserved(0) || g.spriteIsReserved(480) {
		t.Fatal("object-sheet cells must be paint-legal")
	}
	if !g.paintPixel(0, 0, 0, 3) {
		t.Fatal("object tile 0 should be paintable")
	}
	if err := g.persist(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := editor.Open(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ObjSheet == nil {
		t.Fatal("objsheet.json was not saved")
	}
	found := false
	for _, sprite := range reloaded.ObjSheet.Sprites {
		if sprite.ID != 0 {
			continue
		}
		found = true
		if sprite.Pixels[0][0] != 3 {
			t.Fatalf("object tile 0 pixel = %d", sprite.Pixels[0][0])
		}
	}
	if !found {
		t.Fatal("object tile 0 was not authored")
	}
}
