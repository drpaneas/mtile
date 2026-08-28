//go:build !tinygo

package hw

import "testing"

func TestHostVRAMOffsetRejectsExactEnd(t *testing.T) {
	tests := []struct {
		name  string
		addr  uintptr
		width int
		want  bool
	}{
		{"last valid word", VRAM + uintptr(len(hostVRAM)-4), 4, true},
		{"exact end", VRAM + uintptr(len(hostVRAM)), 0, false},
		{"crosses end", VRAM + uintptr(len(hostVRAM)-1), 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := hostVRAMOffset(tt.addr, tt.width); ok != tt.want {
				t.Fatalf("hostVRAMOffset(%#x, %d) ok = %t, want %t", tt.addr, tt.width, ok, tt.want)
			}
		})
	}
}

// TestSetPaletteRAM32WritesPairAndRejectsOutOfRange guards the batched
// two-entry palette write ApplyDisplayPalette uses. A 32-bit PALRAM store
// must start on an even color index; an odd index is a misaligned GBA write.
func TestSetPaletteRAM32WritesPairAndRejectsOutOfRange(t *testing.T) {
	ResetForTest()
	SetPaletteRAM32(254, 0x1111, 0x2222)
	if got := ReadPaletteRAM16(254); got != 0x1111 {
		t.Fatalf("palette[254] = %#x, want %#x", got, uint16(0x1111))
	}
	if got := ReadPaletteRAM16(255); got != 0x2222 {
		t.Fatalf("palette[255] = %#x, want %#x", got, uint16(0x2222))
	}
	ResetForTest()
	SetPaletteRAM32(255, 0x3333, 0x4444)
	if got := ReadPaletteRAM16(255); got != 0 {
		t.Fatalf("SetPaletteRAM32(255, ...) wrote past the last entry: palette[255] = %#x, want 0", got)
	}
	SetPaletteRAM32(-1, 0x3333, 0x4444)
	if got := ReadPaletteRAM16(0); got != 0 {
		t.Fatalf("SetPaletteRAM32(-1, ...) wrote at a negative index: palette[0] = %#x, want 0", got)
	}
	ResetForTest()
	SetPaletteRAM16(1, 0x00aa)
	SetPaletteRAM16(2, 0x00bb)
	SetPaletteRAM32(1, 0x3333, 0x4444)
	if got := ReadPaletteRAM16(1); got != 0x00aa {
		t.Fatalf("SetPaletteRAM32(1, ...) wrote an odd index: palette[1] = %#x, want %#x", got, uint16(0x00aa))
	}
	if got := ReadPaletteRAM16(2); got != 0x00bb {
		t.Fatalf("SetPaletteRAM32(1, ...) wrote the following entry: palette[2] = %#x, want %#x", got, uint16(0x00bb))
	}
}

func TestCopyToVRAM32DMARejectsOversizeCount(t *testing.T) {
	ResetForTest()
	src := make([]byte, (0xFFFF+1)*4)
	for i := range src {
		src[i] = 0xAB
	}
	CopyToVRAM32DMA(src, VRAM, 0xFFFF+1)
	if got := ReadVRAM32(VRAM); got != 0 {
		t.Fatalf("oversize DMA count wrote VRAM[0] = %#x, want 0", got)
	}
}

func TestHostPaletteOffsetRejectsExactEnd(t *testing.T) {
	tests := []struct {
		name  string
		addr  uintptr
		width int
		want  bool
	}{
		{"last valid entry", PALRAM + uintptr(len(hostPalette)*2-2), 2, true},
		{"exact end", PALRAM + uintptr(len(hostPalette)*2), 0, false},
		{"crosses end", PALRAM + uintptr(len(hostPalette)*2-1), 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := hostPaletteOffset(tt.addr, tt.width); ok != tt.want {
				t.Fatalf("hostPaletteOffset(%#x, %d) ok = %t, want %t", tt.addr, tt.width, ok, tt.want)
			}
		})
	}
}
