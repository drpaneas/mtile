//go:build mtile_mode0 && !tinygo

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0PresentRequiresCompleteCommitWindow(t *testing.T) {
	// Hardware exposes the line number, not the elapsed cycles within it.
	// Every expected bound therefore excludes the current partial line.
	cases := []struct {
		name   string
		line   uint16
		cycles uint32
		want   bool
	}{
		{"visible-first-line", 0, mode0CommitReserve, false},
		{"visible-last-line", 159, mode0CommitReserve, false},
		{"full-reserve-at-vblank-start", 160, mode0CommitReserve, true},
		{"partial-current-line-is-not-counted", 160, mode0CommitReserve + 1, false},
		{"full-reserve-one-line-late", 161, mode0CommitReserve, false},
		{"observed-late-start", 202, mode0CommitReserve, false},
		{"smaller-work-at-complete-line-bound", 200, 27 * cyclesPerScanline, true},
		{"smaller-work-exceeds-complete-lines", 200, 27*cyclesPerScanline + 1, false},
		{"last-vblank-line-has-no-complete-line", 227, 1, false},
		{"invalid-line", 228, 1, false},
		{"invalid-large-line", 65535, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mode0PresentWindowFits(tc.line, tc.cycles); got != tc.want {
				t.Fatalf("line %d, %d cycles: fits = %t, want %t", tc.line, tc.cycles, got, tc.want)
			}
		})
	}
	if mode0CommitReserve >= (scanlinesPerFrame-ScreenHeight)*cyclesPerScanline {
		t.Fatal("reserve includes the unknown fraction of the starting scanline")
	}
}

func TestMode0PresentRecordsActualHardwareWindow(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.BeginRenderFrame()
	c.Cls(3)
	hw.SetReg16(hw.VCOUNT, 160)
	c.PresentRender()
	start, end := c.LastBlitWindow()
	if start != 160 || end != 160 {
		t.Fatalf("commit window = %d..%d, want 160..160 in the static host clock", start, end)
	}
	if got := hw.ReadPaletteRAM16(0); got != c.palette[c.displayPal[c.mode0Clear]] {
		t.Fatalf("backdrop = %#x after commit, want cls palette entry", got)
	}
}
