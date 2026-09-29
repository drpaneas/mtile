//go:build mtile_mode0

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

// Each Mode 0 map() call owns one 64x32 regular BG. A zero-map build presents
// only the backdrop and optional OBJs. Screen-block pairs are allocated
// downward from 30/31; the remaining lower VRAM holds shared 8bpp character
// tiles. mode0LayerCount is selected by a link-time build tag.
const (
	mode0ForceBlank  = 0x0080
	mode0Display     = 0x0100 // one-layer compatibility value used by tests
	mode0BG0Control  = 0x5E80 // one-layer compatibility value used by tests
	mode0ScreenBase  = hw.VRAM + 60*1024
	mode0BGVRAMBytes = 64 * 1024
	mode0TileLimit   = (30 - 2*(mode0LayerCount-1)) * 32
	// A commit includes CPU work as well as DMA: up to four 4 KiB maps,
	// mutable/packed tiles, OAM compaction, and both hardware palettes. Reserve
	// all 67 complete scanlines for the general path. A separately verified
	// immutable warm path uses mode0RequiredCommitCycles to account for its
	// actual dirty maps. VCOUNT cannot tell how much of the current scanline
	// has elapsed, so neither path counts that partial line. Preparation and
	// compiler resource limits must still keep the actual commit inside one
	// VBlank; waiting cannot make oversized work fit. LastBlitWindow records
	// the actual transfer for native verification.
	mode0CommitReserve = (scanlinesPerFrame - ScreenHeight - 1) * cyclesPerScanline
)

func mode0LayerDisplay(seen uint8) uint16 {
	return uint16(seen)<<8 | mode0ObjectDisplayBits()
}

func mode0PresentWindowFits(line uint16, cycles uint32) bool {
	if line < ScreenHeight || line >= scanlinesPerFrame {
		return false
	}
	completeLines := scanlinesPerFrame - int(line) - 1
	return cycles <= uint32(completeLines*cyclesPerScanline)
}

// InitHardware materializes the cartridge sheet in tile VRAM. Immutable
// sheets use the transpiler's compact cell table; a cartridge that calls
// sset() uses its full row-major sheet and stages each 8x8 cell once.
func (c *Context) InitHardware() {
	if c == nil {
		return
	}
	hw.StartFrameCycleCounter()
	coldStart := hw.ReadFrameCycleCounter()
	hw.SetReg16(hw.DISPCNT, mode0ForceBlank)
	for layer := 0; layer < mode0LayerCount; layer++ {
		hw.SetReg16(hw.BG0CNT+uintptr(layer*2), mode0LayerControl(layer))
		hw.SetReg16(hw.BG0HOFS+uintptr(layer*4), 0)
		hw.SetReg16(hw.BG0VOFS+uintptr(layer*4), 0)
	}
	if c.cart == nil {
		return
	}
	var uploaded uint32
	// A zero-layer cartridge can still use the authored map, flags, and sheet
	// as gameplay data, or render with OBJs. It cannot observe BG character or
	// screen VRAM, though, so do not clear 64 KiB or stage the sheet into the
	// unused BG tile store. mode0LayerCount is selected by a build tag, making
	// this branch and its whole loop disappear from the linked zero-layer ROM.
	if mode0LayerCount != 0 {
		for offset := 0; offset < mode0BGVRAMBytes; offset += 4 {
			hw.SetVRAM32(hw.VRAM+uintptr(offset), 0)
		}
		if c.cart.SpriteN > mode0TileLimit {
			return
		}
		for id := 1; id < c.cart.SpriteN; id++ {
			if !c.stageMode0Tile(id) {
				continue
			}
			hw.CopyToVRAM32DMA(c.mode0TileStage[:], hw.VRAM+uintptr(id*64), 16)
			uploaded += 64
		}
	}
	c.paletteDirty = true
	c.ApplyDisplayPalette()
	c.initMode0Objects()
	c.mode0ColdCycles = hw.ReadFrameCycleCounter() - coldStart
	c.mode0UploadStats.ColdTileBytes = uploaded
}

// FinalizeInitHardware commits sset() and palette changes made by _init while
// the LCD is still force-blanked. Otherwise the first visible frame would need
// both complete palette uploads alongside its cold map/OAM commit. Per-frame
// sset() is separately bounded by the compiler for the later VBlank commits.
func (c *Context) FinalizeInitHardware() {
	if c == nil {
		return
	}
	start := hw.ReadFrameCycleCounter()
	uploaded, _ := c.uploadMode0DirtyTiles()
	c.applyMode0Palettes()
	c.initMode0ResidentPoses()
	c.mode0ColdCycles += hw.ReadFrameCycleCounter() - start
	c.mode0UploadStats.ColdTileBytes += uploaded
}

// stageMode0Tile resolves either generated sheet representation into the
// fixed 64-byte GBA tile layout. It returns false for an implicit empty tile.
func (c *Context) clearRender(color int) {
	c.ensureDrawState()
	c.ClipReset()
	c.cursor.x, c.cursor.y = 0, 0
	c.mode0Clear = c.drawPal[color]
	c.mode0ClsCalled = true
}

func (c *Context) BeginRenderFrame() {
	c.mode0DrawActive = true
	c.mode0ClsCalled = false
	c.mode0MapCalls = 0
	c.mode0MapSeen = 0
	c.beginMode0Objects()
}

// PrepareRender stages CPU-only upload work before waiting for VBlank. The
// visible commit then transfers the already prepared tiles and map shadows.
func (c *Context) PrepareRender() {
	c.prepareMode0TileUploads()
	c.prepareMode0PackedObjects()
	c.prepareMode0PsprStreams()
}

func (c *Context) PresentRender() {
	if c == nil {
		return
	}
	c.mode0DrawActive = false
	if !c.mode0ClsCalled || c.mode0MapCalls > mode0LayerCount {
		hw.SetReg16(hw.DISPCNT, mode0ForceBlank)
		return
	}
	for layer := 0; layer < mode0LayerCount; layer++ {
		if c.mode0MapSeen&(1<<uint(layer)) != 0 && !c.mode0MapReady[layer] {
			hw.SetReg16(hw.DISPCNT, mode0ForceBlank)
			return
		}
	}
	// The outer frame pacer may return late in an already running VBlank.
	// Starting visible map/OAM writes there lets the next scan observe only
	// part of a frame. Retain the previous complete frame until a window with
	// enough complete scanlines for the pending commit is available. The host
	// waits are inert; native first-scan fixtures verify the work bounds too.
	reserve := c.mode0RequiredCommitCycles()
	if !mode0PresentWindowFits(hw.GetReg16(hw.VCOUNT), reserve) {
		hw.WaitVDraw()
		hw.WaitForPresentWindow()
	}
	c.lastBlitStart = hw.GetReg16(hw.VCOUNT)
	commitStart := hw.ReadFrameCycleCounter()
	// Commit scroll first. A frame that changes several 4 KiB screen blocks
	// can consume most of VBlank; leaving BGxHOFS/BGxVOFS until after those
	// DMAs lets the next scan begin with the previous frame's offset and tears
	// the top lines. The maps are still copied entirely inside the same
	// presentation window, but their matching offsets are now latched before
	// active display can resume.
	for layer := 0; layer < mode0LayerCount; layer++ {
		hw.SetReg16(hw.BG0HOFS+uintptr(layer*4), c.mode0ScrollX[layer])
		hw.SetReg16(hw.BG0VOFS+uintptr(layer*4), c.mode0ScrollY[layer])
	}
	c.applyMode0Palettes()
	hw.SetPaletteRAM16(0, c.palette[c.displayPal[c.mode0Clear]])
	coldCycles := c.mode0ColdCycles
	c.mode0ColdCycles = 0
	c.mode0UploadStats.LastTileBytes = 0
	c.mode0UploadStats.LastMapBytes = 0
	c.mode0UploadStats.LastOAMBytes = 0
	c.mode0UploadStats.LastOBJTileBytes = 0

	c.mode0UploadStats.LastTileBytes, c.mode0UploadStats.LastOBJTileBytes = c.uploadMode0DirtyTiles()
	for layer := 0; layer < mode0LayerCount; layer++ {
		if c.mode0MapSeen&(1<<uint(layer)) == 0 || !c.mode0MapDirty[layer] {
			continue
		}
		hw.CopyToVRAM32DMA(c.mode0Screen[layer][:], mode0LayerScreenBase(layer), len(c.mode0Screen[layer])/4)
		c.mode0UploadStats.LastMapBytes += uint32(len(c.mode0Screen[layer]))
		c.mode0MapDirty[layer] = false
	}
	c.mode0UploadStats.LastOAMBytes = c.presentMode0Objects()
	hw.SetReg16(hw.DISPCNT, mode0LayerDisplay(c.mode0MapSeen))
	c.lastBlitEnd = hw.GetReg16(hw.VCOUNT)
	commitEnd := hw.ReadFrameCycleCounter()
	commitCycles := commitEnd - commitStart
	c.lastBlitCycles = coldCycles + commitCycles
	c.accountPostPaceWork(commitCycles, commitEnd)
}

// Legacy Mode 4 presentation methods remain as inert compatibility points for
// diagnostic packages. Generated Mode 0 games call only the lifecycle above.
func (c *Context) PrepareHiddenPage() { c.PrepareRender() }
func (c *Context) BlitToHiddenPage() bool {
	c.PrepareRender()
	return true
}
func (c *Context) FlipPage() {}
func (c *Context) SetBlitKind(kind BlitKind) {
	if kind >= BlitDMA && kind <= BlitIWRAM {
		c.blitKind = kind
	}
}
func (c *Context) BlitKind() BlitKind { return c.blitKind }
