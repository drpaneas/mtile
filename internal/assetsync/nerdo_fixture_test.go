package assetsync

import (
	"encoding/json"
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

// TestNerdoIdlePosesOmitAllZeroCells is the deterministic fixture check for
// Task 6: given the real nerdo spritesheet, it rebuilds the four idle poses
// with the omit-all-zero-cells algorithm and checks two properties against
// the checked-in examples/nerdo/assets/poses.json:
//
//  1. Every listed tile ID in the rebuilt poses is within 0..SpriteN-1.
//  2. Every 8x8 cell inside an idle rectangle that the rebuilt poses omit is
//     genuinely all zero (so omitting it loses no visible pixel).
//  3. The checked-in poses.json decodes to the same poses the algorithm
//     produces, so the fixture cannot silently drift from the sheet.
//
// Before examples/nerdo/assets/poses.json exists, LoadPoses returns the
// empty default and step 3 fails, which is the required red step.
func TestNerdoIdlePosesOmitAllZeroCells(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "nerdo")
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := buildNerdoPoses(assets)

	for poseIdx, pose := range want.Poses {
		for partIdx, part := range pose.Parts {
			if part.Tile < 0 || part.Tile >= assets.SpriteN {
				t.Fatalf("rebuilt pose %d part %d tile %d is outside 0..%d", poseIdx, partIdx, part.Tile, assets.SpriteN-1)
			}
		}
	}

	cols := assets.SpriteCols
	for poseIdx, rect := range nerdoIdleRects {
		sx, sy, sw, sh := rect[0], rect[1], rect[2], rect[3]
		listed := make(map[int]bool, len(want.Poses[poseIdx].Parts))
		for _, part := range want.Poses[poseIdx].Parts {
			listed[part.Tile] = true
		}
		for ty := sy; ty < sy+sh; ty += 8 {
			for tx := sx; tx < sx+sw; tx += 8 {
				id := (ty/8)*cols + (tx / 8)
				if !listed[id] && cellHasNonZero(assets, tx, ty) {
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

	got, err := LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, wantNormalized) {
		t.Fatalf("examples/nerdo/assets/poses.json does not match the rebuilt idle poses (missing file decodes empty):\n got  = %+v\n want = %+v", *got, wantNormalized)
	}
}
