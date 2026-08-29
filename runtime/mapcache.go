package runtime

// Large maps remain in cartridge ROM and are read through a small, fixed
// working set. Four 32x32 chunks cover a 64x64-tile neighborhood, which is
// larger than the native 30x20 screen and leaves room for camera movement.
const (
	MapChunkWidth  = 32
	MapChunkHeight = 32
	MapChunkCells  = MapChunkWidth * MapChunkHeight
	MapCacheSlots  = 4
	MapCacheCells  = MapChunkCells * MapCacheSlots
	// Maps at or below this size remain a complete mutable EWRAM map. This
	// deliberately covers the original Mtile 128x64 map (8192 cells), so
	// its mset() behavior is not reduced by the sparse streaming edit budget.
	MapStreamingThresholdCells = 16384
	MapEditCapacity            = 4096
	// MapEditProbeLimit bounds the number of slots a lookup or insertion
	// may inspect. A new edit that cannot be found or placed within this
	// budget fails closed as MapEditFull; existing edits are always stored
	// within the same budget and remain updatable.
	MapEditProbeLimit = 100
	// MapEditFillLimit caps how many distinct cells the sparse overlay may
	// hold, well below the physical MapEditCapacity slot count. The overlay
	// is open-addressed with linear probing, and every Mget/Mset - plus every
	// visible cell during map() - pays for one probe sequence. Measured
	// against sequential, common-map-width strides, and random insertion
	// order, capping the *count* of live entries at 75% of MapEditCapacity
	// keeps the observed worst probe length below MapEditProbeLimit. At the
	// natural load, sequential/strided access measured single digits and
	// uniform-random access measured below 100. The probe budget makes that
	// performance property hard: a collision pattern cannot create a longer
	// chain, but it may reduce the usable edit count below MapEditFillLimit
	// because the insertion fails closed. See the probe budget tests in
	// mapcache_test.go.
	MapEditFillLimit = MapEditCapacity * 3 / 4
)

// MapChunk identifies one cache slot. X and Y are chunk coordinates, not
// pixel or tile coordinates. Valid is separate from the coordinates so chunk
// (0,0) is represented without a sentinel value.
type MapChunk struct {
	X     uint32
	Y     uint32
	Stamp uint32
	Valid uint16
	_     uint16
}

// MapEdit is one sparse write overlay entry. Index is the row-major map cell
// index, allowing the entry to stay eight bytes while supporting maps whose
// dimensions are independently large. Used distinguishes an edit to sprite
// ID 0 from an unused slot.
type MapEdit struct {
	Index uint32
	Value uint16
	Used  uint16
}

type MapError uint8

const (
	MapOK MapError = iota
	MapEditFull
	MapCacheInvalid
)

func (c *Context) LastMapError() MapError {
	if c == nil {
		return MapOK
	}
	return c.mapError
}

func (c *Context) ClearMapError() {
	if c != nil {
		c.mapError = MapOK
	}
}

func (c *Context) mapStreamingGet(x, y int) uint16 {
	if c == nil || c.cart == nil || !c.cart.MapStreaming || x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return 0
	}
	index, ok := mapCellIndex(x, y, c.cart.MapW)
	if !ok {
		return 0
	}
	if value, found := c.mapEditLookup(index); found {
		if slot := c.mapChunkSlot(x/MapChunkWidth, y/MapChunkHeight); slot >= 0 {
			c.touchMapChunk(slot)
		}
		return value
	}
	slot := c.mapChunkSlot(x/MapChunkWidth, y/MapChunkHeight)
	if slot < 0 {
		slot = c.loadMapChunk(x/MapChunkWidth, y/MapChunkHeight)
		if slot < 0 {
			return 0
		}
	} else {
		c.touchMapChunk(slot)
	}
	localX := x % MapChunkWidth
	localY := y % MapChunkHeight
	return c.cart.Map[slot*MapChunkCells+localY*MapChunkWidth+localX]
}

// mapStreamingSet reports whether the authoritative map value changed. The
// renderer uses that signal to invalidate a materialized hardware tilemap
// without treating idempotent mset() calls as per-frame upload work.
func (c *Context) mapStreamingSet(x, y int, value uint16) bool {
	if c == nil || c.cart == nil || !c.cart.MapStreaming || x < 0 || y < 0 || x >= c.cart.MapW || y >= c.cart.MapH {
		return false
	}
	index, ok := mapCellIndex(x, y, c.cart.MapW)
	if !ok {
		c.mapError = MapCacheInvalid
		return false
	}
	slot, editSlot := c.mapEditLookupSlot(index)
	if editSlot < 0 {
		if c.mapEditCount >= MapEditFillLimit {
			// No new edit can be admitted. Avoid loading an uncached chunk just
			// to discover that the write must fail; a missing overlay entry
			// means the ROM value is authoritative.
			if uint64(index) >= uint64(len(c.mapROM)) {
				c.mapError = MapCacheInvalid
				return false
			}
			if c.mapROM[int(index)] != value {
				c.mapError = MapEditFull
			}
			return false
		}
		if slot < 0 {
			slot = c.loadMapChunk(x/MapChunkWidth, y/MapChunkHeight)
			if slot < 0 {
				return false
			}
		}
		localX := x % MapChunkWidth
		localY := y % MapChunkHeight
		cacheIndex := slot*MapChunkCells + localY*MapChunkWidth + localX
		if c.cart.Map[cacheIndex] == value {
			// A write that already matches ROM (or the current cached value)
			// needs no overlay entry and cannot consume the edit budget.
			return false
		}
		editSlot = c.mapEditInsert(index, value)
		if editSlot < 0 {
			c.mapError = MapEditFull
			return false
		}
	} else {
		if c.cart.MapEdits[editSlot].Value == value {
			return false
		}
		c.cart.MapEdits[editSlot].Value = value
	}
	if slot >= 0 {
		c.touchMapChunk(slot)
		localX := x % MapChunkWidth
		localY := y % MapChunkHeight
		c.cart.Map[slot*MapChunkCells+localY*MapChunkWidth+localX] = value
	}
	return true
}

func mapCellIndex(x, y, width int) (uint32, bool) {
	if x < 0 || y < 0 || width <= 0 || x >= width {
		return 0, false
	}
	index := uint64(y)*uint64(width) + uint64(x)
	if index > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(index), true
}

// mapEditLookup reads only the sparse overlay. It deliberately does not also
// resolve the map chunk containing index: chunk loading calls this for all
// 1,024 cells, and coupling an edit probe to a four-slot cache scan made a
// cold streamed-map load needlessly expensive. Callers that need both use
// mapEditLookupSlot instead.
func (c *Context) mapEditLookup(index uint32) (uint16, bool) {
	if len(c.cart.MapEdits) != MapEditCapacity {
		c.mapError = MapCacheInvalid
		return 0, false
	}
	// Until the first Mset creates an overlay entry, ROM is authoritative.
	// This also keeps a scrolling game from doing 1,024 empty hash probes
	// whenever it crosses a 32x32 chunk boundary.
	if c.mapEditCount == 0 {
		return 0, false
	}
	start := mapEditHash(index)
	for probe := 0; probe < MapEditProbeLimit; probe++ {
		slot := (start + probe) & (MapEditCapacity - 1)
		edit := c.cart.MapEdits[slot]
		if edit.Used == 0 {
			return 0, false
		}
		if edit.Index == index {
			return edit.Value, true
		}
	}
	return 0, false
}

// mapEditLookupSlot returns both the cache slot containing the coordinate and
// the sparse-edit slot. The first result lets Mset update an already loaded
// chunk without loading a new one; the second is -1 when no edit exists.
func (c *Context) mapEditLookupSlot(index uint32) (int, int) {
	if len(c.cart.MapEdits) != MapEditCapacity {
		c.mapError = MapCacheInvalid
		return -1, -1
	}
	start := mapEditHash(index)
	for probe := 0; probe < MapEditProbeLimit; probe++ {
		slot := (start + probe) & (MapEditCapacity - 1)
		edit := c.cart.MapEdits[slot]
		if edit.Used == 0 {
			return c.mapChunkSlotByIndex(index), -1
		}
		if edit.Index == index {
			return c.mapChunkSlotByIndex(index), slot
		}
	}
	return c.mapChunkSlotByIndex(index), -1
}

// mapEditInsert adds a new overlay entry for an index that mapEditLookupSlot
// has already confirmed is not present. It fails closed at MapEditFillLimit
// distinct entries - leaving MapEditCapacity-MapEditFillLimit slots always
// empty - rather than at the physical MapEditCapacity. It also refuses a
// placement whose probe sequence exceeds MapEditProbeLimit, so no successful
// edit can make a later lookup exceed that budget. Updating an existing
// entry's value never goes through this path (mapStreamingSet writes
// c.cart.MapEdits[editSlot].Value directly), so it is never subject to either
// admission limit.
func (c *Context) mapEditInsert(index uint32, value uint16) int {
	if c.mapEditCount >= MapEditFillLimit {
		return -1
	}
	start := mapEditHash(index)
	for probe := 0; probe < MapEditProbeLimit; probe++ {
		slot := (start + probe) & (MapEditCapacity - 1)
		edit := &c.cart.MapEdits[slot]
		if edit.Used == 0 {
			edit.Index = index
			edit.Value = value
			edit.Used = 1
			c.mapEditCount++
			return slot
		}
	}
	return -1
}

func mapEditHash(index uint32) int {
	return int((index * 2654435761) >> 20)
}

func (c *Context) mapChunkSlotByIndex(index uint32) int {
	if c.cart.MapW <= 0 {
		return -1
	}
	x := int(index % uint32(c.cart.MapW))
	y := int(index / uint32(c.cart.MapW))
	return c.mapChunkSlot(x/MapChunkWidth, y/MapChunkHeight)
}

func (c *Context) mapChunkSlot(chunkX, chunkY int) int {
	if chunkX < 0 || chunkY < 0 || len(c.cart.MapChunks) != MapCacheSlots {
		return -1
	}
	for slot, chunk := range c.cart.MapChunks {
		if chunk.Valid != 0 && chunk.X == uint32(chunkX) && chunk.Y == uint32(chunkY) {
			return slot
		}
	}
	return -1
}

func (c *Context) touchMapChunk(slot int) {
	if slot < 0 || slot >= len(c.cart.MapChunks) {
		return
	}
	c.cart.MapChunks[slot].Stamp = c.nextMapStamp()
}

func (c *Context) loadMapChunk(chunkX, chunkY int) int {
	if len(c.mapROM) == 0 || len(c.cart.Map) != MapCacheCells ||
		len(c.cart.MapChunks) != MapCacheSlots || len(c.cart.MapEdits) != MapEditCapacity {
		c.mapError = MapCacheInvalid
		return -1
	}
	slot := -1
	for i, chunk := range c.cart.MapChunks {
		if chunk.Valid == 0 {
			slot = i
			break
		}
	}
	if slot < 0 {
		slot = 0
		for i := 1; i < MapCacheSlots; i++ {
			if c.cart.MapChunks[i].Stamp < c.cart.MapChunks[slot].Stamp {
				slot = i
			}
		}
	}
	stamp := c.nextMapStamp()
	meta := &c.cart.MapChunks[slot]
	meta.X, meta.Y, meta.Stamp, meta.Valid = uint32(chunkX), uint32(chunkY), stamp, 1
	base := slot * MapChunkCells
	for localY := 0; localY < MapChunkHeight; localY++ {
		mapY := chunkY*MapChunkHeight + localY
		for localX := 0; localX < MapChunkWidth; localX++ {
			mapX := chunkX*MapChunkWidth + localX
			value := uint16(0)
			if mapX >= 0 && mapY >= 0 && mapX < c.cart.MapW && mapY < c.cart.MapH {
				index, _ := mapCellIndex(mapX, mapY, c.cart.MapW)
				value = c.mapROM[int(index)]
				if edited, found := c.mapEditLookup(index); found {
					value = edited
				}
			}
			c.cart.Map[base+localY*MapChunkWidth+localX] = value
		}
	}
	return slot
}

func (c *Context) nextMapStamp() uint32 {
	c.mapStamp++
	if c.mapStamp == 0 {
		// Avoid an LRU inversion after 2^32 chunk loads. Keep the cache
		// contents, but rebase all stamps before handing out stamp 1.
		for i := range c.cart.MapChunks {
			c.cart.MapChunks[i].Stamp = 0
		}
		c.mapStamp = 1
	}
	return c.mapStamp
}
