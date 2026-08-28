//go:build tinygo

// Command benchspike measures the runtime's per-frame hot paths in real
// ARM7TDMI cycles using the GBA's TM0/TM1 cascade, then prints a table of
// cycles and %-of-frame-budget on screen and parks the results in named
// EWRAM words for a debugger.
//
// The frame budget is one full 228-scanline display interval:
// 1232 * 228 = 280,896 cycles. A workload's percentage is against that
// number; the practical update+draw budget is lower because the DMA blit
// (~77k cycles) and palette/flip also spend from the same frame.
//
// Build: make benchspike-gba (runtime/), then run benchspike.gba in mGBA.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	"unsafe" // unsafe.String for zero-allocation number formatting
)

const (
	sheetW  = 128
	sheetH  = 128
	spriteN = (sheetW / 8) * (sheetH / 8)
	mapW    = 64
	mapH    = 64

	cyclesPerFrame = 1232 * 228
)

// makeSheetROM is evaluated by TinyGo's global interpreter, producing a
// realistic half-transparent sheet without maintaining a 16 KB hand-written
// literal. It is copied to EWRAM for the measured mutable baseline.
func makeSheetROM() (sheet [sheetW * sheetH]byte) {
	for y := 0; y < sheetH; y++ {
		for x := 0; x < sheetW; x++ {
			i := y*sheetW + x
			if y < 8 || (i/4)%2 == 0 {
				sheet[i] = byte(1 + i%15)
			}
		}
	}
	return sheet
}

//go:section .rodata
var sheetROM = makeSheetROM()

//go:section .rodata
var mapROM = [mapW * mapH]uint16{}

//go:section .rodata
var flagROM = [spriteN]byte{}

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

// benchStatus: 0xBE7C0001 = measuring, 0xBE7C0002 = done.
//
//go:section .ewram
var benchStatus volatile.Register32

// benchCycles[i] holds the cycle count for workload i, in the order the
// workloads table below declares them. Read with a debugger via the
// benchCycles symbol; benchCount holds how many entries are valid.
//
//go:section .ewram
var benchCycles [24]volatile.Register32

//go:section .ewram
var benchCount volatile.Register32

const (
	statusMeasuring = 0xBE7C0001
	statusDone      = 0xBE7C0002
	statusFail      = 0xDEADBE7C
)

func halt() {
	for {
	}
}

// utoa formats v into buf and returns the digit count. buf must hold 10.
func utoa(buf []byte, v uint32) int {
	if v == 0 {
		buf[0] = '0'
		return 1
	}
	n := 0
	for v > 0 {
		buf[n] = byte('0' + v%10)
		v /= 10
		n++
	}
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return n
}

// row print buffer. Static so printing allocates nothing.
var rowBuf [40]byte

func printRow(y int, name string, cycles uint32) {
	n := 0
	for i := 0; i < len(name) && n < 12; i++ {
		rowBuf[n] = name[i]
		n++
	}
	for n < 13 {
		rowBuf[n] = ' '
		n++
	}
	n += utoa(rowBuf[n:], cycles)
	rowBuf[n] = ' '
	n++
	pct := cycles / (cyclesPerFrame / 100)
	n += utoa(rowBuf[n:], pct)
	rowBuf[n] = '%'
	n++
	ctx.Print(unsafe.String(&rowBuf[0], n), 4, y, 7)
}

func measure(f func()) uint32 {
	hw.StartCycleCounter()
	f()
	return hw.StopCycleCounter()
}

func authorAssets() {
	// Map rows 0-31: all sprite ids (50% transparent art, worst case).
	// Map rows 32-47: opaque row-0 sprites only (best case).
	// Map rows 48+: checkerboard, half the cells empty (sprite 0) - the
	// density of a real platformer room.
	for y := 0; y < mapH; y++ {
		for x := 0; x < mapW; x++ {
			switch {
			case y >= 48:
				if (x+y)&1 == 0 {
					ctx.Mset(x, y, 1+(y*mapW+x)%(spriteN-1))
				}
			case y >= 32:
				ctx.Mset(x, y, 1+(y*mapW+x)%15)
			default:
				ctx.Mset(x, y, 1+(y*mapW+x)%(spriteN-1))
			}
		}
	}
	// Flag bit 1 on every fourth sprite, for the layer-mask pass: a masked
	// map() call must reject three quarters of the cells cheaply.
	for id := 0; id < spriteN; id += 4 {
		ctx.Fset(id, 1, true)
	}
	authorSparseGBAScreen()
}

// authorSparseGBAScreen lays out a 17%-dense 30x20 room at map rows 56-75,
// matching examples/mapdemo. Opaque terrain uses row-0 sprite ids; sky
// decorations use the half-transparent ids below.
func authorSparseGBAScreen() {
	const baseY = 56
	put := func(x, y, t int) {
		if x < 0 || x >= 30 || y < 0 || y >= 20 {
			return
		}
		ctx.Mset(x, baseY+y, t)
	}
	for x := 0; x < 30; x++ {
		put(x, 18, 1)
		put(x, 19, 2)
	}
	for x := 22; x < 30; x++ {
		put(x, 17, 1)
		if x > 25 {
			put(x, 16, 1)
		}
	}
	for x := 22; x < 30; x++ {
		put(x, 18, 2)
	}
	for x := 26; x < 30; x++ {
		put(x, 17, 2)
	}
	for x := 4; x < 9; x++ {
		put(x, 13, 3)
	}
	for x := 12; x < 16; x++ {
		put(x, 10, 3)
	}
	for x := 19; x < 23; x++ {
		put(x, 7, 3)
	}
	for y := 14; y < 18; y++ {
		put(2, y, 4)
	}
	for x := 9; x < 12; x++ {
		put(x, 15, 1)
	}
	for _, p := range [][2]int{{5, 12}, {7, 12}, {13, 9}, {14, 9}, {20, 6}, {3, 17}, {10, 17}, {15, 17}, {24, 16}, {27, 15}} {
		put(p[0], p[1], 16)
	}
	for _, p := range [][2]int{{6, 12}, {21, 6}, {12, 17}, {25, 16}} {
		put(p[0], p[1], 17)
	}
}

func initMutableAssets() bool {
	image := rt.AssetImage{
		SheetW: sheetW, SheetH: sheetH,
		SpriteCols: sheetW / 8, SpriteRows: sheetH / 8, SpriteN: spriteN,
		MapW: mapW, MapH: mapH,
		Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],
	}
	cartData.SheetW, cartData.SheetH = sheetW, sheetH
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = sheetW/8, sheetH/8, spriteN
	cartData.MapW, cartData.MapH = mapW, mapH
	cartData.SheetReadOnly = false
	cartData.Sheet, cartData.Map, cartData.Flags = sheetRAM[:], mapRAM[:], flagRAM[:]
	return ctx.Init(&image, &cartData)
}

func main() {
	if !initMutableAssets() {
		benchStatus.Set(statusFail)
		halt()
	}
	ctx.InitHardware()
	authorAssets()
	benchStatus.Set(statusMeasuring)

	runWorkloads()
	benchStatus.Set(statusDone)

	// Present the table.
	ctx.Cls(0)
	ctx.Print("MTILE BENCH (CYCLES, % FRAME)", 4, 4, 7)
	for i := 0; i < len(workloads); i++ {
		printRow(12+i*8, workloads[i].name, results[i])
	}
	ctx.WaitForPresentWindow()
	ctx.PrepareHiddenPage()
	ctx.FlipPage()
	halt()
}

type workload struct {
	name string
	f    func()
}

var results [24]uint32

// workloads is package-level: an 18-entry slice literal inside main
// exceeds TinyGo's stack-allocation threshold and would need runtime.alloc,
// which -gc=none forbids. A global composite literal lives in .data.
var workloads = []workload{
	{"CLS", func() { ctx.Cls(1) }},
	{"RECTFILL FS", func() { ctx.Rectfill(0, 0, 239, 159, 5) }},
	{"MAP 30X20", func() { ctx.Map(0, 0, 0, 0, 30, 20, 0) }},
	{"MAP OPAQUE", func() { ctx.Map(0, 32, 0, 0, 30, 20, 0) }},
	{"SPR X100", func() {
		for s := 0; s < 100; s++ {
			ctx.Spr(1+s%32, (s*16)%232, (s*8)%152, 1, 1, false, false)
		}
	}},
	{"SPRODD X100", func() {
		for s := 0; s < 100; s++ {
			ctx.Spr(1+s%32, 1+(s*16)%224, (s*8)%152, 1, 1, false, false)
		}
	}},
	{"SPRFLIP X100", func() {
		for s := 0; s < 100; s++ {
			ctx.Spr(1+s%32, (s*16)%232, (s*8)%152, 1, 1, true, false)
		}
	}},
	{"SSPR2X X10", func() {
		for s := 0; s < 10; s++ {
			ctx.SSpr(8, 0, 8, 8, s*20, 100, 16, 16, false, false)
		}
	}},
	{"PRINT 40CH", func() {
		ctx.Print("0123456789012345678901234567890123456789", 0, 0, 7)
	}},
	{"LINE DIAG", func() { ctx.Line(0, 0, 239, 159, 7) }},
	{"CIRCFILL R16", func() { ctx.Circfill(120, 80, 16, 9) }},
	{"SINCOS X100", func() {
		acc := rt.Number(0)
		for i := 0; i < 100; i++ {
			n := rt.Number(i << 8)
			acc += n.Sin() + n.Cos()
		}
		sink.Set(uint32(acc))
	}},
	{"BLIT DMA", func() { ctx.PrepareHiddenPage() }},
	// Practical 30 Hz questions: a PICO-8 room is 16x16 cells, not the
	// GBA's 30x20; real maps are half empty; layered redraws use masks.
	{"MAP 16X16", func() { ctx.Map(0, 0, 0, 0, 16, 16, 0) }},
	{"MAP 16 HALF", func() { ctx.Map(0, 48, 0, 0, 16, 16, 0) }},
	{"MAP 16 MASK", func() { ctx.Map(0, 0, 0, 0, 16, 16, 2) }},
	{"MAP30 HALF", func() { ctx.Map(0, 48, 0, 0, 30, 16, 0) }},
	{"MAP 17%", func() { ctx.Map(0, 56, 0, 0, 30, 20, 0) }},
	// A whole Celeste-class tick: clear, dense room, 30 sprites, HUD
	// text, and 24 particle rectfills. If this fits with the 77k blit,
	// the port is proven before writing a line of Lua.
	{"P8 TICK", func() {
		ctx.Cls(1)
		ctx.Map(0, 48, 0, 0, 16, 16, 0)
		for s := 0; s < 30; s++ {
			ctx.Spr(1+s%32, (s*16)%232, (s*8)%152, 1, 1, s&3 == 0, false)
		}
		for p := 0; p < 24; p++ {
			x := (p * 9) % 230
			y := (p * 13) % 150
			ctx.Rectfill(x, y, x+1, y+1, 7)
		}
		ctx.Print("3200 M", 4, 4, 7)
	}},
	// Full GBA-screen tick at realistic platformer density (examples/mapdemo).
	{"GBA TICK", func() {
		ctx.Cls(1)
		ctx.Map(0, 56, 0, 0, 30, 20, 0)
		ctx.Spr(1, 60, 136, 1, 1, false, false)
		ctx.Print("cpu 62%", 4, 4, 7)
		ctx.Print("arrows move", 4, 150, 7)
	}},
}

func runWorkloads() {
	for i := range workloads {
		// Run from a fresh VBlank edge so DMA/HBlank interference is the
		// same for every workload; one run is exact on this hardware.
		hw.WaitVDraw()
		results[i] = measure(workloads[i].f)
		benchCycles[i].Set(results[i])
	}
	benchCount.Set(uint32(len(workloads)))
}

//go:section .ewram
var sink volatile.Register32
