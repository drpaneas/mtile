package runtime

import "github.com/drpaneas/mtile/runtime/hw"

// The system overlay owns one 64x64, 4bpp OBJ. Mode 0's proof reserves its
// character cells, five palette entries, one OAM entry and 64 scanline cycles.
// Mode 4 has no game OBJs. Neither path changes the frozen BGs or framebuffers.
const (
	PauseOBJTileBase              = 960 // 32-byte units: the last 2 KiB of OBJ VRAM
	PauseOBJPaletteStart          = 241
	PauseOBJPaletteColors         = 5
	pauseTileBase                 = PauseOBJTileBase
	pauseTileAddress              = hw.OBJVRAM + pauseTileBase*32
	pauseBlendControl     uintptr = 0x04000050
)

type pauseVideo struct {
	tiles, pixels                 [512]uint32
	oam                           [256]uint32
	palettes                      [PauseOBJPaletteColors]uint16
	display, blend                uint16
	saved, shown, dirty, prepared bool
}

func (v *pauseVideo) capture() {
	v.display, v.blend = hw.GetReg16(hw.DISPCNT), hw.GetReg16(pauseBlendControl)
	hw.ReadVideoWords32DMA(v.tiles[:], pauseTileAddress)
	hw.ReadVideoWords32DMA(v.oam[:], hw.OAM)
	for i := range v.palettes {
		v.palettes[i] = hw.ReadPaletteRAM16(256 + PauseOBJPaletteStart + i)
	}
	v.saved = true
}

func (v *pauseVideo) present() {
	if !v.shown {
		// Move the complete game painter stack together without changing order.
		if v.display&0x1000 != 0 {
			hw.CopyWordsToVRAM32DMA(v.oam[:254], hw.OAM+8, 254)
		} else {
			for i := 1; i < 128; i++ {
				hw.SetReg16(hw.OAM+uintptr(i*8), 0x0200)
			}
		}
		for i, color := range [PauseOBJPaletteColors]uint16{0x1042, 0x28c3, 0x7b8a, 0x7fff, 0x4631} {
			hw.SetPaletteRAM16(256+PauseOBJPaletteStart+i, color)
		}
		hw.SetReg16(hw.OAM, 48)
		hw.SetReg16(hw.OAM+2, 88|3<<14)
		hw.SetReg16(hw.OAM+4, pauseTileBase|15<<12)
		hw.SetReg16(pauseBlendControl, 0)
	}
	if v.dirty || !v.shown {
		hw.CopyWordsToVRAM32DMA(v.pixels[:], pauseTileAddress, len(v.pixels))
	}
	hw.SetReg16(hw.DISPCNT, v.display&^0x80|0x1040)
	v.shown, v.dirty = true, false
}

// Rasterize before the presentation wait. Pixel indices are local to the
// private palette; game camera, clipping, palettes and text APIs cannot affect it.
func (v *pauseVideo) pixel(x, y int, color uint32) {
	if uint(x) >= 64 || uint(y) >= 64 {
		return
	}
	word := (y/8*8+x/8)*8 + y%8
	shift := uint(x % 8 * 4)
	v.pixels[word] = v.pixels[word]&^(15<<shift) | color<<shift
}

// Fixed 5x7 uppercase glyphs. Each letter advances nine pixels, giving the
// visibly spaced message “P A U S E D” a 50-pixel ink width plus its shadow.
func (v *pauseVideo) preparePaused() {
	for i := range v.pixels {
		v.pixels[i] = 0
	}
	glyphs := [6][5]byte{
		{0x7f, 0x09, 0x09, 0x09, 0x06}, // P
		{0x7e, 0x09, 0x09, 0x09, 0x7e}, // A
		{0x3f, 0x40, 0x40, 0x40, 0x3f}, // U
		{0x46, 0x49, 0x49, 0x49, 0x31}, // S
		{0x7f, 0x49, 0x49, 0x49, 0x41}, // E
		{0x7f, 0x41, 0x41, 0x22, 0x1c}, // D
	}
	// Draw shadow first so it never obscures another letter's white ink.
	for pass := 1; pass >= 0; pass-- {
		color := uint32(4)
		if pass == 1 {
			color = 1
		}
		for letter, cols := range glyphs {
			for col, bits := range cols {
				for row := 0; row < 7; row++ {
					if bits&(1<<uint(row)) != 0 {
						v.pixel(7+letter*9+col+pass, 28+row+pass, color)
					}
				}
			}
		}
	}
}
func (v *pauseVideo) restore() {
	if !v.saved {
		return
	}
	hw.CopyWordsToVRAM32DMA(v.tiles[:], pauseTileAddress, len(v.tiles))
	hw.CopyWordsToVRAM32DMA(v.oam[:], hw.OAM, len(v.oam))
	for i, color := range v.palettes {
		hw.SetPaletteRAM16(256+PauseOBJPaletteStart+i, color)
	}
	hw.SetReg16(pauseBlendControl, v.blend)
	hw.SetReg16(hw.DISPCNT, v.display)
	v.saved, v.shown = false, false
}
