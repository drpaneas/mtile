//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

const (
	mode0ObjectTileLimit = Mode0ObjectTileLimit
	mode0ObjectLimit     = 128 // physical OAM entry count; scanline cycles are proven statically
	mode0ObjectDisable   = 0x0200
)

type mode0ObjectState struct {
	mode0TextState
	mode0PoseStreamState
	// mode0OAM must stay 4-byte aligned: presentMode0Objects uploads it with
	// DMA32, whose hardware source address cannot begin on a halfword boundary.
	mode0OAM [hw.OAMBytes]byte
	mode0ObjectPackState
	mode0ObjectCount       int
	mode0PreviousObjs      int
	mode0OAMDirty          bool
	mode0ObjectZeroVisible bool
	mode0PsprLastID        [mode0PsprSiteLimit]int
	mode0PsprSeq           [mode0PsprSiteLimit]uint16
	mode0PsprPending       [mode0PsprSiteLimit]mode0PsprPending
	mode0PsprTiles         [mode0PsprWindowLimit]uint16
	mode0PsprOwner         [mode0PsprWindowLimit]uint8
	mode0PsprSiteCount     int
	mode0PsprDMAFirst      int
	mode0PsprDMALast       int
	mode0PsprFallback      bool
	mode0PoseResidentReady bool
}

func mode0ObjectDisplayBits() uint16 {
	return 0x1040 // OBJ_ON | one-dimensional OBJ tile mapping
}

func mode0PutOAM16(data *[hw.OAMBytes]byte, offset int, value uint16) {
	data[offset] = byte(value)
	data[offset+1] = byte(value >> 8)
}

// OAM entries are eight bytes in the EWRAM shadow. Store the complete entry
// as two aligned words instead of eight byte stores. The fourth attribute
// remains zero: these are regular objects, without affine parameter slots.
func mode0PutOAMEntry(data *[hw.OAMBytes]byte, slot int, attr0, attr1, attr2 uint16) {
	if uint(slot) >= mode0ObjectLimit {
		return
	}
	offset := slot * 8
	pointer := unsafe.Pointer(&data[offset])
	if littleEndianNumberWords && uintptr(pointer)&3 == 0 {
		*(*uint32)(pointer) = uint32(attr0) | uint32(attr1)<<16
		*(*uint32)(unsafe.Add(pointer, 4)) = uint32(attr2)
		return
	}
	mode0PutOAM16(data, offset, attr0)
	mode0PutOAM16(data, offset+2, attr1)
	mode0PutOAM16(data, offset+4, attr2)
	mode0PutOAM16(data, offset+6, 0)
}

func (c *Context) applyMode0ObjectPalette() {
	for index := 0; index < 256; index++ {
		hw.SetPaletteRAM16(256+index, c.palette[c.rendererPaletteIndex(index)])
	}
	if c.cart != nil {
		for index, source := range c.cart.PoseOBJPalettes {
			hw.SetPaletteRAM16(256+index, c.palette[c.rendererPaletteIndex(int(source))])
		}
	}
}

// Both banks use the same draw/display remaps. Consume the shared dirty flag
// only after copying both, so OAM movement alone never rewrites 256 colors.
// Palette changes are committed even on a frame with no visible objects.
func (c *Context) applyMode0Palettes() {
	c.ensurePaletteState()
	if !c.paletteDirty {
		return
	}
	// Both banks have the same mapping. Resolve each color once and write
	// paired halfwords to each bank, avoiding a second 256-color remap pass.
	for index := 0; index < 256; index += 2 {
		lo := c.palette[c.rendererPaletteIndex(index)]
		hi := c.palette[c.rendererPaletteIndex(index+1)]
		hw.SetPaletteRAM32(index, lo, hi)
		hw.SetPaletteRAM32(256+index, lo, hi)
	}
	if c.cart != nil && len(c.cart.PoseOBJPalettes) > 0 {
		for index, source := range c.cart.PoseOBJPalettes {
			hw.SetPaletteRAM16(256+index, c.palette[c.rendererPaletteIndex(int(source))])
		}
	}
	c.paletteDirty = false
}

func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	if sx < 0 || sy < 0 || sx&7 != 0 || sy&7 != 0 || sw <= 0 || sh <= 0 ||
		sw&7 != 0 || sh&7 != 0 || dw != sw || dh != sh || c.cart == nil {
		return true
	}
	id := (sy/8)*c.cart.SpriteCols + sx/8
	c.renderSprite(id, dx, dy, sw/8, sh/8, flipX, flipY)
	return true
}

func (c *Context) initMode0Objects() {
	if c == nil || c.cart == nil {
		return
	}
	c.mode0PoseResidentReady = false
	for offset := 0; offset < 32*1024; offset += 4 {
		hw.SetVRAM32(hw.OBJVRAM+uintptr(offset), 0)
	}
	for object := 0; object < 128; object++ {
		mode0PutOAM16(&c.mode0OAM, object*8, mode0ObjectDisable)
	}
	hw.CopyToVRAM32DMA(c.mode0OAM[:], hw.OAM, len(c.mode0OAM)/4)
	if !c.stageMode0Tile(0) {
		for index := range c.mode0TileStage {
			c.mode0TileStage[index] = 0
		}
	}
	c.uploadMode0ObjectTile(0, c.mode0TileStage[:])
	limit := c.cart.SpriteN
	if limit > mode0ObjectTileLimit {
		limit = mode0ObjectTileLimit
	}
	for id := 1; id < limit; id++ {
		if c.stageMode0Tile(id) {
			c.uploadMode0ObjectTile(id, c.mode0TileStage[:])
		}
	}
	c.applyMode0ObjectPalette()
	for site := range c.mode0PsprLastID {
		c.mode0PsprLastID[site] = -1
	}
}

func (c *Context) uploadMode0ObjectTile(id int, tile []byte) uint32 {
	if id < 0 || id >= mode0ObjectTileLimit || mode0PackedObjectReservedSource(id) ||
		mode0TextObjectTileReserved(id) || len(tile) < 64 {
		return 0
	}
	if id == 0 {
		c.mode0ObjectZeroVisible = mode0ObjectTileHasVisiblePixel(tile)
	}
	// Background-sheet mutations also update the legacy resident OBJ bank.
	// A streamed pose may currently own that destination, so restore its ROM
	// pixels in the later pose upload pass even when its pose ID is unchanged.
	c.invalidateMode0PsprTile(id)
	hw.CopyToVRAM32DMA(tile[:64], hw.OBJVRAM+uintptr(id*64), 16)
	return 64
}

func mode0ObjectTileHasVisiblePixel(tile []byte) bool {
	limit := len(tile)
	if limit > 64 {
		limit = 64
	}
	for index := 0; index < limit; index++ {
		if tile[index] != 0 {
			return true
		}
	}
	return false
}

func (c *Context) mode0ObjectTileMutated(id int) {
	if c == nil {
		return
	}
	c.invalidateMode0PsprSource(id)
	if id != 0 {
		return
	}
	if !c.stageMode0Tile(0) {
		c.mode0ObjectZeroVisible = false
		return
	}
	c.mode0ObjectZeroVisible = mode0ObjectTileHasVisiblePixel(c.mode0TileStage[:])
}

func (c *Context) mode0ObjectIntersectsClip(x, y, width, height int) bool {
	return c != nil && width > 0 && height > 0 &&
		x < c.clipX1 && y < c.clipY1 && x+width > c.clipX0 && y+height > c.clipY0
}

// mode0ObjectPriorityBits derives the semantic depth lane from painter order.
// Each materialized map before the object is behind it; every remaining map
// is in front. An OBJ wins ties with a BG at the same priority, so N-seen is
// the exact Attribute 2 priority for every representable lane. The compiler
// rejects objects behind all materialized BGs; the zero-map fallback keeps
// direct runtime tests and mapless cartridges safely frontmost.
func (c *Context) mode0ObjectPriorityBits() uint16 {
	if c == nil || c.mode0MapCalls <= 0 {
		return 0
	}
	priority := mode0LayerCount - c.mode0MapCalls
	if priority < 0 {
		priority = 0
	}
	if priority > 3 {
		priority = 3
	}
	return uint16(priority) << 10
}

func (c *Context) beginMode0Objects() {
	c.beginMode0Text()
	c.mode0PsprSeq = [mode0PsprSiteLimit]uint16{}
	for index := 0; index < c.mode0PreviousObjs; index++ {
		mode0PutOAM16(&c.mode0OAM, index*8, mode0ObjectDisable)
	}
	c.mode0OAMDirty = c.mode0PreviousObjs != 0
	c.mode0ObjectCount = 0
}

// compactMode0OAM moves the reverse-built painter stack to OAM0..N-1 before
// DMA. Later draw calls retain lower OAM indices (and therefore higher OBJ
// priority), while active entries remain contiguous from OAM0 as required for
// the documented maximum per-scanline rendering capacity.
func (c *Context) compactMode0OAM() {
	count := c.mode0ObjectCount
	if count <= 0 || count >= mode0ObjectLimit {
		// No live entries need moving at zero; a full OAM already begins at
		// entry zero and has no disabled tail. Avoid copying it onto itself.
		return
	}
	sourceOffset := (mode0ObjectLimit - count) * 8
	// The shared ARM memmove copies aligned blocks in register bursts and
	// preserves overlap when more than half of OAM is occupied. Host copy
	// retains the same semantics, including unusual alignment.
	copy(c.mode0OAM[:count*8], c.mode0OAM[sourceOffset:])
	inactiveStart := count
	if sourceStart := mode0ObjectLimit - count; sourceStart > inactiveStart {
		inactiveStart = sourceStart
	}
	for object := inactiveStart; object < mode0ObjectLimit; object++ {
		mode0PutOAM16(&c.mode0OAM, object*8, mode0ObjectDisable)
	}
}

func (c *Context) renderSprite(id, x, y, w, h int, flipX, flipY bool) {
	if c == nil || c.cart == nil || id < 0 || w <= 0 || h <= 0 {
		return
	}
	cols := c.cart.SpriteCols
	if cols <= 0 {
		return
	}
	if id == 0 && w == 1 && h == 1 && !c.mode0ObjectZeroVisible {
		return
	}
	if w == 1 && h == 1 {
		// A single cell cannot cross a source row. Validated sheet dimensions
		// fit in an int, so its linear bound needs only one multiplication.
		// Avoid the software id/cols division on every ordinary 8x8 OBJ.
		if id >= cols*c.cart.SpriteRows {
			return
		}
	} else {
		baseCellX, baseCellY := id%cols, id/cols
		if baseCellX+w > cols || baseCellY+h > c.cart.SpriteRows {
			return
		}
	}
	baseX, baseY := x-c.camX, y-c.camY
	if w == 1 && h == 1 {
		if id >= c.cart.SpriteN || id >= mode0ObjectTileLimit ||
			c.mode0ObjectCount >= mode0ObjectLimit || !c.mode0ObjectIntersectsClip(baseX, baseY, 8, 8) {
			return
		}
		attr0 := uint16(baseY&0xff) | 0x2000
		attr1 := uint16(baseX & 0x1ff)
		if flipX {
			attr1 |= 1 << 12
		}
		if flipY {
			attr1 |= 1 << 13
		}
		mode0PutOAMEntry(&c.mode0OAM, 127-c.mode0ObjectCount, attr0, attr1,
			uint16(id*2)|c.mode0ObjectPriorityBits())
		c.mode0ObjectCount++
		c.mode0OAMDirty = true
		return
	}
	packedCells := 0
	if w == h && (w == 2 || w == 4) {
		packedCells = w
	}
	if packedCells != 0 && c.mode0ObjectCount < mode0ObjectLimit {
		if tile, ok := c.mode0PackedObjectTile(id, packedCells); ok {
			// Populate the deterministic packing cache even for an offscreen
			// first use, then suppress the OAM entry before coordinate masking
			// can alias it back into the viewport.
			if !c.mode0ObjectIntersectsClip(baseX, baseY, w*8, h*8) {
				return
			}
			slot := 127 - c.mode0ObjectCount
			attr0 := uint16(baseY&0xff) | 0x2000 // 8bpp, square
			attr1 := uint16(baseX&0x1ff) | uint16(packedCells/2)<<14
			if flipX {
				attr1 |= 1 << 12
			}
			if flipY {
				attr1 |= 1 << 13
			}
			mode0PutOAMEntry(&c.mode0OAM, slot, attr0, attr1, uint16(tile*2)|c.mode0ObjectPriorityBits())
			c.mode0ObjectCount++
			c.mode0OAMDirty = true
			return
		}
	}
	if !c.mode0ObjectIntersectsClip(baseX, baseY, w*8, h*8) {
		return
	}
	if c.mode0ObjectCount+w*h > mode0ObjectLimit {
		return
	}
	wroteObject := false
	for drawY := 0; drawY < h; drawY++ {
		for drawX := 0; drawX < w; drawX++ {
			objectX, objectY := baseX+drawX*8, baseY+drawY*8
			if !c.mode0ObjectIntersectsClip(objectX, objectY, 8, 8) {
				continue
			}
			sourceX, sourceY := drawX, drawY
			if flipX {
				sourceX = w - 1 - drawX
			}
			if flipY {
				sourceY = h - 1 - drawY
			}
			sourceID := id + sourceY*cols + sourceX
			if sourceID < 0 || sourceID >= c.cart.SpriteN || sourceID >= mode0ObjectTileLimit {
				continue
			}
			if sourceID == 0 && !c.mode0ObjectZeroVisible {
				continue
			}
			slot := 127 - c.mode0ObjectCount
			attr0 := uint16(objectY&0xff) | 0x2000 // 8bpp, square 8x8
			attr1 := uint16(objectX & 0x1ff)
			if flipX {
				attr1 |= 1 << 12
			}
			if flipY {
				attr1 |= 1 << 13
			}
			attr2 := uint16(sourceID*2) | c.mode0ObjectPriorityBits() // 8bpp uses two 32-byte tile-number units
			mode0PutOAMEntry(&c.mode0OAM, slot, attr0, attr1, attr2)
			c.mode0ObjectCount++
			wroteObject = true
		}
	}
	if wroteObject {
		c.mode0OAMDirty = true
	}
}

func (c *Context) presentMode0Objects() uint32 {
	c.mode0UploadStats.LastOBJTileBytes += c.presentMode0PackedObjects() + c.presentMode0Text() + c.presentMode0PsprStreams()
	if !c.mode0OAMDirty {
		return 0
	}
	c.compactMode0OAM()
	hw.CopyToVRAM32DMA(c.mode0OAM[:], hw.OAM, len(c.mode0OAM)/4)
	c.mode0PreviousObjs = c.mode0ObjectCount
	c.mode0OAMDirty = false
	return uint32(len(c.mode0OAM))
}
