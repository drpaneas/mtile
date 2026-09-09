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
	// Optional immutable-sheet metadata indexed by compact slot, not sprite
	// ID. Zero means mixed/unknown; 1..256 encode a solid source color + 1.
	// Init validates it against the source pixels before rendering trusts it.
	SheetSolid []uint16
	MapChunks  []MapChunk
	MapEdits   []MapEdit
	// Optional Mode 4 raster storage. The generator supplies it only when a
	// small, fixed, single-plane map and the target's EWRAM budget permit it.
	MapRasterPixels []byte
	MapRasterSpans  []MapRasterSpan
	// Optional fixed-map cache, partitioned by logical background plane.
	// Each span stores only nonzero source pixels, before palette remapping.
	// Uniform spans store a color directly; only mixed runs need pixel storage.
	MapPackedPixels []byte
	MapPackedSpans  []MapPackedSpan
	MapPackedLayers [4]MapPackedLayer
}

// MapPackedSpan describes an opaque source run. X/Y retain map coordinates;
// the low 15 Length bits hold its length. With the high Length bit clear,
// Offset addresses packed pixels. With it set, Offset stores a source color
// and the run has no pixel payload.
type MapPackedSpan struct {
	X, Y, Length, Offset uint16
}

// MapPackedLayer bounds one plane's independent cache storage. Ends are
// exclusive. An all-zero descriptor disables caching for that plane.
type MapPackedLayer struct {
	PixelStart, PixelEnd, SpanStart, SpanEnd uint16
}

// MapRasterSpan is one opaque horizontal run in a cached small map.
// The linear offset makes an unshifted full-screen pass a compact copy list.
// Y avoids division when clipping or translating that list for camera motion.
type MapRasterSpan struct {
	Offset uint16
	Length uint8
	Y      uint8
}

const MapRasterMaxSpans = 2048

// MapRasterHeapReserve lets generated constant memory accounting include both
// Lua semispaces, even in heap-elided builds where that is conservative.
const MapRasterHeapReserve = 2 * heapSize

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
	// Keep both operands: mapStorageStreaming differs between generated build tags.
	if mapStorageFixed && image.MapStreaming != mapStorageStreaming { //nolint:staticcheck // S1002: simplifying against the host constant breaks streamed builds.
		return false
	}
	readOnlySheet := sheetStorageReadOnly || image.SheetReadOnly || ram.SheetReadOnly || len(ram.Sheet) == 0
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
		if len(ram.SheetSolid) != 0 {
			if len(ram.SheetSolid) != len(image.Sheet)/64 {
				return false
			}
			for slot, encoded := range ram.SheetSolid {
				if encoded == 0 {
					continue
				}
				if encoded > 256 {
					return false
				}
				for _, pixel := range image.Sheet[slot*64 : (slot+1)*64] {
					if pixel != byte(encoded-1) {
						return false
					}
				}
			}
		}
	} else if len(image.Sheet) != sheetN || len(ram.Sheet) != sheetN ||
		len(image.SheetIndex) != 0 || len(ram.SheetIndex) != 0 || len(ram.SheetSolid) != 0 {
		return false
	}
	if !packedMapStorageValid(image, ram, readOnlySheet, uint8(imageMask)) {
		return false
	}
	if !mapStorageStreaming && (mapStorageFixed || !image.MapStreaming) {
		return len(ram.Map) == mapN && len(ram.MapChunks) == 0 && len(ram.MapEdits) == 0
	}
	return len(ram.Map) == MapCacheCells &&
		len(ram.MapChunks) == MapCacheSlots &&
		len(ram.MapEdits) == MapEditCapacity
}

func packedMapStorageValid(image *AssetImage, ram *AssetMemory, readOnlySheet bool, planeMask uint8) bool {
	if len(ram.MapPackedPixels) == 0 && len(ram.MapPackedSpans) == 0 {
		return ram.MapPackedLayers == [4]MapPackedLayer{}
	}
	if !readOnlySheet || image.MapStreaming || len(ram.MapPackedPixels) == 0 ||
		len(ram.MapPackedPixels) >= 65536 || len(ram.MapPackedSpans) == 0 || len(ram.MapPackedSpans) >= 65536 {
		return false
	}
	enabled := false
	for bg, layer := range ram.MapPackedLayers {
		if layer == (MapPackedLayer{}) {
			continue
		}
		if planeMask&(1<<uint(bg)) == 0 || layer.PixelStart >= layer.PixelEnd ||
			layer.SpanStart >= layer.SpanEnd || layer.PixelStart&3 != 0 || layer.PixelEnd&3 != 0 ||
			int(layer.PixelEnd) > len(ram.MapPackedPixels) || int(layer.SpanEnd) > len(ram.MapPackedSpans) {
			return false
		}
		for _, previous := range ram.MapPackedLayers[:bg] {
			if previous == (MapPackedLayer{}) {
				continue
			}
			if layer.PixelStart < previous.PixelEnd && previous.PixelStart < layer.PixelEnd ||
				layer.SpanStart < previous.SpanEnd && previous.SpanStart < layer.SpanEnd {
				return false
			}
		}
		enabled = true
	}
	return enabled
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

//go:section .rodata
var mapNibblePopulation = [16]uint8{0, 1, 1, 2, 1, 2, 2, 3, 1, 2, 2, 3, 2, 3, 3, 4}

// mapPlaneSlots converts a logical BG ID to its dense storage-plane index.
// Keeping the complete four-plane mapping in ROM avoids a variable-length
// population-count loop in collision-heavy mget() workloads without spending
// any per-Context EWRAM.
//
//go:section .rodata
var mapPlaneSlots = [16][4]int8{
	{-1, -1, -1, -1},
	{0, -1, -1, -1},
	{-1, 0, -1, -1},
	{0, 1, -1, -1},
	{-1, -1, 0, -1},
	{0, -1, 1, -1},
	{-1, 0, 1, -1},
	{0, 1, 2, -1},
	{-1, -1, -1, 0},
	{0, -1, -1, 1},
	{-1, 0, -1, 1},
	{0, 1, -1, 2},
	{-1, -1, 0, 1},
	{0, -1, 1, 2},
	{-1, 0, 1, 2},
	{0, 1, 2, 3},
}

func mapMaskPopulation(mask uint8) int {
	return int(mapNibblePopulation[mask&0x0f] + mapNibblePopulation[mask>>4])
}

func mapPlaneSlot(mask uint8, layer, layers int) (int, bool) {
	if layer < 0 || layer >= layers || layer >= 4 || mask&^uint8(0x0f) != 0 {
		return 0, false
	}
	slot := mapPlaneSlots[mask][layer]
	return int(slot), slot >= 0
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
	if sheetStorageReadOnly || c.cart.SheetReadOnly {
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
	if c == nil || c.cart == nil || sheetStorageReadOnly || c.cart.SheetReadOnly || color < 0 || color > 255 ||
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

// MgetLayer reads one logical authored background plane. Invalid or
// non-resident layers follow Mget's out-of-range behavior and return 0.
// Mget remains the compatibility-fast BG0 entry point used by legacy carts.
func (c *Context) MgetLayer(layer, x, y int) uint16 {
	return c.mapGetLayer(layer, x, y)
}

// MgetCompact is the generated-code fast path for a compile-time logical
// layer and its compile-time dense storage slot. Hand-written/runtime callers
// fall back to the general path unless the selected map storage is fixed and
// compact. Invalid coordinates, layers, or absent slots still return zero.
func (c *Context) MgetCompact(layer, storageLayer, x, y int) uint16 {
	if !mapStorageFixed || mapStorageStreaming {
		return c.mapGetLayer(layer, x, y)
	}
	if c == nil || c.cart == nil || layer < 0 || layer >= c.cart.MapLayers ||
		storageLayer < 0 || storageLayer >= mapMaskPopulation(c.cart.MapPlaneMask) ||
		x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return 0
	}
	return c.cart.Map[storageLayer*c.cart.MapW*c.cart.MapH+y*c.cart.MapW+x]
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
	if mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming {
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
	if mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming {
		if c.mapStreamingSet(x, y, uint16(id)) {
			c.mapMutated(x, y, uint16(id))
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
	c.mapMutated(x, y, uint16(id))
}

// MsetLayer updates one logical authored background plane. The build planner
// retains every statically or dynamically targeted plane, so a valid layer
// that starts empty has mutable storage in generated carts. Invalid layers,
// coordinates, and sprite IDs are ignored exactly like legacy Mset inputs.
func (c *Context) MsetLayer(layer, x, y, id int) {
	if layer == 0 {
		c.Mset(x, y, id)
		return
	}
	if c == nil || c.cart == nil || layer < 0 || layer >= c.cart.MapLayers ||
		id < 0 || id >= c.cart.SpriteN ||
		x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return
	}
	if mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming {
		if c.mapStreamingSetLayer(layer, x, y, uint16(id)) {
			c.mapLayerMutated(layer, x, y, uint16(id))
		}
		return
	}
	storageLayer, ok := mapPlaneSlot(c.cart.MapPlaneMask, layer, c.cart.MapLayers)
	if !ok {
		return
	}
	index := storageLayer*c.cart.MapW*c.cart.MapH + y*c.cart.MapW + x
	value := uint16(id)
	if c.cart.Map[index] == value {
		return
	}
	c.cart.Map[index] = value
	c.mapLayerMutated(layer, x, y, uint16(id))
}

// MsetCompact is MgetCompact's mutation counterpart. The logical layer is
// retained so Mode 0 invalidates/patches the correct visible BG even though
// dense storage lookup itself has been removed.
func (c *Context) MsetCompact(layer, storageLayer, x, y, id int) {
	if !mapStorageFixed || mapStorageStreaming {
		c.MsetLayer(layer, x, y, id)
		return
	}
	if c == nil || c.cart == nil || layer < 0 || layer >= c.cart.MapLayers ||
		storageLayer < 0 || storageLayer >= mapMaskPopulation(c.cart.MapPlaneMask) ||
		id < 0 || id >= c.cart.SpriteN || x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return
	}
	index := storageLayer*c.cart.MapW*c.cart.MapH + y*c.cart.MapW + x
	value := uint16(id)
	if c.cart.Map[index] == value {
		return
	}
	c.cart.Map[index] = value
	c.mapLayerMutated(layer, x, y, uint16(id))
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
