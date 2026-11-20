//go:build tinygo

package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"runtime/volatile"
	"unsafe" // required for //go:section
)

const (
	smokeSheetW, smokeSheetH = 8, 8
	smokeMapW, smokeMapH     = 1, 1
	smokeSpriteN             = 1
)

//go:section .rodata
var sheetROM = [smokeSheetW * smokeSheetH]byte{0: 0x12, 63: 0x34}

//go:section .rodata
var mapROM = [smokeMapW * smokeMapH]uint16{0: 0x0056}

//go:section .rodata
var flagROM = [smokeSpriteN]byte{0: 0x78}

// The context retains this descriptor for the lifetime of the cartridge.
var img = rt.AssetImage{
	SheetW: smokeSheetW, SheetH: smokeSheetH,
	SpriteCols: smokeSheetW / 8, SpriteRows: smokeSheetH / 8,
	MapW: smokeMapW, MapH: smokeMapH, SpriteN: smokeSpriteN,
	Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],
}

//go:section .ewram_begin
var ewramBegin uint32 // first .ewram object; proves zero loop started

//go:section .ewram_end
var ewramEnd uint32 // last .ewram object; proves zero loop reached _ewram_end

//go:section .ewram
var sheetRAM [smokeSheetW * smokeSheetH]byte

//go:section .ewram
var mapRAM [smokeMapW * smokeMapH]uint16

//go:section .ewram
var flagRAM [smokeSpriteN]byte

//go:section .ewram
var cartData rt.AssetMemory

//go:section .ewram
var ctx rt.Context

// smokeStatus is an EWRAM status word for the host debugger (nm / gdb).
// It is not GBA MMIO. volatile.Register32 keeps TinyGo from dropping the store.
//
//go:section .ewram
var smokeStatus volatile.Register32

//go:section .ewram
var smokeProbe uint32

const (
	smokeOK       = 0xA5A5A5A5
	smokeFailZero = 0xDEAD0001
	smokeFailCopy = 0xDEAD0002
	smokePoison   = 0xF1F1F1F1 // crt0 fill before zero; must not equal smokeOK
)

func halt() {
	for {
	}
}

func allZero[T any](p *T) bool {
	n := unsafe.Sizeof(*p)
	b := unsafe.Slice((*byte)(unsafe.Pointer(p)), n)
	for i := range b {
		if volatile.LoadUint8(&b[i]) != 0 {
			return false
		}
	}
	return true
}

func main() {
	if volatile.LoadUint32(&ewramBegin) != 0 || volatile.LoadUint32(&ewramEnd) != 0 ||
		!allZero(&sheetRAM) || !allZero(&mapRAM) || !allZero(&flagRAM) ||
		!allZero(&cartData) || !allZero(&ctx) {
		smokeStatus.Set(smokeFailZero)
		halt()
	}
	cartData.SheetW, cartData.SheetH = smokeSheetW, smokeSheetH
	cartData.SpriteCols, cartData.SpriteRows = smokeSheetW/8, smokeSheetH/8
	cartData.MapW, cartData.MapH = smokeMapW, smokeMapH
	cartData.SpriteN = smokeSpriteN
	cartData.Sheet = sheetRAM[:]
	cartData.Map = mapRAM[:]
	cartData.Flags = flagRAM[:]
	if !ctx.Init(&img, &cartData) {
		smokeStatus.Set(smokeFailCopy)
		halt()
	}
	if volatile.LoadUint8(&sheetRAM[0]) != 0x12 || volatile.LoadUint8(&sheetRAM[63]) != 0x34 ||
		volatile.LoadUint16(&mapRAM[0]) != 0x0056 || volatile.LoadUint8(&flagRAM[0]) != 0x78 {
		smokeStatus.Set(smokeFailCopy)
		halt()
	}
	if volatile.LoadUint8(&sheetROM[0]) != 0x12 || volatile.LoadUint8(&sheetROM[63]) != 0x34 ||
		volatile.LoadUint16(&mapROM[0]) != 0x0056 || volatile.LoadUint8(&flagROM[0]) != 0x78 {
		smokeStatus.Set(smokeFailCopy)
		halt()
	}
	volatile.StoreUint32(&smokeProbe, 0xCAFEBABE)
	if volatile.LoadUint32(&smokeProbe) != 0xCAFEBABE {
		smokeStatus.Set(smokeFailCopy)
		halt()
	}
	smokeStatus.Set(smokeOK)
	halt()
}
