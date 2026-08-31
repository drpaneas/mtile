package runtime

// AssetImage is the ROM-side view of a cartridge: dimensions plus slice
// headers pointing at generated ROM arrays.
type AssetImage struct {
	SheetW, SheetH         int
	SpriteCols, SpriteRows int
	MapW, MapH             int
	MapLayers              int
	MapPlaneMask           uint8
	SpriteN                int
	MapStreaming           bool
	// SheetReadOnly avoids a full mutable copy when generated Lua never calls
	// sset. SheetIndex maps logical sprite IDs to compact 64-byte cells; zero
	// means an implicit all-zero cell, otherwise the value is slot+1.
	SheetReadOnly bool
	Sheet         []byte
	SheetIndex    []uint16
	Map           []uint16
	Flags         []byte
}

// AssetMemory is the RAM copy Init writes into. Generated main points the
// slices at EWRAM arrays before calling Init.
type AssetMemory struct {
	SheetW, SheetH         int
	SpriteCols, SpriteRows int
	MapW, MapH             int
	MapLayers              int
	MapPlaneMask           uint8
	SpriteN                int
	MapStreaming           bool
	SheetReadOnly          bool
	Sheet                  []byte
	SheetIndex             []uint16
	Map                    []uint16
	Flags                  []byte
	MapChunks              []MapChunk
	MapEdits               []MapEdit
}

func mulFitInt(a, b int) (int, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	if a == 0 || b == 0 {
		return 0, true
	}
	if uint64(a) > ^uint64(0)/uint64(b) {
		return 0, false
	}
	p := uint64(a) * uint64(b)
	maxInt := int(^uint(0) >> 1)
	if p > uint64(maxInt) {
		return 0, false
	}
	return int(p), true
}

func assetLensExact(image *AssetImage, ram *AssetMemory) bool {
	if image == nil || ram == nil {
		return false
	}
	if image.SheetW < 1 || image.SheetH < 1 || image.MapW < 0 || image.MapH < 0 {
		return false
	}
	if image.SheetW != ram.SheetW || image.SheetH != ram.SheetH ||
		image.MapW != ram.MapW || image.MapH != ram.MapH {
		return false
	}
	imageLayers := normalizedMapLayers(image.MapLayers)
	ramLayers := normalizedMapLayers(ram.MapLayers)
	if imageLayers < 1 || imageLayers != ramLayers {
		return false
	}
	cols := image.SheetW / 8
	rows := image.SheetH / 8
	if image.SpriteCols != cols || ram.SpriteCols != cols ||
		image.SpriteRows != rows || ram.SpriteRows != rows {
		return false
	}
	if image.MapStreaming != ram.MapStreaming {
		return false
	}
	readOnlySheet := image.SheetReadOnly || ram.SheetReadOnly || len(ram.Sheet) == 0
	sheetN, ok := mulFitInt(image.SheetW, image.SheetH)
	if !ok {
		return false
	}
	mapPlaneN, ok := mulFitInt(image.MapW, image.MapH)
	if !ok {
		return false
	}
	imageMask := normalizedMapPlaneMask(image.MapPlaneMask, imageLayers, len(image.Map), mapPlaneN)
	if imageMask < 0 {
		return false
	}
	if ram.MapPlaneMask != 0 && ram.MapPlaneMask != uint8(imageMask) {
		return false
	}
	mapN, ok := mulFitInt(mapPlaneN, mapMaskPopulation(uint8(imageMask)))
	if !ok || uint64(mapN) > uint64(^uint32(0)) {
		return false
	}
	spriteN, ok := mulFitInt(cols, rows)
	if !ok || spriteN > 65536 || image.SpriteN != spriteN || ram.SpriteN != spriteN {
		return false
	}
	if len(image.Map) != mapN || len(image.Flags) != spriteN || len(ram.Flags) != spriteN {
		return false
	}
	if readOnlySheet {
		if len(image.Sheet)%64 != 0 || len(ram.Sheet) != len(image.Sheet) ||
			len(image.SheetIndex) != spriteN || len(ram.SheetIndex) != spriteN {
			return false
		}
		for _, slot := range image.SheetIndex {
			if slot != 0 && int(slot)*64 > len(image.Sheet) {
				return false
			}
		}
	} else if len(image.Sheet) != sheetN || len(ram.Sheet) != sheetN ||
		len(image.SheetIndex) != 0 || len(ram.SheetIndex) != 0 {
		return false
	}
	if !image.MapStreaming {
		return len(ram.Map) == mapN && len(ram.MapChunks) == 0 && len(ram.MapEdits) == 0
	}
	return len(ram.Map) == MapCacheCells &&
		len(ram.MapChunks) == MapCacheSlots &&
		len(ram.MapEdits) == MapEditCapacity
}

func normalizedMapLayers(layers int) int {
	if layers == 0 {
		return 1
	}
	if layers < 1 || layers > 4 {
		return -1
	}
	return layers
}

func normalizedMapPlaneMask(mask uint8, layers, mapLen, planeCells int) int {
	valid := uint8((1 << uint(layers)) - 1)
	if mask&^valid != 0 {
		return -1
	}
	if mask != 0 {
		return int(mask)
	}
	// Compatibility with assets generated before sparse plane metadata: a
	// full logical-plane payload implied that every plane was stored.
	if planeCells > 0 && mapLen == planeCells*layers {
		return int(valid)
	}
	if mapLen != 0 {
		return -1
	}
	return 0
}

func mapMaskPopulation(mask uint8) int {
	n := 0
	for mask != 0 {
		n += int(mask & 1)
		mask >>= 1
	}
	return n
}

func mapPlaneSlot(mask uint8, layer, layers int) (int, bool) {
	if layer < 0 || layer >= layers || mask&(1<<uint(layer)) == 0 {
		return 0, false
	}
	return mapMaskPopulation(mask & uint8((1<<uint(layer))-1)), true
}

func mapLogicalPlane(mask uint8, slot, layers int) (int, bool) {
	if slot < 0 {
		return 0, false
	}
	seen := 0
	for layer := 0; layer < layers; layer++ {
		if mask&(1<<uint(layer)) == 0 {
			continue
		}
		if seen == slot {
			return layer, true
		}
		seen++
	}
	return 0, false
}

// Sget/Sset access the full native 8bpp sheet. They intentionally do not
// alias map memory: sheet pixels and map cells are independent buffers.
func (c *Context) Sget(x, y int) byte {
	if c == nil || c.cart == nil || x < 0 || y < 0 || x >= c.cart.SheetW || y >= c.cart.SheetH {
		return 0
	}
	if c.cart.SheetReadOnly {
		cell := (y/8)*c.cart.SpriteCols + x/8
		if cell < 0 || cell >= len(c.cart.SheetIndex) {
			return 0
		}
		slot := c.cart.SheetIndex[cell]
		if slot == 0 {
			return 0
		}
		return c.cart.Sheet[(int(slot)-1)*64+(y&7)*8+(x&7)]
	}
	return c.cart.Sheet[y*c.cart.SheetW+x]
}

func (c *Context) Sset(x, y, color int) {
	if c == nil || c.cart == nil || c.cart.SheetReadOnly || color < 0 || color > 255 ||
		x < 0 || y < 0 || x >= c.cart.SheetW || y >= c.cart.SheetH {
		return
	}
	index := y*c.cart.SheetW + x
	value := byte(color)
	if c.cart.Sheet[index] == value {
		return
	}
	c.cart.Sheet[index] = value
	c.sheetMutated(x, y)
}

func (c *Context) Mget(x, y int) uint16 {
	return c.mapGetLayer(0, x, y)
}

func (c *Context) mapGetLayer(layer, x, y int) uint16 {
	if c == nil || c.cart == nil || x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return 0
	}
	if layer < 0 || layer >= c.cart.MapLayers {
		return 0
	}
	storageLayer, ok := mapPlaneSlot(c.cart.MapPlaneMask, layer, c.cart.MapLayers)
	if !ok {
		return 0
	}
	if c.cart.MapStreaming {
		return c.mapStreamingGetLayer(layer, x, y)
	}
	return c.cart.Map[storageLayer*c.cart.MapW*c.cart.MapH+y*c.cart.MapW+x]
}

// Mset ignores IDs that cannot name a complete sprite cell. This keeps a bad
// map cell from becoming an out-of-bounds sheet access later.
func (c *Context) Mset(x, y, id int) {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN ||
		x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return
	}
	if c.cart.MapStreaming {
		if c.mapStreamingSet(x, y, uint16(id)) {
			c.mapMutated(x, y)
		}
		return
	}
	if c.cart.MapPlaneMask&1 == 0 {
		return
	}
	index := y*c.cart.MapW + x
	value := uint16(id)
	if c.cart.Map[index] == value {
		return
	}
	c.cart.Map[index] = value
	c.mapMutated(x, y)
}

// Fget reports one flag bit for sprite id. Out-of-range id or bit is false.
func (c *Context) Fget(id, bit int) bool {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN || bit < 0 || bit > 7 {
		return false
	}
	return c.cart.Flags[id]&(1<<uint(bit)) != 0
}

// FgetByte returns sprite n's whole flag byte (the no-bit fget(n) form). Out-of-range
// id is 0.
func (c *Context) FgetByte(id int) int {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN {
		return 0
	}
	return int(c.cart.Flags[id])
}

// Fset writes one flag bit for sprite id. Out-of-range id or bit is a no-op.
func (c *Context) Fset(id, bit int, val bool) {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN || bit < 0 || bit > 7 {
		return
	}
	mask := byte(1 << uint(bit))
	before := c.cart.Flags[id]
	if val {
		c.cart.Flags[id] |= mask
	} else {
		c.cart.Flags[id] &^= mask
	}
	if c.cart.Flags[id] != before {
		c.flagsMutated(id)
	}
}

// FsetByte writes sprite n's whole flag byte (the no-bit fset(n, v) form). Out-of-range
// id is a no-op.
func (c *Context) FsetByte(id, flags int) {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN {
		return
	}
	value := byte(flags)
	if c.cart.Flags[id] == value {
		return
	}
	c.cart.Flags[id] = value
	c.flagsMutated(id)
}
