package assetsync

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadObjSheetMissingFileIsNil(t *testing.T) {
	dir := t.TempDir()
	sheet, err := LoadObjSheet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sheet != nil {
		t.Fatalf("missing objsheet.json = %+v, want nil", sheet)
	}
}

func TestLoadObjSheetRejectsInvalidJSONAndDimensions(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "objsheet.json")
	if err := os.WriteFile(path, []byte(`{"width":8,`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadObjSheet(dir); err == nil || !strings.Contains(err.Error(), "objsheet.json") {
		t.Fatalf("invalid JSON error = %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"width":9,"height":8,"sprites":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadObjSheet(dir); err == nil || !strings.Contains(err.Error(), "objsheet.json") || !strings.Contains(err.Error(), "8") {
		t.Fatalf("non-multiple-of-8 error = %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"width":524296,"height":8,"sprites":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadObjSheet(dir); err == nil || !strings.Contains(err.Error(), "objsheet.json") || !strings.Contains(err.Error(), "65536") {
		t.Fatalf("axis overflow error = %v", err)
	}
}

func TestSaveAndLoadDefaultObjSheet(t *testing.T) {
	dir := t.TempDir()
	sheet := &Spritesheet{Width: DefaultObjSheetW, Height: DefaultObjSheetH}
	if err := SaveObjSheet(dir, sheet); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadObjSheet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Width != DefaultObjSheetW || loaded.Height != DefaultObjSheetH {
		t.Fatalf("loaded = %+v", loaded)
	}
	if DefaultObjSheetW != 512 || DefaultObjSheetH != 320 {
		t.Fatalf("defaults = %dx%d, want 512x320", DefaultObjSheetW, DefaultObjSheetH)
	}
}

func TestSaveObjSheetAcceptsMaxSquare(t *testing.T) {
	dir := t.TempDir()
	sheet := &Spritesheet{Width: 2048, Height: 2048}
	if err := SaveObjSheet(dir, sheet); err != nil {
		t.Fatal(err)
	}
}

func TestLoadUsesObjectSheetAsPoseBank(t *testing.T) {
	dir := t.TempDir()
	sheet := Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{testSprite(0, 0, 0, 8, 8, 1, 0)}}
	mapData := Map{Width: 1, Height: 1, Cells: []int{0}}
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	obj := Spritesheet{Width: 16, Height: 8, Sprites: []SpriteRecord{testSprite(1, 8, 0, 8, 8, 7, 99)}}
	if err := SaveObjSheet(dir, &obj); err != nil {
		t.Fatal(err)
	}
	posesJSON := `{"version":1,"poses":[{"name":"idle","parts":[{"tile":1,"x":0,"y":0}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "poses.json"), []byte(posesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.ObjSheetW != 16 || assets.ObjSheetH != 8 || assets.ObjSpriteN != 2 {
		t.Fatalf("object sheet metadata = %dx%d n=%d", assets.ObjSheetW, assets.ObjSheetH, assets.ObjSpriteN)
	}
	if len(assets.ObjSheetROM) != 16*8 || assets.ObjSheetROM[8] != 7 {
		t.Fatalf("object sheet pixels len=%d pixel=%d", len(assets.ObjSheetROM), assets.ObjSheetROM[8])
	}
	if len(assets.PoseParts) != 1 || assets.PoseParts[0].Tile != 1 {
		t.Fatalf("pose parts = %+v", assets.PoseParts)
	}
	if assets.FlagROM[0] != 0 {
		t.Fatalf("object-sheet flags leaked onto the BG sheet: %#x", assets.FlagROM[0])
	}
}

func TestFlattenPosesRejectsTileOutsideObjectSheet(t *testing.T) {
	dir := t.TempDir()
	sheet := Spritesheet{Width: 16, Height: 8, Sprites: []SpriteRecord{testSprite(1, 8, 0, 8, 8, 1, 0)}}
	mapData := Map{Width: 1, Height: 1, Cells: []int{0}}
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	obj := Spritesheet{Width: 8, Height: 8}
	if err := SaveObjSheet(dir, &obj); err != nil {
		t.Fatal(err)
	}
	posesJSON := `{"version":1,"poses":[{"name":"idle","parts":[{"tile":1,"x":0,"y":0}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "poses.json"), []byte(posesJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected pose tile to fail against the object sheet")
	}
	for _, want := range []string{"poses.json", "object sheet", "tile", "1"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestLoadWithoutObjSheetLeavesZeroObjectBank(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if assets.ObjSpriteN != 0 || len(assets.ObjSheetROM) != 0 {
		t.Fatalf("unexpected object bank: n=%d rom=%d", assets.ObjSpriteN, len(assets.ObjSheetROM))
	}
}

func TestObjectSheetAcceptsAllUint16TileIDsInAnyAspectRatio(t *testing.T) {
	for _, size := range [][2]int{{1032, 8}, {2048, 2048}, {MaxObjSheetCells * 8, 8}, {8, MaxObjSheetCells * 8}} {
		if err := ValidateObjSheetDimensions(size[0], size[1]); err != nil {
			t.Fatalf("%dx%d: %v", size[0], size[1], err)
		}
	}
	for _, size := range [][2]int{{0, 8}, {8, 7}, {(MaxObjSheetCells + 1) * 8, 8}, {2048, 2056}} {
		if err := ValidateObjSheetDimensions(size[0], size[1]); err == nil {
			t.Fatalf("accepted %dx%d", size[0], size[1])
		}
	}
}

func TestCompactObjectSheetRetainsLastOf65536PaintedTiles(t *testing.T) {
	sheet := make([]byte, MaxObjSheetCells*64)
	for id := 0; id < MaxObjSheetCells; id++ {
		// Two bytes encode all 65,536 IDs; the marker keeps ID zero painted.
		var tile [64]byte
		tile[0], tile[1], tile[2] = byte(id), byte(id>>8), 1
		writeObjectTestCell(sheet, 2048, id, tile)
	}
	compact, index := CompactObjectSheetCells(sheet, 2048, MaxObjSheetCells)
	if len(compact) != len(sheet) || index[65535] != 65536 {
		t.Fatalf("full OBJ bank: %d bytes, last index %d", len(compact), index[65535])
	}
	if compact[65535*64] == 0 {
		t.Fatal("last tile became transparent")
	}
	for id, slot := range index {
		if slot != uint32(id+1) {
			t.Fatalf("unique tile %d maps to slot %d", id, slot)
		}
	}
	assertObjectCellsUnpack(t, sheet, 2048, compact, index)
}

func TestCompactObjectSheetSharesIdenticalPixelsWithoutChangingIDs(t *testing.T) {
	var a, b [64]byte
	a[0], a[9], a[63] = 7, 13, 22
	b = a
	b[8] = 3
	sheet := make([]byte, 24*16)
	for id, tile := range map[int][64]byte{1: a, 2: a, 3: b, 5: a} {
		writeObjectTestCell(sheet, 24, id, tile)
	}
	authored := append([]byte(nil), sheet...)
	compact, index := CompactObjectSheetCells(sheet, 24, 6)
	if want := []uint32{0, 1, 1, 2, 0, 1}; !reflect.DeepEqual(index, want) {
		t.Fatalf("index = %v, want %v", index, want)
	}
	if want := append(append([]byte(nil), a[:]...), b[:]...); !bytes.Equal(compact, want) {
		t.Fatal("compact pixels do not follow first logical ID order")
	}
	analysis := AnalyzeObjectSheetCells(sheet, 24, 6)
	if !reflect.DeepEqual(analysis.Index, index) ||
		!reflect.DeepEqual(analysis.CanonicalIDs, []int{-1, 1, 1, 3, -1, 1}) ||
		analysis.PaintedTiles != 4 || analysis.UniqueTiles != 2 || analysis.SavedBytes != 128 {
		t.Fatalf("analysis = %+v", analysis)
	}
	if !bytes.Equal(sheet, authored) {
		t.Fatal("compaction or analysis changed authored pixels")
	}
	assertObjectCellsUnpack(t, sheet, 24, compact, index)
	secondCompact, secondIndex := CompactObjectSheetCells(sheet, 24, 6)
	if !bytes.Equal(compact, secondCompact) || !reflect.DeepEqual(index, secondIndex) {
		t.Fatal("compaction is not deterministic")
	}
	background, backgroundIndex := CompactSheetCells(sheet, 24, 6)
	if len(background) != 4*64 || !reflect.DeepEqual(backgroundIndex, []uint16{0, 1, 2, 3, 0, 4}) {
		t.Fatal("OBJ deduplication must not merge background tile storage")
	}
}

func TestCompactObjectSheetLeavesTransparentCellsImplicit(t *testing.T) {
	sheet := make([]byte, 16*16)
	compact, index := CompactObjectSheetCells(sheet, 16, 4)
	analysis := AnalyzeObjectSheetCells(sheet, 16, 4)
	if len(compact) != 0 || !reflect.DeepEqual(index, []uint32{0, 0, 0, 0}) ||
		!reflect.DeepEqual(analysis.CanonicalIDs, []int{-1, -1, -1, -1}) ||
		analysis.PaintedTiles != 0 || analysis.UniqueTiles != 0 || analysis.SavedBytes != 0 {
		t.Fatalf("transparent compaction = %v, %+v", index, analysis)
	}
}

func writeObjectTestCell(sheet []byte, width, id int, tile [64]byte) {
	x, y := id%(width/8)*8, id/(width/8)*8
	for row := 0; row < 8; row++ {
		copy(sheet[(y+row)*width+x:], tile[row*8:row*8+8])
	}
}

func assertObjectCellsUnpack(t *testing.T, sheet []byte, width int, compact []byte, index []uint32) {
	t.Helper()
	unpacked := make([]byte, len(sheet))
	for id, slot := range index {
		if slot == 0 {
			continue
		}
		var tile [64]byte
		copy(tile[:], compact[(int(slot)-1)*64:int(slot)*64])
		writeObjectTestCell(unpacked, width, id, tile)
	}
	if !bytes.Equal(unpacked, sheet) {
		t.Fatal("compiled compact bank does not reconstruct the authored pixels")
	}
}

func TestLoadObjSheetRejectsMalformedRecordsBeforeEditorUse(t *testing.T) {
	for _, test := range []struct {
		name   string
		sprite SpriteRecord
	}{
		{"negative rectangle", SpriteRecord{ID: 0, X: -1, Width: 8, Height: 8, Pixels: testSprite(0, 0, 0, 8, 8, 1, 0).Pixels}},
		{"out of bounds", testSprite(0, 8, 0, 8, 8, 1, 0)},
		{"missing rows", SpriteRecord{ID: 0, Width: 8, Height: 8}},
		{"bad row width", SpriteRecord{ID: 0, Width: 8, Height: 1, Pixels: [][]int{{1}}}},
		{"invalid palette index", testSprite(0, 0, 0, 8, 8, 256, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			sheet := Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{test.sprite}}
			if err := writeJSON(filepath.Join(dir, "assets", "objsheet.json"), sheet); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadObjSheet(dir); err == nil || !strings.Contains(err.Error(), "objsheet.json") {
				t.Fatalf("malformed object record was not rejected: %v", err)
			}
		})
	}
}
