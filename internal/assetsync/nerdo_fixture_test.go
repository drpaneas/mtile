package assetsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// nerdoIdleRects are the four legacy sspr idle rectangles from
// examples/nerdo/game.lua before its pspr migration: (sx, sy, sw, sh). Each
// rectangle's top-left is the pose origin, so pspr(id, 10, 10) draws at the
// same world position as the old sspr(sx, sy, sw, sh, 10, 10) call.
var nerdoIdleRects = [4][4]int{
	{0, 8, 40, 80},
	{40, 8, 40, 80},
	{80, 8, 40, 80},
	{120, 8, 40, 80},
}

var nerdoIdlePoseNames = [4]string{"idle_0", "idle_1", "idle_2", "idle_3"}

// buildNerdoPoses rebuilds the four idle poses from the flattened nerdo
// spritesheet using the omit-all-zero-cells algorithm: for every 8x8 cell in
// a rectangle, compute its spritesheet cell ID from the sheet's column
// count, and keep it as a PosePart only if any of its 64 pixels is nonzero.
func buildNerdoPoses(assets *Assets) PoseFile {
	cols := assets.SpriteCols
	file := PoseFile{Version: 1}
	for i, rect := range nerdoIdleRects {
		sx, sy, sw, sh := rect[0], rect[1], rect[2], rect[3]
		var parts []PosePartRecord
		for ty := sy; ty < sy+sh; ty += 8 {
			for tx := sx; tx < sx+sw; tx += 8 {
				id := (ty/8)*cols + (tx / 8)
				if cellHasNonZero(assets, tx, ty) {
					parts = append(parts, PosePartRecord{Tile: id, X: tx - sx, Y: ty - sy})
				}
			}
		}
		file.Poses = append(file.Poses, PoseRecord{Name: nerdoIdlePoseNames[i], Parts: parts})
	}
	return file
}

func packNerdoObjectBank(assets *Assets) (Spritesheet, PoseFile) {
	sheet := Spritesheet{Width: DefaultObjSheetW, Height: DefaultObjSheetH}
	cols := DefaultObjSheetW / 8
	file := PoseFile{Version: 1}
	for i, rect := range nerdoIdleRects {
		sx, sy, sw, sh := rect[0], rect[1], rect[2], rect[3]
		across := max(1, sheet.Width/sw)
		originX, originY := i%across*sw, i/across*sh
		var parts []PosePartRecord
		for ty := sy; ty < sy+sh; ty += 8 {
			for tx := sx; tx < sx+sw; tx += 8 {
				if !cellHasNonZero(assets, tx, ty) {
					continue
				}
				dstX := originX + (tx - sx)
				dstY := originY + ty - sy
				id := (dstY/8)*cols + dstX/8
				sheet.Sprites = append(sheet.Sprites, copySheetCell(assets, id, dstX, dstY, tx, ty))
				parts = append(parts, PosePartRecord{Tile: id, X: tx - sx, Y: ty - sy})
			}
		}
		file.Poses = append(file.Poses, PoseRecord{Name: nerdoIdlePoseNames[i], Parts: parts})
	}
	return sheet, file
}

func buildNerdoPosesFromObjectBank(assets *Assets, authored PoseFile) PoseFile {
	cols := assets.ObjSpriteCols
	file := PoseFile{Version: 1}
	for i, rect := range nerdoIdleRects {
		_, _, sw, sh := rect[0], rect[1], rect[2], rect[3]
		first := authored.Poses[i].Parts[0]
		originX, originY := first.Tile%cols*8-first.X, first.Tile/cols*8-first.Y
		var parts []PosePartRecord
		for ty := 0; ty < sh; ty += 8 {
			for tx := 0; tx < sw; tx += 8 {
				dstX := originX + tx
				dstY := originY + ty
				if !objCellHasNonZero(assets, dstX, dstY) {
					continue
				}
				id := (dstY/8)*cols + dstX/8
				parts = append(parts, PosePartRecord{Tile: id, X: tx, Y: ty})
			}
		}
		file.Poses = append(file.Poses, PoseRecord{Name: nerdoIdlePoseNames[i], Parts: parts})
	}
	return file
}

func copySheetCell(assets *Assets, id, dstX, dstY, srcX, srcY int) SpriteRecord {
	pixels := make([][]int, 8)
	for row := 0; row < 8; row++ {
		pixels[row] = make([]int, 8)
		srcRow := (srcY + row) * assets.SheetW
		for col := 0; col < 8; col++ {
			pixels[row][col] = int(assets.SheetROM[srcRow+srcX+col])
		}
	}
	return SpriteRecord{ID: id, X: dstX, Y: dstY, Width: 8, Height: 8, Pixels: pixels}
}

func TestWriteNerdoObjectBank(t *testing.T) {
	if os.Getenv("MTILE_WRITE_NERDO_OBJ") == "" {
		t.Skip("set MTILE_WRITE_NERDO_OBJ=1 to rewrite examples/nerdo object-bank assets")
	}
	dir := filepath.Join("..", "..", "examples", "nerdo")
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sheet, poses := packNerdoObjectBank(assets)
	if err := SaveObjSheet(dir, &sheet); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "assets", "poses.json"), poses); err != nil {
		t.Fatal(err)
	}
}

func TestPackNerdoObjectBankFitsFighterFrames(t *testing.T) {
	assets := &Assets{SheetW: 240, SheetH: 160, SpriteCols: 30, SheetROM: make([]byte, 240*160)}
	for i, rect := range nerdoIdleRects {
		assets.SheetROM[rect[1]*assets.SheetW+rect[0]] = byte(i + 1)
	}
	sheet, poses := packNerdoObjectBank(assets)
	if sheet.Width != DefaultObjSheetW || sheet.Height != DefaultObjSheetH || len(poses.Poses) != 4 {
		t.Fatalf("generated bank = %dx%d with%d frames", sheet.Width, sheet.Height, len(poses.Poses))
	}
	for _, sprite := range sheet.Sprites {
		if sprite.X < 0 || sprite.Y < 0 || sprite.X+8 > sheet.Width || sprite.Y+8 > sheet.Height {
			t.Fatalf("generated sprite %d exceeds its page: %+v", sprite.ID, sprite)
		}
	}
	first := poses.Poses[3].Parts[0]
	if x, y := first.Tile%(sheet.Width/8)*8-first.X, first.Tile/(sheet.Width/8)*8-first.Y; x != 120 || y != 0 {
		t.Fatalf("fourth frame origin=(%d,%d), want(120,0)", x, y)
	}
}

// cellHasNonZero reports whether any of the 64 pixels in the 8x8 cell at
// (tx, ty) of the flattened sheet is nonzero. Cells with no authored
// SpriteRecord are already zero-filled by Flatten, so this also correctly
// omits cells that were never authored.
func cellHasNonZero(assets *Assets, tx, ty int) bool {
	for y := ty; y < ty+8; y++ {
		row := y * assets.SheetW
		for x := tx; x < tx+8; x++ {
			if assets.SheetROM[row+x] != 0 {
				return true
			}
		}
	}
	return false
}

func objCellHasNonZero(assets *Assets, tx, ty int) bool {
	if assets.ObjSheetW <= 0 || tx < 0 || ty < 0 || tx+8 > assets.ObjSheetW || ty+8 > assets.ObjSheetH {
		return false
	}
	for y := ty; y < ty+8; y++ {
		row := y * assets.ObjSheetW
		for x := tx; x < tx+8; x++ {
			if assets.ObjSheetROM[row+x] != 0 {
				return true
			}
		}
	}
	return false
}

// syntheticNerdoPackingProject uses the legacy rectangle dimensions with
// deterministic artwork. The editable example project is never a test input.
// Both implicit and explicitly authored blank cells surround patterned cells;
// some painted cells contain only a bottom-right pixel to catch partial scans.
func syntheticNerdoPackingProject(t *testing.T) (string, *Assets) {
	t.Helper()
	dir := t.TempDir()
	sheet := Spritesheet{Width: 240, Height: 160}
	for frame, rect := range nerdoIdleRects {
		for y := 0; y < rect[3]/8; y++ {
			for x := 0; x < rect[2]/8; x++ {
				kind := (x + 2*y + frame) % 4
				if kind == 0 {
					continue // Implicit blank tile.
				}
				px, py := rect[0]+x*8, rect[1]+y*8
				sprite := testSprite(py/8*30+px/8, px, py, 8, 8, 0, 0)
				if kind == 2 {
					sprite.Pixels[7][7] = 1 + (frame+x+y)%15
				} else if kind == 3 {
					for row := 0; row < 8; row++ {
						for col := 0; col < 8; col++ {
							if (row+col)%3 != 0 {
								sprite.Pixels[row][col] = 1 + (frame+x+y+row+col)%15
							}
						}
					}
				}
				sheet.Sprites = append(sheet.Sprites, sprite)
			}
		}
	}
	writeTestProject(t, dir, canonicalPalette(), sheet, Map{Width: 1, Height: 1, Cells: []int{0}})
	source, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	objects, poses := packNerdoObjectBank(source)
	if err := SaveObjSheet(dir, &objects); err != nil {
		t.Fatal(err)
	}
	if err := SavePoses(dir, &poses); err != nil {
		t.Fatal(err)
	}
	return dir, source
}

// The mandatory legacy packing regression checks a synthetic project: valid
// source IDs, omission of every blank tile, preservation of every visible
// pixel, and exact pose metadata after saving and reloading the object bank.
func TestNerdoIdlePosesOmitAllZeroCells(t *testing.T) {
	dir, original := syntheticNerdoPackingProject(t)
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if assets.ObjSpriteN == 0 {
		t.Fatal("synthetic project is missing assets/objsheet.json")
	}
	// Source rectangles may move when the editor reflows fixed-size pages.
	// Recover each origin from its authored references instead of assuming a
	// particular sheet width or a horizontal strip of all four frames.
	if err := ValidateObjSheetDimensions(assets.ObjSheetW, assets.ObjSheetH); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Poses) != len(nerdoIdleRects) {
		t.Fatalf("synthetic project has %d poses, want four idle frames", len(got.Poses))
	}
	for i, pose := range got.Poses {
		if pose.Name != nerdoIdlePoseNames[i] || len(pose.Parts) == 0 {
			t.Fatalf("idle frame %d has missing parts or incorrect name %q", i, pose.Name)
		}
		first := pose.Parts[0]
		originX, originY := first.Tile%assets.ObjSpriteCols*8-first.X, first.Tile/assets.ObjSpriteCols*8-first.Y
		for _, part := range pose.Parts {
			if part.Tile < 0 || part.Tile >= assets.ObjSpriteN || part.X < 0 || part.X >= nerdoIdleRects[i][2] || part.Y < 0 || part.Y >= nerdoIdleRects[i][3] || part.X%8 != 0 || part.Y%8 != 0 || part.FlipX || part.FlipY {
				t.Fatalf("idle frame %d has invalid tile/offset/flip: %+v", i, part)
			}
			x, y := part.Tile%assets.ObjSpriteCols*8, part.Tile/assets.ObjSpriteCols*8
			if x != originX+part.X || y != originY+part.Y || !objCellHasNonZero(assets, x, y) {
				t.Fatalf("idle frame %d has a scattered or fully transparent part: %+v", i, part)
			}
		}
		if len(pose.Parts) >= nerdoIdleRects[i][2]/8*(nerdoIdleRects[i][3]/8) {
			t.Fatalf("pose %d retained blank cells", i)
		}
		// Reconstruct exclusively from registered parts and compare against
		// every original source pixel. Correct OBJ pixels alone are not enough
		// if a visible source cell was omitted from the pose's part list.
		rect := nerdoIdleRects[i]
		rendered := make([]byte, rect[2]*rect[3])
		for _, part := range pose.Parts {
			sx, sy := part.Tile%assets.ObjSpriteCols*8, part.Tile/assets.ObjSpriteCols*8
			for row := 0; row < 8; row++ {
				copy(rendered[(part.Y+row)*rect[2]+part.X:], assets.ObjSheetROM[(sy+row)*assets.ObjSheetW+sx:(sy+row)*assets.ObjSheetW+sx+8])
			}
		}
		for y := 0; y < rect[3]; y++ {
			for x := 0; x < rect[2]; x++ {
				want := original.SheetROM[(rect[1]+y)*original.SheetW+rect[0]+x]
				if rendered[y*rect[2]+x] != want {
					t.Fatalf("pose %d pixel (%d,%d) changed during packing", i, x, y)
				}
			}
		}
	}
	want := buildNerdoPosesFromObjectBank(assets, *got)

	for poseIdx, pose := range want.Poses {
		for partIdx, part := range pose.Parts {
			if part.Tile < 0 || part.Tile >= assets.ObjSpriteN {
				t.Fatalf("rebuilt pose %d part %d tile %d is outside object sheet 0..%d", poseIdx, partIdx, part.Tile, assets.ObjSpriteN-1)
			}
		}
	}

	objCols := assets.ObjSpriteCols
	for poseIdx, rect := range nerdoIdleRects {
		sw, sh := rect[2], rect[3]
		listed := make(map[int]bool, len(want.Poses[poseIdx].Parts))
		for _, part := range want.Poses[poseIdx].Parts {
			listed[part.Tile] = true
		}
		first := got.Poses[poseIdx].Parts[0]
		originX, originY := first.Tile%objCols*8-first.X, first.Tile/objCols*8-first.Y
		for ty := 0; ty < sh; ty += 8 {
			for tx := 0; tx < sw; tx += 8 {
				id := ((originY+ty)/8)*objCols + (originX+tx)/8
				if !listed[id] && objCellHasNonZero(assets, originX+tx, originY+ty) {
					t.Fatalf("pose %d cell (%d,%d) tile %d is nonzero but omitted from the rebuilt pose", poseIdx, tx, ty, id)
				}
			}
		}
	}

	// Normalize "want" through the same JSON codec LoadPoses uses so nil vs.
	// empty-slice differences from the Go builder cannot cause a spurious
	// mismatch against the file-decoded value.
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var wantNormalized PoseFile
	if err := json.Unmarshal(wantRaw, &wantNormalized); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(*got, wantNormalized) {
		t.Fatalf("saved synthetic poses do not match the rebuilt idle poses:\n got  = %+v\n want = %+v", *got, wantNormalized)
	}
}
