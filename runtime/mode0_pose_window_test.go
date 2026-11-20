//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_no_pspr

package runtime

import "testing"

// Compare every representable window with the original per-tile contract.
// Run with text and object packing both individually enabled and together:
// each atlas must remain protected, including zero-length boundary windows.
func TestMode0PSprWindowReservationsMatchEveryTile(t *testing.T) {
	reference := func(base, count int) bool {
		if base < 0 || count < 0 || base > mode0PsprWindowLimit-count {
			return false
		}
		for tile := base; tile < base+count; tile++ {
			if mode0PackedObjectReservedSource(tile) || mode0TextObjectTileReserved(tile) {
				return false
			}
		}
		return true
	}
	for base := -1; base <= mode0PsprWindowLimit+1; base++ {
		for count := -1; count <= mode0PsprWindowLimit+1; count++ {
			if got, want := mode0PsprWindowAvailable(base, count), reference(base, count); got != want {
				t.Fatalf("window (%d,%d): available=%t, want %t", base, count, got, want)
			}
		}
	}
	maxInt := int(^uint(0) >> 1)
	for _, window := range [][2]int{{maxInt, 1}, {1, maxInt}, {0, maxInt}, {-maxInt - 1, 0}, {0, -maxInt - 1}} {
		if mode0PsprWindowAvailable(window[0], window[1]) {
			t.Fatalf("accepted invalid window %v", window)
		}
	}
}
