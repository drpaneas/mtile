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
)

func mode0LayerScreenBase(layer int) uintptr {
	return hw.VRAM + uintptr((30-layer*2)*2048)
}

func mode0LayerControl(layer int) uint16 {
	// Later map() calls are in front, matching Mode 4 painter order.
	priority := mode0LayerCount - 1 - layer
	return uint16(priority) | 0x0080 | uint16(30-layer*2)<<8 | 0x4000
}

func mode0LayerDisplay(seen uint8) uint16 {
	return uint16(seen)<<8 | mode0ObjectDisplayBits()
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

// FinalizeInitHardware commits sset() calls made by _init while the LCD is
// still force-blanked. Per-frame sset() is separately bounded by the compiler
// so the later visible commit remains inside VBlank.
func (c *Context) FinalizeInitHardware() {
	if c == nil {
		return
	}
	start := hw.ReadFrameCycleCounter()
	uploaded, _ := c.uploadMode0DirtyTiles()
	c.mode0ColdCycles += hw.ReadFrameCycleCounter() - start
	c.mode0UploadStats.ColdTileBytes += uploaded
}

// stageMode0Tile resolves either generated sheet representation into the
// fixed 64-byte GBA tile layout. It returns false for an implicit empty tile.
func (c *Context) stageMode0Tile(id int) bool {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN {
		return false
	}
	if sheetStorageReadOnly || c.cart.SheetReadOnly {
		if id >= len(c.cart.SheetIndex) {
			return false
		}
		slot := int(c.cart.SheetIndex[id])
		if slot == 0 {
			for i := range c.mode0TileStage {
				c.mode0TileStage[i] = 0
			}
			return false
		}
		start := (slot - 1) * 64
		if start < 0 || start+64 > len(c.cart.Sheet) {
			return false
		}
		copy(c.mode0TileStage[:], c.cart.Sheet[start:start+64])
		return true
	}
	cols := c.cart.SpriteCols
	if cols <= 0 {
		return false
	}
	cellX, cellY := id%cols, id/cols
	startX, startY := cellX*8, cellY*8
	if startX+8 > c.cart.SheetW || startY+8 > c.cart.SheetH {
		return false
	}
	for row := 0; row < 8; row++ {
		src := (startY+row)*c.cart.SheetW + startX
		copy(c.mode0TileStage[row*8:row*8+8], c.cart.Sheet[src:src+8])
	}
	return true
}

func (c *Context) clearRender(color int) {
	c.ensureDrawState()
	c.ClipReset()
	c.cursor.x, c.cursor.y = 0, 0
	c.mode0Clear = c.drawPal[color]
	c.mode0ClsCalled = true
}

func positiveMod(value, modulus int) int {
	result := value % modulus
	if result < 0 {
		result += modulus
	}
	return result
}

// mode0ScreenEntry implements the GBA's 64x32 regular-background block
// layout: the right half lives in the second 32x32 screen block.
func mode0ScreenEntry(x, y int) int {
	x = positiveMod(x, 64)
	y = positiveMod(y, 32)
	return (x/32)*1024 + y*32 + x%32
}

// Map preserves the Mode 4 map contract while materializing only the window
// required by the current call. Each statically proven call owns one hardware
// BG; fine camera movement updates scroll registers without a screen upload.
func (c *Context) Map(cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if c == nil {
		return
	}
	mapIndex := c.mode0MapCalls
	c.mapLayerAt(mapIndex, 0, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
}

// MapBG binds one authored plane to its matching logical hardware BG slot.
// The transpiler proves stable BG IDs and ordering before selecting Mode 0.
func (c *Context) MapBG(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	c.mapLayerAt(bg, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
}

// MapBGAt binds a stable logical source plane to a densely allocated hardware
// BG slot. Empty gaps in BG0..BG3 therefore consume neither a screen shadow
// nor a hardware background.
func (c *Context) MapBGAt(slot, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if slot < 0 {
		return
	}
	c.mapLayerAt(slot, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
}

// MapAll materializes the selected non-empty logical planes in BG order while
// packing them into consecutive hardware slots.
func (c *Context) MapAll(mask uint8) {
	slot := 0
	for bg := 0; bg < 4; bg++ {
		if mask&(1<<uint(bg)) == 0 {
			continue
		}
		c.MapBGAt(slot, bg, 0, 0, 0, 0, 0, 0, 0)
		slot++
	}
}

func (c *Context) mapLayerAt(mapIndex, sourceBG, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if c == nil || c.cart == nil {
		return
	}
	if sourceBG < 0 || sourceBG >= c.cart.MapLayers {
		return
	}
	c.ensureDrawState()
	if cellsW <= 0 {
		cellsW = (c.clipX1 - c.clipX0 + 7) / 8
		if hidden := c.clipX0 - (screenX + cellX*8 - c.camX); hidden > 0 {
			cellsW += (hidden + 7) / 8
		}
	}
	if cellsH <= 0 {
		cellsH = (c.clipY1 - c.clipY0 + 7) / 8
		if hidden := c.clipY0 - (screenY + cellY*8 - c.camY); hidden > 0 {
			cellsH += (hidden + 7) / 8
		}
	}
	if cellsW <= 0 || cellsH <= 0 || cellsW > 64 || cellsH > 32 {
		return
	}
	if mapIndex >= mode0LayerCount {
		return
	}
	c.mode0MapCalls++
	c.mode0MapSeen |= 1 << uint(mapIndex)

	originX := screenX + cellX*8 - c.camX
	originY := screenY + cellY*8 - c.camY
	scrollX := positiveMod(-originX, 8)
	scrollY := positiveMod(-originY, 8)
	tileX := (originX + scrollX) / 8
	tileY := (originY + scrollY) / 8
	c.mode0ScrollX[mapIndex] = uint16(scrollX)
	c.mode0ScrollY[mapIndex] = uint16(scrollY)

	var mask byte
	if layer > 0 && layer <= 255 {
		mask = byte(layer)
	}
	if c.mode0MapSigSet[mapIndex] && c.mode0MapCellX[mapIndex] == cellX && c.mode0MapCellY[mapIndex] == cellY &&
		c.mode0MapTileX[mapIndex] == tileX && c.mode0MapTileY[mapIndex] == tileY &&
		c.mode0MapCellsW[mapIndex] == cellsW && c.mode0MapCellsH[mapIndex] == cellsH &&
		c.mode0MapLayer[mapIndex] == mask && c.mode0MapSource[mapIndex] == uint8(sourceBG) {
		return
	}

	for i := range c.mode0Screen[mapIndex] {
		c.mode0Screen[mapIndex][i] = 0
	}
	for my := 0; my < cellsH; my++ {
		screenCellY := originY + my*8
		// Keep the cell exactly on the right/bottom boundary as a guard.
		// A one-pixel pan changes only hardware scroll (not the cached tile
		// placement), so that cell can become visible without rematerializing.
		if screenCellY+8 <= c.clipY0 || screenCellY > c.clipY1 {
			continue
		}
		mapY := cellY + my
		if mapY < 0 || mapY >= c.cart.MapH {
			continue
		}
		for mx := 0; mx < cellsW; mx++ {
			screenCellX := originX + mx*8
			if screenCellX+8 <= c.clipX0 || screenCellX > c.clipX1 {
				continue
			}
			mapX := cellX + mx
			if mapX < 0 || mapX >= c.cart.MapW {
				continue
			}
			id := c.mapGetLayer(sourceBG, mapX, mapY)
			if id == 0 || int(id) >= c.cart.SpriteN || id >= mode0TileLimit {
				continue
			}
			if mask != 0 && c.cart.Flags[id]&mask != mask {
				continue
			}
			entry := mode0ScreenEntry(tileX+mx, tileY+my) * 2
			c.mode0Screen[mapIndex][entry] = byte(id)
			c.mode0Screen[mapIndex][entry+1] = byte(id >> 8)
		}
	}
	c.mode0MapCellX[mapIndex], c.mode0MapCellY[mapIndex] = cellX, cellY
	c.mode0MapTileX[mapIndex], c.mode0MapTileY[mapIndex] = tileX, tileY
	c.mode0MapCellsW[mapIndex], c.mode0MapCellsH[mapIndex] = cellsW, cellsH
	c.mode0MapLayer[mapIndex] = mask
	c.mode0MapSource[mapIndex] = uint8(sourceBG)
	c.mode0MapSigSet[mapIndex] = true
	c.mode0MapReady[mapIndex] = true
	c.mode0MapDirty[mapIndex] = true
}

func (c *Context) BeginRenderFrame() {
	c.mode0DrawActive = true
	c.mode0ClsCalled = false
	c.mode0MapCalls = 0
	c.mode0MapSeen = 0
	c.beginMode0Objects()
}

// PrepareRender is intentionally empty: Mode 0 has no software framebuffer
// to copy during VDraw.
func (c *Context) PrepareRender() {}

func (c *Context) uploadMode0DirtyTiles() (uint32, uint32) {
	if mode0SheetImmutable {
		return 0, 0
	}
	if c == nil || c.cart == nil {
		return 0, 0
	}
	var bgUploaded, objUploaded uint32
	limit := c.cart.SpriteN
	if limit > mode0TileLimit {
		limit = mode0TileLimit
	}
	for id := 0; id < limit; id++ {
		mask := byte(1 << uint(id&7))
		if c.mode0TileDirty[id>>3]&mask == 0 {
			continue
		}
		c.mode0TileDirty[id>>3] &^= mask
		staged := c.stageMode0Tile(id)
		if !staged {
			for i := range c.mode0TileStage {
				c.mode0TileStage[i] = 0
			}
		}
		if id == 0 {
			objUploaded += c.uploadMode0ObjectTile(id, c.mode0TileStage[:])
			continue
		}
		hw.CopyToVRAM32DMA(c.mode0TileStage[:], hw.VRAM+uintptr(id*64), 16)
		objUploaded += c.uploadMode0ObjectTile(id, c.mode0TileStage[:])
		bgUploaded += 64
	}
	return bgUploaded, objUploaded
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
	c.ApplyDisplayPalette()
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
