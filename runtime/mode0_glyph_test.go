package runtime

import "testing"

func TestOverlayMode0TextSheetWritesGlyphsAndClearsUnusedCells(t *testing.T) {
	const sheetW, sheetH = 240, 160
	sheet := make([]byte, sheetW*sheetH)
	for i := range sheet {
		sheet[i] = 9
	}
	keys := []uint16{Mode0TextGlyphKey('7', 12)}
	OverlayMode0TextSheet(sheet, sheetW, sheetH, keys)

	want := BuildMode0GlyphTile('7', 12)
	id := Mode0TextSheetBase
	cols := sheetW / 8
	col, row := id%cols, id/cols
	origin := row*8*sheetW + col*8
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := sheet[origin+y*sheetW+x]
			if got != want[y*8+x] {
				t.Fatalf("glyph pixel (%d,%d)=%d, want %d", x, y, got, want[y*8+x])
			}
		}
	}

	unused := Mode0TextSheetBase + 1
	col, row = unused%cols, unused/cols
	origin = row*8*sheetW + col*8
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if sheet[origin+y*sheetW+x] != 0 {
				t.Fatalf("unused reserved cell %d kept authored ink", unused)
			}
		}
	}
	// Physical OBJ glyph relocation must not overwrite newly reserved
	// addresses in the authored BG sheet. Its legacy text bank stays 480..511.
	for id := 448; id < 480; id++ {
		origin := (id/cols)*8*sheetW + (id%cols)*8
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if sheet[origin+y*sheetW+x] != 9 {
					t.Fatalf("authored tile %d was overwritten", id)
				}
			}
		}
	}
	if sheet[0] != 9 {
		t.Fatal("overlay mutated a cell outside the Mode 0 text bank")
	}
}

func TestOverlayMode0TextSheetNoopsTinySheets(t *testing.T) {
	sheet := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	OverlayMode0TextSheet(sheet, 8, 8, []uint16{Mode0TextGlyphKey('A', 7)})
	for i, got := range sheet {
		if got != byte(i+1) {
			t.Fatalf("tiny sheet[%d]=%d, want unchanged", i, got)
		}
	}
}
