//go:build mtile_mode0 && mtile_mode0_sheet_immutable

package runtime

import "testing"

func TestMode0ImmutableSheetElidesDirtyTileScan(t *testing.T) {
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	c.mode0TileDirty[0] = 1
	bg, obj := c.uploadMode0DirtyTiles()
	if bg != 0 || obj != 0 {
		t.Fatalf("immutable upload = (%d,%d), want (0,0)", bg, obj)
	}
	if c.mode0TileDirty[0] != 1 {
		t.Fatal("immutable upload path scanned and consumed a dirty bit")
	}
}
