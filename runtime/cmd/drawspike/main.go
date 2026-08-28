//go:build tinygo

package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

const (
	sheetW  = 16
	sheetH  = 8
	spriteN = 2
	mapW    = 1
	mapH    = 1
)

// The spike deliberately uses initialized ROM arrays and zero-initialized
// EWRAM arrays, matching the layout a later assetsync/code generator must
// produce.
//
//go:section .rodata
var sheetROM = [sheetW * sheetH]byte{
	0, 1, 1, 1, 1, 1, 1, 0,
	1, 2, 2, 2, 2, 2, 2, 1,
	1, 2, 0, 0, 0, 0, 2, 1,
	1, 2, 0, 2, 2, 0, 2, 1,
	1, 2, 0, 2, 2, 0, 2, 1,
	1, 2, 0, 0, 0, 0, 2, 1,
	1, 2, 2, 2, 2, 2, 2, 1,
	0, 1, 1, 1, 1, 1, 1, 0,
	0, 3, 3, 3, 3, 3, 3, 0,
	3, 4, 4, 4, 4, 4, 4, 3,
	3, 4, 0, 0, 0, 0, 4, 3,
	3, 4, 0, 4, 4, 0, 4, 3,
	3, 4, 0, 4, 4, 0, 4, 3,
	3, 4, 0, 0, 0, 0, 4, 3,
	3, 4, 4, 4, 4, 4, 4, 3,
	0, 3, 3, 3, 3, 3, 3, 0,
}

//go:section .rodata
var mapROM = [mapW * mapH]uint16{0}

//go:section .rodata
var flagROM = [spriteN]byte{1, 2}

//go:section .rodata
var paletteROM = [256]uint16{
	0: 0x0000, 1: 0x28A3, 2: 0x288F, 3: 0x2A00,
	4: 0x1955, 5: 0x254B, 6: 0x6318, 7: 0x77DF,
	8: 0x241F, 9: 0x029F, 10: 0x13BF, 11: 0x1B80,
	12: 0x7EA5, 13: 0x4DD0, 14: 0x55DF, 15: 0x573F,
	// Extended indices are native GBA colors too. The spike exercises 255
	// explicitly, so do not leave it at the zero-value black entry.
	16: 0x0010, 17: 0x0011, 18: 0x0012, 19: 0x0013, 20: 0x0014, 21: 0x0015, 22: 0x0016, 23: 0x0017,
	24: 0x0018, 25: 0x0019, 26: 0x001A, 27: 0x001B, 28: 0x001C, 29: 0x001D, 30: 0x001E, 31: 0x001F,
	32: 0x0020, 33: 0x0021, 34: 0x0022, 35: 0x0023, 36: 0x0024, 37: 0x0025, 38: 0x0026, 39: 0x0027,
	40: 0x0028, 41: 0x0029, 42: 0x002A, 43: 0x002B, 44: 0x002C, 45: 0x002D, 46: 0x002E, 47: 0x002F,
	48: 0x0030, 49: 0x0031, 50: 0x0032, 51: 0x0033, 52: 0x0034, 53: 0x0035, 54: 0x0036, 55: 0x0037,
	56: 0x0038, 57: 0x0039, 58: 0x003A, 59: 0x003B, 60: 0x003C, 61: 0x003D, 62: 0x003E, 63: 0x003F,
	64: 0x0040, 65: 0x0041, 66: 0x0042, 67: 0x0043, 68: 0x0044, 69: 0x0045, 70: 0x0046, 71: 0x0047,
	72: 0x0048, 73: 0x0049, 74: 0x004A, 75: 0x004B, 76: 0x004C, 77: 0x004D, 78: 0x004E, 79: 0x004F,
	80: 0x0050, 81: 0x0051, 82: 0x0052, 83: 0x0053, 84: 0x0054, 85: 0x0055, 86: 0x0056, 87: 0x0057,
	88: 0x0058, 89: 0x0059, 90: 0x005A, 91: 0x005B, 92: 0x005C, 93: 0x005D, 94: 0x005E, 95: 0x005F,
	96: 0x0060, 97: 0x0061, 98: 0x0062, 99: 0x0063, 100: 0x0064, 101: 0x0065, 102: 0x0066, 103: 0x0067,
	104: 0x0068, 105: 0x0069, 106: 0x006A, 107: 0x006B, 108: 0x006C, 109: 0x006D, 110: 0x006E, 111: 0x006F,
	112: 0x0070, 113: 0x0071, 114: 0x0072, 115: 0x0073, 116: 0x0074, 117: 0x0075, 118: 0x0076, 119: 0x0077,
	120: 0x0078, 121: 0x0079, 122: 0x007A, 123: 0x007B, 124: 0x007C, 125: 0x007D, 126: 0x007E, 127: 0x007F,
	128: 0x0080, 129: 0x0081, 130: 0x0082, 131: 0x0083, 132: 0x0084, 133: 0x0085, 134: 0x0086, 135: 0x0087,
	136: 0x0088, 137: 0x0089, 138: 0x008A, 139: 0x008B, 140: 0x008C, 141: 0x008D, 142: 0x008E, 143: 0x008F,
	144: 0x0090, 145: 0x0091, 146: 0x0092, 147: 0x0093, 148: 0x0094, 149: 0x0095, 150: 0x0096, 151: 0x0097,
	152: 0x0098, 153: 0x0099, 154: 0x009A, 155: 0x009B, 156: 0x009C, 157: 0x009D, 158: 0x009E, 159: 0x009F,
	160: 0x00A0, 161: 0x00A1, 162: 0x00A2, 163: 0x00A3, 164: 0x00A4, 165: 0x00A5, 166: 0x00A6, 167: 0x00A7,
	168: 0x00A8, 169: 0x00A9, 170: 0x00AA, 171: 0x00AB, 172: 0x00AC, 173: 0x00AD, 174: 0x00AE, 175: 0x00AF,
	176: 0x00B0, 177: 0x00B1, 178: 0x00B2, 179: 0x00B3, 180: 0x00B4, 181: 0x00B5, 182: 0x00B6, 183: 0x00B7,
	184: 0x00B8, 185: 0x00B9, 186: 0x00BA, 187: 0x00BB, 188: 0x00BC, 189: 0x00BD, 190: 0x00BE, 191: 0x00BF,
	192: 0x00C0, 193: 0x00C1, 194: 0x00C2, 195: 0x00C3, 196: 0x00C4, 197: 0x00C5, 198: 0x00C6, 199: 0x00C7,
	200: 0x00C8, 201: 0x00C9, 202: 0x00CA, 203: 0x00CB, 204: 0x00CC, 205: 0x00CD, 206: 0x00CE, 207: 0x00CF,
	208: 0x00D0, 209: 0x00D1, 210: 0x00D2, 211: 0x00D3, 212: 0x00D4, 213: 0x00D5, 214: 0x00D6, 215: 0x00D7,
	216: 0x00D8, 217: 0x00D9, 218: 0x00DA, 219: 0x00DB, 220: 0x00DC, 221: 0x00DD, 222: 0x00DE, 223: 0x00DF,
	224: 0x00E0, 225: 0x00E1, 226: 0x00E2, 227: 0x00E3, 228: 0x00E4, 229: 0x00E5, 230: 0x00E6, 231: 0x00E7,
	232: 0x00E8, 233: 0x00E9, 234: 0x00EA, 235: 0x00EB, 236: 0x00EC, 237: 0x00ED, 238: 0x00EE, 239: 0x00EF,
	240: 0x00F0, 241: 0x00F1, 242: 0x00F2, 243: 0x00F3, 244: 0x00F4, 245: 0x00F5, 246: 0x00F6, 247: 0x00F7,
	248: 0x00F8,
	249: 0x00F9, 250: 0x00FA, 251: 0x00FB, 252: 0x00FC, 253: 0x00FD, 254: 0x00FE,
	255: 0x7FFF,
}

//go:section .ewram
var sheetRAM [sheetW * sheetH]byte

//go:section .ewram
var mapRAM [mapW * mapH]uint16

//go:section .ewram
var flagRAM [spriteN]byte

//go:section .ewram
var cartData rt.AssetMemory

//go:section .ewram
var ctx rt.Context

//go:section .ewram
var drawStatus volatile.Register32

// Packed VCOUNT start (low 16) and end (high 16) for each presentation path.
//
//go:section .ewram
var blitDMAWindow volatile.Register32

//go:section .ewram
var blitDMACycles volatile.Register32

//go:section .ewram
var blitCPUWindow volatile.Register32

//go:section .ewram
var blitCPUCycles volatile.Register32

//go:section .ewram
var blitIWRAMWindow volatile.Register32

//go:section .ewram
var blitIWRAMCycles volatile.Register32

//go:section .ewram
var lastPresentWindow volatile.Register32

//go:section .ewram
var lastPresentCycles volatile.Register32

const (
	drawOK    = 0xD7A70001
	measureOK = 0xD7A70002
	drawFail  = 0xDEAD0002
)

func halt() {
	for {
	}
}

func packWindow(start, end uint16) uint32 {
	return uint32(start) | uint32(end)<<16
}

func recordKind(kind rt.BlitKind) bool {
	ctx.SetBlitKind(kind)
	ctx.WaitForPresentWindow()
	ok := ctx.BlitToHiddenPage()
	start, end := ctx.LastBlitWindow()
	window := packWindow(start, end)
	cycles := ctx.LastBlitCycles()
	switch kind {
	case rt.BlitDMA:
		blitDMAWindow.Set(window)
		blitDMACycles.Set(cycles)
	case rt.BlitCPU:
		blitCPUWindow.Set(window)
		blitCPUCycles.Set(cycles)
	case rt.BlitIWRAM:
		blitIWRAMWindow.Set(window)
		blitIWRAMCycles.Set(cycles)
	}
	ctx.WaitVDraw()
	return ok
}

func main() {
	cartData.SheetW, cartData.SheetH = sheetW, sheetH
	cartData.SpriteCols, cartData.SpriteRows = sheetW/8, sheetH/8
	cartData.MapW, cartData.MapH = mapW, mapH
	cartData.SpriteN = spriteN
	cartData.Sheet = sheetRAM[:]
	cartData.Map = mapRAM[:]
	cartData.Flags = flagRAM[:]
	image := rt.AssetImage{
		SheetW: sheetW, SheetH: sheetH,
		SpriteCols: sheetW / 8, SpriteRows: sheetH / 8,
		MapW: mapW, MapH: mapH, SpriteN: spriteN,
		Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],
	}
	if !ctx.Init(&image, &cartData) {
		drawStatus.Set(drawFail)
		halt()
	}
	if (rt.One/4+ctx.Time()).Sin() != -rt.One || ctx.Time().Cos() != rt.One {
		drawStatus.Set(drawFail)
		halt()
	}
	ctx.LoadPalette(&paletteROM)
	ctx.InitHardware()
	// Clear once before entering the persistent drawing loop. There is no
	// implicit per-frame clear: the moving primitives intentionally leave a
	// trail: there is no implicit per-frame clear.
	ctx.Cls(0)
	drawStatus.Set(drawOK)
	// Start the spike on a fresh VDraw interval rather than inheriting an
	// arbitrary scanline from ROM startup.
	ctx.WaitVDraw()

	// DMA is the production presentation path and is the only one required
	// to land within VBlank. CPU and IWRAM are recorded as diagnostic
	// baselines only; both are
	// known to miss VBlank on real hardware and are not required to pass.
	okDMA := recordKind(rt.BlitDMA)
	recordKind(rt.BlitCPU)
	recordKind(rt.BlitIWRAM)
	if okDMA {
		drawStatus.Set(measureOK)
	}

	ctx.SetBlitKind(rt.BlitDMA)

	tick := uint32(0)
	x := 104
	for {
		ctx.PollInput()
		if tick&1 == 0 {
			ctx.AdvanceTick()
			if ctx.Btn(0) {
				x--
			}
			if ctx.Btn(1) {
				x++
			}
			ctx.Rectfill(16, 16, 64, 64, 1)
			ctx.Line(0, 0, 20, 10, 8)
			ctx.Rect(80, 8, 100, 24, 9)
			ctx.Circ(200, 24, 10, 10)
			ctx.Circfill(220, 24, 6, 11)
			ctx.Print("P2", 8, 8, 7)
			ctx.Map(0, 0, 0, 120, 1, 1, 0)
			ctx.Spr(0, x, 68, 1, 1, false, false)
			ctx.Spr(1, x+8, 68, 1, 1, true, false)
			ctx.Pset(239, 159, 255)
		} else {
			ctx.CollectIfNeeded()
		}

		ctx.WaitForPresentWindow()
		if tick&1 == 0 {
			ctx.ApplyDisplayPalette()
			if ctx.BlitToHiddenPage() {
				start, end := ctx.LastBlitWindow()
				lastPresentWindow.Set(packWindow(start, end))
				lastPresentCycles.Set(ctx.LastBlitCycles())
				ctx.FlipPage()
			}
		}
		ctx.WaitVDraw()
		tick++
	}
}
