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
	font                          *Font
	tiles, pixels                 [512]uint32
	oam                           [256]uint32
	palettes                      [PauseOBJPaletteColors]uint16
	display, blend                uint16
	saved, shown, dirty, prepared bool
	key                           uint32
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
func (v *pauseVideo) rect(x, y, w, h int, color uint32) {
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			v.pixel(col, row, color)
		}
	}
}
func (v *pauseVideo) text(s string, x, y int, color uint32) {
	for i := 0; i < len(s); i++ {
		columns := v.font.Columns(rune(s[i]))
		for col, bits := range columns {
			for row := 0; row < 5; row++ {
				if bits&(1<<uint(row)) != 0 {
					v.pixel(x+col, y+row, color)
				}
			}
		}
		x += 5
	}
}
func (v *pauseVideo) centered(s string, y int, color uint32) {
	v.text(s, (64-(len(s)*5-1))/2, y, color)
}
func (v *pauseVideo) panel(tall bool) {
	for i := range v.pixels {
		v.pixels[i] = 0
	}
	top, bottom := 8, 56
	if tall {
		top, bottom = 0, 64
	}
	v.rect(2, top, 60, bottom-top, 1)
	v.rect(0, top+2, 64, bottom-top-4, 1)
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
