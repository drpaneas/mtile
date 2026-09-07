//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_objpack && mtile_mode0_sheet_immutable && !tinygo

package runtime

import "testing"

func TestMode0CommitBudgetChecksPendingPackedTiles(t *testing.T) {
	c := &Context{paletteReady: true}
	if c.mode0RequiredCommitCycles() != 22000 {
		t.Fatal("empty packed cache should use the warm bound")
	}
	for bit := uint(0); bit < 8; bit++ {
		c.mode0PackedObjectDirty = 1 << bit
		if c.mode0RequiredCommitCycles() != mode0CommitReserve {
			t.Fatalf("packed dirty bit %d did not reserve full window", bit)
		}
	}
}
