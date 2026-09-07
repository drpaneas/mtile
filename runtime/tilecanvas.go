//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"unsafe"
)

// The private hybrid shares ordinary Mode 0 tilemaps and uses an OBJ canvas
// for foreground raster calls. These are hardware/storage limits, not assets.
const (
	tileCanvasExperiment   = true
	mode4Display           = 0x0404
	forceBlank             = 0x0080
	pageBit                = 0x0010
	mode0LayerCount        = 4
	mode0LayerStorageCount = 4
	mode0TileLimit         = (32 - 2*mode0LayerCount) * 32
	canvasCommitOverhead   = 32000
)

func (c *Context) InitHardware() {
	hw.SetReg16(hw.DISPCNT, forceBlank)
	if c.cart == nil || !(sheetStorageReadOnly || c.cart.SheetReadOnly) {
		panic("tile canvas requires an immutable spritesheet; use Mode 4")
	}
	if c.cart.SpriteN > mode0TileLimit {
		panic("tile canvas background tiles exceed VRAM; use Mode 4")
	}
	for offset := 0; offset < 65536; offset += 4 {
		hw.SetVRAM32(hw.VRAM+uintptr(offset), 0)
	}
	for i := 0; i < 128; i++ {
		hw.SetReg16(hw.OAM+uintptr(i*8), 0x0200)
	}
	clear(c.canvasTiles[:])
	hw.Fill32(unsafe.Slice((*byte)(unsafe.Pointer(&c.canvasCoverage[0])), objectCanvasCoverageWords*4), 0, objectCanvasCoverageWords)
	clear(c.canvasCandidates[:])
	c.mode0UploadStats = RendererUploadStats{}
	for id := 1; id < c.cart.SpriteN; id++ {
		if c.stageMode0Tile(id) {
			hw.CopyToVRAM32DMA(c.mode0TileStage[:], hw.VRAM+uintptr(id*64), 16)
			c.mode0UploadStats.ColdTileBytes += 64
		}
	}
	for layer := 0; layer < 4; layer++ {
		hw.SetReg16(hw.BG0CNT+uintptr(layer*2), mode0LayerControl(layer))
		hw.SetReg16(hw.BG0HOFS+uintptr(layer*4), 0)
		hw.SetReg16(hw.BG0VOFS+uintptr(layer*4), 0)
	}
	c.paletteDirty = true
	c.ApplyDisplayPalette()
}

func (c *Context) BeginRenderFrame() {
	c.canvasDrawn = false
	c.canvasHasPixels = false
	c.canvasRunCount = 0
	c.mode0DrawActive = true
	c.mode0ClsCalled = false
	c.mode0MapCalls = 0
	c.mode0MapSeen = 0
	c.canvasLastBG = -1
	c.mode0UploadStats.LastTileBytes = 0
	c.mode0UploadStats.LastMapBytes = 0
	c.mode0UploadStats.LastOBJTileBytes = 0
	c.mode0UploadStats.LastOAMBytes = 0
}

func (c *Context) clearRender(color int) {
	if c.canvasDrawn {
		panic("tile canvas requires one frame-initial cls; use Mode 4")
	}
	c.ensureDrawState()
	c.canvasDrawn = true
	c.canvasHasPixels = false
	c.mode0ClsCalled = true
	c.mode0Clear = c.drawPal[color]
	c.ClipReset()
	c.cursor.x, c.cursor.y = 0, 0
	// Exact coverage distinguishes opaque zero from untouched pixels. Old
	// framebuffer bytes outside that mask need no clear or upload.
	hw.Fill32(unsafe.Slice((*byte)(unsafe.Pointer(&c.canvasCoverage[0])), objectCanvasCoverageWords*4), 0, objectCanvasCoverageWords)
	clear(c.canvasCandidates[:])
}

func (c *Context) checkCanvasMap(slot, bg int) {
	if !c.canvasDrawn || c.canvasHasPixels || slot != bg || bg <= c.canvasLastBG || bg < 0 || bg >= 4 {
		panic("tile canvas requires ordered backgrounds before foreground raster calls; use Mode 4")
	}
	c.ensureDrawState()
	if !c.drawPalIdentity || !c.transpKey0 || c.clipX0 != 0 || c.clipY0 != 0 || c.clipX1 != ScreenWidth || c.clipY1 != ScreenHeight {
		panic("tile canvas maps require identity draw palette, key-zero transparency and full clip; use Mode 4")
	}
	c.canvasLastBG = bg
}

func (c *Context) PrepareRender() {
	c.lastBlitCycles = 0
	if !c.canvasDrawn {
		if c.canvasHasPixels {
			panic("tile canvas foreground requires frame-initial cls; use Mode 4")
		}
		// An empty draw is intentional during hit-stop: keep the complete
		// prior hardware image, including its scroll, palette and OAM.
		return
	}
	start := hw.ReadFrameCycleCounter()
	c.canvasResult = packObjectCanvas(c.framebuffer[:], &c.canvasCoverage, &c.canvasCandidates, c.canvasTiles[:], c.canvasOAM[:])
	if c.canvasResult.status != objectCanvasPackOK {
		canvasDiagnosticFailure("object canvas capacity", uint32(c.canvasResult.status))
	}
	var cost, bytes uint32
	c.canvasRunCount, cost, bytes = prepareObjectCanvasRuns(&c.canvasResult.changed, &c.canvasRuns)
	c.mode0UploadStats.LastOBJTileBytes = bytes
	c.canvasUploadEstimate = canvasCommitOverhead + cost + 2*objectCanvasOAMBytes + 192
	for layer := 0; layer < 4; layer++ {
		if c.mode0MapSeen&(1<<uint(layer)) != 0 && c.mode0MapDirty[layer] {
			c.canvasUploadEstimate += 2*4096 + 192
		}
	}
	c.lastBlitCycles = hw.ReadFrameCycleCounter() - start
}

func (c *Context) PresentRender() {
	c.mode0DrawActive = false
	if !c.canvasDrawn {
		return
	}
	if c.canvasUploadEstimate > 68*1232 {
		canvasDiagnosticFailure("tile canvas upload budget", c.canvasUploadEstimate)
	}
	line := hw.GetReg16(hw.VCOUNT)
	if line < 160 || c.canvasUploadEstimate > uint32(228-int(line)-1)*1232 {
		hw.WaitVDraw()
		hw.WaitForPresentWindow()
	}
	c.lastBlitStart = hw.GetReg16(hw.VCOUNT)
	start := hw.ReadFrameCycleCounter()
	for layer := 0; layer < 4; layer++ {
		hw.SetReg16(hw.BG0HOFS+uintptr(layer*4), c.mode0ScrollX[layer])
		hw.SetReg16(hw.BG0VOFS+uintptr(layer*4), c.mode0ScrollY[layer])
	}
	c.applyCanvasPalette()
	for layer := 0; layer < 4; layer++ {
		if c.mode0MapSeen&(1<<uint(layer)) != 0 && c.mode0MapDirty[layer] {
			hw.CopyToVRAM32DMA(c.mode0Screen[layer][:], mode0LayerScreenBase(layer), 1024)
			c.mode0UploadStats.LastMapBytes += 4096
			c.mode0MapDirty[layer] = false
		}
	}
	for _, run := range c.canvasRuns[:c.canvasRunCount] {
		first, size := int(run&127)*256, int(run>>7)*256
		hw.CopyToVRAM32DMA(c.canvasTiles[first:first+size], hw.OBJVRAM+uintptr(first), size/4)
	}
	hw.CopyToVRAM32DMA(c.canvasOAM[:], hw.OAM, objectCanvasOAMBytes/4)
	c.mode0UploadStats.LastOAMBytes = objectCanvasOAMBytes
	c.mode0UploadStats.LastTileBytes = c.mode0UploadStats.LastOBJTileBytes
	c.canvasCommittedObjects = uint32(c.canvasResult.objects)
	c.lastBlitEnd = hw.GetReg16(hw.VCOUNT)
	end := hw.ReadFrameCycleCounter()
	elapsed := end - start
	c.lastBlitCycles += elapsed
	c.accountPostPaceWork(elapsed, end)
	if c.lastBlitEnd < 160 || elapsed >= 68*1232 {
		hw.SetReg16(hw.DISPCNT, forceBlank)
		canvasDiagnosticFailure("tile canvas late commit", elapsed)
	}
	hw.SetReg16(hw.DISPCNT, uint16(c.mode0MapSeen)<<8|0x1040)
}

// Combine both hardware palettes in one IWRAM pass. OBJ palette zero is
// transparent; its dynamically chosen opaque-zero entry uses logical zero.
// BG palette zero remains the cls backdrop, with no reserved source colors.
//
//go:noinline
//go:section .iwram
func (c *Context) applyCanvasPalette() {
	zero := c.palette[c.displayPal[0]]
	backdrop := c.palette[c.displayPal[c.mode0Clear]]
	for i := 0; i < 256; i += 2 {
		lo, hi := c.palette[c.displayPal[i]], c.palette[c.displayPal[i+1]]
		objLo, objHi := lo, hi
		if byte(i) == c.canvasResult.opaqueZero {
			objLo = zero
		}
		if byte(i+1) == c.canvasResult.opaqueZero {
			objHi = zero
		}
		if i == 0 {
			lo = backdrop
		}
		hw.SetPaletteRAM32(i, lo, hi)
		hw.SetPaletteRAM32(256+i, objLo, objHi)
	}
	c.paletteDirty = false
}

func canvasDiagnosticFailure(message string, value uint32) {
	hw.SetReg16(0x04fff780, 0xc0de)
	for i := 0; i < len(message); i += 2 {
		v := uint16(message[i]) | 0x2000
		if i+1 < len(message) {
			v = uint16(message[i]) | uint16(message[i+1])<<8
		}
		hw.SetReg16(0x04fff600+uintptr(i), v)
	}
	pos := (len(message) + 1) &^ 1
	hw.SetReg16(0x04fff600+uintptr(pos), 0x7820)
	const digits = "0123456789abcdef"
	for i := 0; i < 8; i += 2 {
		a, b := digits[(value>>uint(28-i*4))&15], digits[(value>>uint(24-i*4))&15]
		hw.SetReg16(0x04fff600+uintptr(pos+2+i), uint16(a)|uint16(b)<<8)
	}
	hw.SetReg16(0x04fff600+uintptr(pos+10), 0)
	hw.SetReg16(0x04fff700, 0x101)
	panic("tile canvas presentation rejected; inspect emulator log or use Mode 4")
}

func (c *Context) SetBlitKind(kind BlitKind) { c.blitKind = kind }
func (c *Context) BlitKind() BlitKind        { return c.blitKind }
func (c *Context) PrepareHiddenPage()        { c.PrepareRender() }
func (c *Context) BlitToHiddenPage() bool    { c.PrepareRender(); return true }
func (c *Context) FlipPage()                 {}
