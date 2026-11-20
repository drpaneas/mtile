//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

type mode0PsprPending struct {
	dirty       bool
	prepared    bool
	id, base, n int
}

func (c *Context) invalidateMode0PsprTile(tile int) {
	if uint(tile) >= mode0PsprWindowLimit {
		return
	}
	site := int(c.mode0PsprOwner[tile]) - 1
	if site < 0 {
		return
	}
	pending := &c.mode0PsprPending[site]
	if pending.n == 0 || tile < pending.base || tile >= pending.base+pending.n {
		return
	}
	if pending.prepared {
		// A background upload can overwrite OBJ VRAM after PrepareRender. The
		// cached pose pixels are already staged; extend its DMA in constant time.
		c.queueMode0PsprUpload(pending)
	} else {
		pending.dirty = true
		c.mode0PsprFallback = true
	}
}

func (c *Context) invalidateMode0PsprSource(tile int) {
	if c.cart == nil || c.cart.ObjSpriteN != 0 {
		return
	}
	for site := 0; site < c.mode0PsprSiteCount; site++ {
		pending := &c.mode0PsprPending[site]
		if c.mode0PsprLocalIndex(pending.base, pending.n, uint16(tile)) >= 0 {
			pending.dirty = true
			pending.prepared = false
			c.mode0PsprFallback = true
		}
	}
}

func mode0PsprWindowAvailable(base, count int) bool {
	if base < 0 || count < 0 || base > mode0PsprWindowLimit-count {
		return false
	}
	// Each optional atlas occupies one contiguous range. Check its overlap
	// once, rather than revisiting every tile of an unchanged pose each draw.
	return !mode0PackedObjectWindowReserved(base, count) && !mode0TextObjectWindowReserved(base, count)
}

func (c *Context) hasMode0PsprUploads() bool {
	return c != nil && (c.mode0PsprDMALast > c.mode0PsprDMAFirst || c.mode0PsprFallback)
}

func (c *Context) claimMode0PsprWindow(site int, pending *mode0PsprPending) {
	for tile := pending.base; tile < pending.base+pending.n; tile++ {
		c.mode0PsprOwner[tile] = uint8(site + 1)
	}
	c.mode0PsprFallback = true
}

func (c *Context) queueMode0PsprUpload(pending *mode0PsprPending) {
	if pending.n == 0 {
		return
	}
	if c.mode0PsprDMALast <= c.mode0PsprDMAFirst || pending.base < c.mode0PsprDMAFirst {
		c.mode0PsprDMAFirst = pending.base
	}
	if end := pending.base + pending.n; end > c.mode0PsprDMALast {
		c.mode0PsprDMALast = end
	}
}

func (c *Context) takeMode0PsprSite(base int) int {
	if c == nil || uint(base) >= mode0PsprSiteLimit {
		return -1
	}
	seq := int(c.mode0PsprSeq[base])
	if base+seq >= mode0PsprSiteLimit {
		return -1
	}
	c.mode0PsprSeq[base]++
	return base + seq
}

func (c *Context) mode0PSpr(site, windowBase, id, x, y int, flipX, flipY bool) {
	if c != nil && c.cart != nil && len(c.cart.PoseOBJResident) != 0 {
		c.mode0ResidentPSpr(id, x, y, flipX, flipY)
		return
	}
	if c == nil || c.cart == nil || uint(site) >= mode0PsprSiteLimit ||
		windowBase < 0 || windowBase >= mode0PsprWindowLimit {
		return
	}
	c.ensureDrawState()
	if site >= c.mode0PsprSiteCount {
		c.mode0PsprSiteCount = site + 1
	}
	pending := &c.mode0PsprPending[site]
	if id < 0 || id >= len(c.cart.PoseIndex) {
		pending.dirty, pending.n = false, 0
		c.mode0PsprLastID[site] = -1
		return
	}
	r := c.cart.PoseIndex[id]
	start, end := int(r.Start), int(r.Start)+int(r.Count)
	if start < 0 || start > len(c.cart.PoseParts) || end > len(c.cart.PoseParts) {
		return
	}
	changed := id != c.mode0PsprLastID[site] || pending.base != windowBase
	if id < len(c.cart.PoseOBJIndex) {
		plan := c.cart.PoseOBJIndex[id]
		objectStart, objectEnd := int(plan.Start), int(plan.Start)+int(plan.Count)
		tileStart, tileEnd := int(plan.TileStart), int(plan.TileStart)+int(plan.TileCount)
		if objectStart < 0 || objectEnd > len(c.cart.PoseOBJParts) || tileStart < 0 || tileEnd > len(c.cart.PoseOBJTiles) ||
			!mode0PsprWindowAvailable(windowBase, int(plan.TileCount)) {
			return
		}
		if changed {
			copy(c.mode0PsprTiles[windowBase:windowBase+int(plan.TileCount)], c.cart.PoseOBJTiles[tileStart:tileEnd])
			pending.dirty, pending.id, pending.base, pending.n = true, id, windowBase, int(plan.TileCount)
			pending.prepared = false
			c.claimMode0PsprWindow(site, pending)
		}
		for _, object := range c.cart.PoseOBJParts[objectStart:objectEnd] {
			c.emitMode0PoseObject(windowBase+int(object.TileOffset), x, y, object, flipX, flipY)
		}
		return
	}
	// Direct runtime callers and older generated images retain the cell plan.
	n := c.fillMode0PsprUniqueOrder(windowBase, c.cart.PoseParts[start:end])
	if n < 0 {
		return
	}
	if changed {
		pending.dirty, pending.id, pending.base, pending.n = true, id, windowBase, n
		pending.prepared = false
		c.claimMode0PsprWindow(site, pending)
	}
	for _, part := range c.cart.PoseParts[start:end] {
		local := c.mode0PsprLocalIndex(windowBase, n, part.Tile)
		if local < 0 {
			continue
		}
		c.emitMode0PoseObject(windowBase+local, x, y, PoseOBJ{X: part.X, Y: part.Y, Width: 8, Height: 8, Flags: part.Flags}, flipX, flipY)
	}
}

// This bank is immutable and occupies a compiler-proven disjoint OBJ range.
// Changing poses, draw counts, flips or positions only changes OAM.
func (c *Context) initMode0ResidentPoses() {
	if c == nil || c.cart == nil || len(c.cart.PoseOBJResident) == 0 {
		return
	}
	words, base := c.cart.PoseOBJResident, c.cart.PoseOBJResidentBase
	if len(words)%16 != 0 || !mode0PsprWindowAvailable(base, len(words)/16) {
		return
	}
	hw.CopyWordsToVRAM32DMA(words, hw.OBJVRAM+uintptr(base*64), len(words))
	c.mode0UploadStats.ColdTileBytes += uint32(len(words) * 4)
	c.mode0PoseResidentReady = true
}

func (c *Context) mode0ResidentPSpr(id, x, y int, flipX, flipY bool) {
	if !c.mode0PoseResidentReady || uint(id) >= uint(len(c.cart.PoseOBJIndex)) {
		return
	}
	plan := c.cart.PoseOBJIndex[id]
	start, end := int(plan.Start), int(plan.Start)+int(plan.Count)
	if start < 0 || end < start || end > len(c.cart.PoseOBJParts) {
		return
	}
	c.ensureDrawState()
	for _, object := range c.cart.PoseOBJParts[start:end] {
		tile := c.cart.PoseOBJResidentBase + int(object.TileOffset)
		if object.Palette > 0 {
			tile = c.cart.PoseOBJResidentBase*2 + int(object.TileOffset)
		}
		c.emitMode0PoseObject(tile, x, y, object, flipX, flipY)
	}
}

func (c *Context) emitMode0PoseObject(tile, x, y int, object PoseOBJ, flipX, flipY bool) {
	width, height := int(object.Width), int(object.Height)
	shape, size, valid := mode0PoseShape(width, height)
	limit := mode0PsprWindowLimit
	if object.Palette > 0 {
		limit *= 2
	}
	if !valid || tile < 0 || tile+width*height/64 > limit {
		return
	}
	ox, oy := int(object.X), int(object.Y)
	partFlipX, partFlipY := object.Flags&PoseFlipX != 0, object.Flags&PoseFlipY != 0
	if flipX {
		ox = -ox - width
		partFlipX = !partFlipX
	}
	if flipY {
		oy = -oy - height
		partFlipY = !partFlipY
	}
	objectX, objectY := x-c.camX+ox, y-c.camY+oy
	if !c.mode0ObjectIntersectsClip(objectX, objectY, width, height) || c.mode0ObjectCount >= mode0ObjectLimit {
		return
	}
	attr0 := uint16(objectY&0xff) | 0x2000 | shape<<14
	attr1 := uint16(objectX&0x1ff) | size<<14
	if partFlipX {
		attr1 |= 1 << 12
	}
	if partFlipY {
		attr1 |= 1 << 13
	}
	attr2 := uint16(tile*2) | c.mode0ObjectPriorityBits()
	if object.Palette > 0 {
		attr0 &^= 0x2000
		attr2 = uint16(tile) | uint16(object.Palette-1)<<12 | c.mode0ObjectPriorityBits()
	}
	mode0PutOAMEntry(&c.mode0OAM, mode0ObjectLimit-1-c.mode0ObjectCount, attr0, attr1, attr2)
	c.mode0ObjectCount++
	c.mode0OAMDirty = true
}

func (c *Context) fillMode0PsprUniqueOrder(base int, parts []PosePart) int {
	n := 0
	for _, part := range parts {
		if c.mode0PsprLocalIndex(base, n, part.Tile) >= 0 {
			continue
		}
		if !mode0PsprWindowAvailable(base+n, 1) {
			return -1
		}
		c.mode0PsprTiles[base+n] = part.Tile
		n++
	}
	return n
}

func (c *Context) mode0PsprLocalIndex(base, n int, tile uint16) int {
	for i := 0; i < n; i++ {
		if c.mode0PsprTiles[base+i] == tile {
			return i
		}
	}
	return -1
}

// Prepare scattered ROM cells and mutable sheet rows during active display.
// VBlank then needs one contiguous DMA, independent of the source tile IDs.
func (c *Context) prepareMode0PsprStreams() {
	if c == nil {
		return
	}
	words := c.mode0PoseStageWords()
	if len(words) == 0 {
		return
	}
	c.mode0PsprFallback = false
	for site := 0; site < c.mode0PsprSiteCount; site++ {
		pending := &c.mode0PsprPending[site]
		if !pending.dirty {
			continue
		}
		if !pending.prepared {
			stageStart := (pending.base - c.cart.PoseOBJStageBase) * 16
			if stageStart < 0 || stageStart+pending.n*16 > len(words) {
				c.mode0PsprFallback = true
				continue
			}
			for i := 0; i < pending.n; i++ {
				dest := pending.base + i
				if !mode0PsprWindowAvailable(dest, 1) {
					continue
				}
				cell := c.poseBankCell(int(c.mode0PsprTiles[dest]))
				stage := unsafe.Slice((*byte)(unsafe.Pointer(&words[stageStart+i*16])), 64)
				if len(cell) >= 64 {
					copy(stage, cell[:64])
				} else {
					clear(stage)
				}
			}
			pending.prepared = true
		}
		c.queueMode0PsprUpload(pending)
		// The cache now names staged pixels. The global DMA span persists until
		// presentation, including across a repeated prepare or skipped present.
		c.mode0PsprLastID[site] = pending.id
		pending.dirty = false
	}
}

func (c *Context) presentMode0PsprStreams() uint32 {
	if c == nil {
		return 0
	}
	var uploaded uint32
	first, last := c.mode0PsprDMAFirst, c.mode0PsprDMALast
	if last > first {
		words := c.mode0PoseStageWords()
		start := (first - c.cart.PoseOBJStageBase) * 16
		end := (last - c.cart.PoseOBJStageBase) * 16
		hw.CopyWordsToVRAM32DMA(words[start:end], hw.OBJVRAM+uintptr(first*64), end-start)
		uploaded = uint32((last - first) * 64)
		c.mode0PsprDMAFirst, c.mode0PsprDMALast = 0, 0
	}
	if !c.mode0PsprFallback {
		return uploaded
	}
	// Direct runtime callers that omit preparation retain exact behavior. The
	// generated frame loop always stages its work before entering VBlank.
	for site := 0; site < c.mode0PsprSiteCount; site++ {
		pending := &c.mode0PsprPending[site]
		if !pending.dirty {
			continue
		}
		for i := 0; i < pending.n; i++ {
			dest := pending.base + i
			if !mode0PsprWindowAvailable(dest, 1) {
				continue
			}
			cell := c.poseBankCell(int(c.mode0PsprTiles[dest]))
			if len(cell) < 64 {
				continue
			}
			hw.CopyToVRAM32DMA(cell[:64], hw.OBJVRAM+uintptr(dest*64), 16)
			uploaded += 64
		}
		c.mode0PsprLastID[site] = pending.id
		pending.dirty = false
	}
	c.mode0PsprFallback = false
	return uploaded
}

func (c *Context) poseBankCell(tile int) []byte {
	if c == nil || c.cart == nil {
		return nil
	}
	if c.cart.ObjSpriteN > 0 {
		if tile < 0 || tile >= c.cart.ObjSpriteN || tile >= len(c.cart.ObjSheetIndex) {
			return emptySpriteCell[:]
		}
		slot := c.cart.ObjSheetIndex[tile]
		if slot == 0 {
			return emptySpriteCell[:]
		}
		start := (int(slot) - 1) * 64
		if start < 0 || start+64 > len(c.cart.ObjSheet) {
			return emptySpriteCell[:]
		}
		return c.cart.ObjSheet[start : start+64]
	}
	if tile < 0 || tile >= c.cart.SpriteN {
		return emptySpriteCell[:]
	}
	if tile < len(c.cart.SheetIndex) {
		slot := c.cart.SheetIndex[tile]
		if slot == 0 {
			return emptySpriteCell[:]
		}
		start := (int(slot) - 1) * 64
		if start < 0 || start+64 > len(c.cart.Sheet) {
			return emptySpriteCell[:]
		}
		return c.cart.Sheet[start : start+64]
	}
	if c.stageMode0Tile(tile) {
		return c.mode0TileStage[:]
	}
	return emptySpriteCell[:]
}

func (c *Context) mode0PSprPart(id, x, y int, flipX, flipY bool) {}
