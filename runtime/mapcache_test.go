package runtime

import (
	"math/rand"
	"strconv"
	"testing"
)

// maxEditProbeDisplacement measures, for every occupied overlay slot, how
// far it sits from mapEditHash(index)'s ideal starting bucket (mod
// MapEditCapacity). That displacement is exactly the number of probes a
// lookup for that index would need, since insertion and lookup share the
// same linear-probe order. Used to turn the empirical bounds documented on
// MapEditFillLimit into an enforced regression check.
func maxEditProbeDisplacement(edits []MapEdit) int {
	max := 0
	for slot, edit := range edits {
		if edit.Used == 0 {
			continue
		}
		start := mapEditHash(edit.Index)
		displacement := (slot - start) & (MapEditCapacity - 1)
		if probes := displacement + 1; probes > max {
			max = probes
		}
	}
	return max
}

// fillStreamingOverlay writes MapEditFillLimit distinct cells (indices)
// through the public Mset API and returns the number that actually landed
// before the caller-supplied loop ran out or MapEditFull fired.
func fillStreamingOverlay(t *testing.T, c *Context, mapW int, indices []int) int {
	t.Helper()
	filled := 0
	for _, index := range indices {
		if filled >= MapEditFillLimit {
			break
		}
		x, y := index%mapW, index/mapW
		c.ClearMapError()
		c.Mset(x, y, 1)
		if c.LastMapError() == MapEditFull {
			t.Fatalf("MapEditFull fired after only %d of %d entries", filled, MapEditFillLimit)
		}
		filled++
	}
	return filled
}

// TestStreamingMapEditProbeLengthRealisticPatterns pins the hard probe budget
// documented on MapEditProbeLimit: for access patterns an actual game would
// produce (painting cells in raster order, or along a fixed map-row/column
// stride), filling the overlay to its count limit keeps every successful
// probe sequence within the runtime budget.
func TestStreamingMapEditProbeLengthRealisticPatterns(t *testing.T) {
	const mapW, mapH = 4096, 4096
	patterns := map[string][]int{
		"sequential": nil,
	}
	seq := make([]int, MapEditFillLimit)
	for i := range seq {
		seq[i] = i
	}
	patterns["sequential"] = seq
	for _, stride := range []int{128, 192, 256, 320, 480, 1024, 4096} {
		s := make([]int, MapEditFillLimit)
		for i := range s {
			s[i] = i * stride
		}
		patterns["stride-"+strconv.Itoa(stride)] = s
	}
	rng := rand.New(rand.NewSource(42))
	seen := map[int]bool{}
	rnd := make([]int, 0, MapEditFillLimit)
	for len(rnd) < MapEditFillLimit {
		v := int(rng.Uint32() % uint32(mapW*mapH))
		if !seen[v] {
			seen[v] = true
			rnd = append(rnd, v)
		}
	}
	patterns["random"] = rnd

	for name, indices := range patterns {
		img, ram := streamingMapFixture(mapW, mapH, func(x, y int) uint16 { return 0 })
		var c Context
		if !c.Init(img, ram) {
			t.Fatalf("%s: streaming Init returned false", name)
		}
		filled := fillStreamingOverlay(t, &c, mapW, indices)
		if filled != MapEditFillLimit {
			t.Fatalf("%s: filled %d entries, want %d", name, filled, MapEditFillLimit)
		}
		if max := maxEditProbeDisplacement(ram.MapEdits); max > MapEditProbeLimit {
			t.Fatalf("%s: max probe length %d exceeds ceiling %d", name, max, MapEditProbeLimit)
		}
	}
}

func collidingMapEditIndices(count int) []int {
	const candidateSpace = 1 << 20
	const targetBucket = 0
	indices := make([]int, 0, count)
	for i := 0; i < candidateSpace && len(indices) < count; i++ {
		if mapEditHash(uint32(i)) == targetBucket {
			indices = append(indices, i)
		}
	}
	return indices
}

// TestStreamingMapEditProbeBudgetFailsClosed proves that a deliberately
// colliding key pattern cannot create an unbounded lookup. The first
// MapEditProbeLimit entries fit; the next one is rejected even though the
// count-based fill limit has not been reached.
func TestStreamingMapEditProbeBudgetFailsClosed(t *testing.T) {
	const mapW, mapH = 4096, 4096
	adversarial := collidingMapEditIndices(MapEditProbeLimit + 1)
	if len(adversarial) != MapEditProbeLimit+1 {
		t.Fatalf("candidate space produced %d colliding indices, want %d", len(adversarial), MapEditProbeLimit+1)
	}

	img, ram := streamingMapFixture(mapW, mapH, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	for i, index := range adversarial[:MapEditProbeLimit] {
		x, y := index%mapW, index/mapW
		c.ClearMapError()
		c.Mset(x, y, 1)
		if c.LastMapError() != MapOK {
			t.Fatalf("colliding edit %d failed with map error %d", i, c.LastMapError())
		}
	}
	if got := c.mapEditCount; got != MapEditProbeLimit {
		t.Fatalf("edit count = %d, want %d", got, MapEditProbeLimit)
	}
	max := maxEditProbeDisplacement(ram.MapEdits)
	if max > MapEditProbeLimit {
		t.Fatalf("max probe length %d exceeds budget %d", max, MapEditProbeLimit)
	}
	for _, index := range adversarial[:MapEditProbeLimit] {
		x, y := index%mapW, index/mapW
		if got := c.Mget(x, y); got != 1 {
			t.Fatalf("adversarial pattern lost or corrupted edit at index %d: got %d, want 1", index, got)
		}
	}
	newIndex := adversarial[MapEditProbeLimit]
	c.ClearMapError()
	c.Mset(newIndex%mapW, newIndex/mapW, 1)
	if c.LastMapError() != MapEditFull {
		t.Fatalf("colliding edit past probe budget = %d, want MapEditFull", c.LastMapError())
	}
	if got := c.Mget(newIndex%mapW, newIndex/mapW); got != 0 {
		t.Fatalf("rejected colliding edit changed map value to %d, want 0", got)
	}
}

func streamingMapFixture(width, height int, fill func(x, y int) uint16) (*AssetImage, *AssetMemory) {
	sheet := make([]byte, 16*8)
	for y := 0; y < 8; y++ {
		for x := 8; x < 16; x++ {
			sheet[y*16+x] = 7
		}
	}
	mapROM := make([]uint16, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			mapROM[y*width+x] = fill(x, y)
		}
	}
	img := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: width, MapH: height, SpriteN: 2, MapStreaming: true,
		Sheet: sheet, Map: mapROM, Flags: []byte{0, 1},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: width, MapH: height, SpriteN: 2, MapStreaming: true,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, MapCacheCells),
		Flags: make([]byte, 2), MapChunks: make([]MapChunk, MapCacheSlots),
		MapEdits: make([]MapEdit, MapEditCapacity),
	}
	return img, ram
}

func TestStreamingMapReadsAllCellsAcrossChunkSeams(t *testing.T) {
	img, ram := streamingMapFixture(96, 80, func(x, y int) uint16 {
		return uint16((x*7 + y*11) & 1)
	})
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	if c.mapROM == nil || len(c.mapROM) != 96*80 {
		t.Fatalf("map ROM was not retained: len=%d", len(c.mapROM))
	}
	for y := 0; y < img.MapH; y++ {
		for x := 0; x < img.MapW; x++ {
			want := img.Map[y*img.MapW+x]
			if got := c.Mget(x, y); got != want {
				t.Fatalf("Mget(%d,%d) = %d, want %d", x, y, got, want)
			}
		}
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("sequential read set map error %d", c.LastMapError())
	}
}

func TestStreamingMapMsetSurvivesEvictionAndPreservesZeroEdits(t *testing.T) {
	img, ram := streamingMapFixture(96, 80, func(x, y int) uint16 {
		return uint16((x + y) & 1)
	})
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	c.Mset(1, 1, 1)
	c.Mset(2, 2, 0) // Used must distinguish an edit to ID 0 from an empty slot.
	if c.Mget(1, 1) != 1 || c.Mget(2, 2) != 0 {
		t.Fatal("initial streaming edits were not readable")
	}

	// Touch six different chunks. The four-slot cache must evict at least two
	// of them, while the sparse edit table remains authoritative.
	for _, p := range [][2]int{{0, 0}, {32, 0}, {64, 0}, {0, 32}, {32, 32}, {64, 32}} {
		_ = c.Mget(p[0], p[1])
	}
	if c.Mget(1, 1) != 1 || c.Mget(2, 2) != 0 {
		t.Fatal("streaming edits were lost after cache eviction")
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("eviction test set map error %d", c.LastMapError())
	}
}

func TestStreamingMapEvictionUsesRecentChunkAccess(t *testing.T) {
	img, ram := streamingMapFixture(96, 64, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	for _, p := range [][2]int{{0, 0}, {32, 0}, {64, 0}, {0, 32}} {
		_ = c.Mget(p[0], p[1])
	}
	_ = c.Mget(0, 0) // Make chunk (0,0) the most recently used one.
	_ = c.Mget(32, 32)
	if c.mapChunkSlot(0, 0) < 0 {
		t.Fatal("recently used chunk (0,0) was evicted")
	}
	if c.mapChunkSlot(1, 0) >= 0 {
		t.Fatal("least-recently used chunk (1,0) was not evicted")
	}
}

func TestStreamingMapMapDrawsAcrossChunkSeam(t *testing.T) {
	img, ram := streamingMapFixture(96, 80, func(x, y int) uint16 { return 1 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	c.PaltColor(0, false)
	c.Clip(0, 0, 16, 8)
	// Map positions source cell cellX at screenX+cellX*8. Offset screenX
	// back by the source coordinate so cells 31 and 32 land at screen X=0
	// and X=8 while still exercising the streaming-cache chunk seam.
	const seamCellX = MapChunkWidth - 1
	c.Map(seamCellX, 0, -seamCellX*8, 0, 2, 1, 0)
	if got := c.Pget(1, 0); got != 7 {
		t.Fatalf("left seam cell pixel = %d, want 7", got)
	}
	if got := c.Pget(9, 0); got != 7 {
		t.Fatalf("right seam cell pixel = %d, want 7", got)
	}
}

func TestStreamingMapEditCapacityFailsClosed(t *testing.T) {
	img, ram := streamingMapFixture(128, 64, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	// The overlay fails closed at MapEditFillLimit distinct entries, not at
	// the larger physical MapEditCapacity slot count: see the MapEditFillLimit
	// doc comment in mapcache.go for why 1,024 slots are deliberately kept
	// empty as probe headroom.
	for i := 0; i < MapEditFillLimit; i++ {
		c.Mset(i%img.MapW, i/img.MapW, 1)
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("filling edit table to MapEditFillLimit set error %d", c.LastMapError())
	}
	c.Mset(0, MapEditFillLimit/img.MapW, 1)
	if c.LastMapError() != MapEditFull {
		t.Fatalf("overflow error = %d, want MapEditFull", c.LastMapError())
	}
	if got := c.Mget(0, 0); got != 1 {
		t.Fatalf("existing edit was corrupted after table overflow: %d", got)
	}
}

// TestStreamingMapEditUpdatesSurviveFillLimit pins the requirement that the
// fill limit only blocks *new* distinct cells from entering the overlay.
// Once at the limit, a game must still be able to change the value already
// stored for a cell it edited earlier - mapStreamingSet's editSlot>=0 branch
// writes straight into the existing slot and never calls mapEditInsert, so
// it is never subject to MapEditFillLimit.
func TestStreamingMapEditUpdatesSurviveFillLimit(t *testing.T) {
	img, ram := streamingMapFixture(128, 64, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	for i := 0; i < MapEditFillLimit; i++ {
		c.Mset(i%img.MapW, i/img.MapW, 1)
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("filling edit table to MapEditFillLimit set error %d", c.LastMapError())
	}
	// The table is now at the fill limit. A brand-new cell must still fail...
	c.Mset(1, MapEditFillLimit/img.MapW, 1)
	if c.LastMapError() != MapEditFull {
		t.Fatalf("new cell past the fill limit = %d, want MapEditFull", c.LastMapError())
	}
	c.ClearMapError()
	// ...but updating a cell edited earlier in the loop above must still
	// work. img.SpriteN is 2 (valid ids 0 and 1), so 0 is the "changed"
	// value distinct from the 1 every cell was filled with above.
	c.Mset(0, 0, 0)
	if c.LastMapError() != MapOK {
		t.Fatalf("updating an existing edit at the fill limit set error %d", c.LastMapError())
	}
	if got := c.Mget(0, 0); got != 0 {
		t.Fatalf("update at the fill limit did not take effect: got %d, want 0", got)
	}
}

func TestStreamingMapFullRejectsUncachedWriteWithoutLoading(t *testing.T) {
	img, ram := streamingMapFixture(128, 64, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	for i := 0; i < MapEditFillLimit; i++ {
		c.Mset(i%img.MapW, i/img.MapW, 1)
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("filling edit table to MapEditFillLimit set error %d", c.LastMapError())
	}
	if c.mapChunkSlot(0, 1) >= 0 {
		t.Fatal("test setup unexpectedly loaded target chunk")
	}
	stamp := c.mapStamp
	c.Mset(0, 32, 1)
	if c.LastMapError() != MapEditFull {
		t.Fatalf("uncached write at fill limit = %d, want MapEditFull", c.LastMapError())
	}
	if c.mapChunkSlot(0, 1) >= 0 || c.mapStamp != stamp {
		t.Fatal("rejected uncached write loaded or touched a map chunk")
	}

	// A write equal to ROM remains a no-op even after the overlay is full.
	c.ClearMapError()
	c.Mset(1, 32, 0)
	if c.LastMapError() != MapOK {
		t.Fatalf("ROM-equivalent write at fill limit = %d, want MapOK", c.LastMapError())
	}
	if c.mapChunkSlot(0, 1) >= 0 || c.mapStamp != stamp {
		t.Fatal("ROM-equivalent write loaded or touched a map chunk")
	}
}

func TestStreamingMapReinitResetsEditCount(t *testing.T) {
	img, ram := streamingMapFixture(128, 64, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("initial streaming Init returned false")
	}
	for i := 0; i < MapEditFillLimit; i++ {
		c.Mset(i%img.MapW, i/img.MapW, 1)
	}
	if c.mapEditCount != MapEditFillLimit {
		t.Fatalf("initial edit count = %d, want %d", c.mapEditCount, MapEditFillLimit)
	}
	if !c.Init(img, ram) {
		t.Fatal("re-init returned false")
	}
	if c.mapEditCount != 0 {
		t.Fatalf("re-init edit count = %d, want 0", c.mapEditCount)
	}
	if got := c.Mget(0, 0); got != 0 {
		t.Fatalf("re-init retained stale edit value %d, want 0", got)
	}
	c.Mset(0, 0, 1)
	if c.LastMapError() != MapOK || c.mapEditCount != 1 {
		t.Fatalf("first post-reinit edit error=%d count=%d, want MapOK/1", c.LastMapError(), c.mapEditCount)
	}
}

func TestStreamingMapNoopMsetDoesNotConsumeOverlay(t *testing.T) {
	img, ram := streamingMapFixture(192, 96, func(x, y int) uint16 { return 0 })
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}
	for i := 0; i < MapEditCapacity+256; i++ {
		x := i % img.MapW
		y := i / img.MapW
		c.Mset(x, y, 0)
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("no-op writes set map error %d", c.LastMapError())
	}
	for i, edit := range ram.MapEdits {
		if edit.Used != 0 {
			t.Fatalf("no-op write occupied edit slot %d", i)
		}
	}
}

func TestStreamingMapInitRejectsWrongCacheShape(t *testing.T) {
	img, ram := streamingMapFixture(96, 80, func(x, y int) uint16 { return 0 })
	ram.Map = make([]uint16, MapCacheCells-1)
	var c Context
	if c.Init(img, ram) {
		t.Fatal("Init accepted an undersized streaming cache")
	}
	if c.CartReady() {
		t.Fatal("failed streaming Init left cart ready")
	}
}

func TestStreamingMapRandomizedAccessMatchesReference(t *testing.T) {
	const width, height = 192, 96
	img, ram := streamingMapFixture(width, height, func(x, y int) uint16 {
		return uint16((x*13 + y*17) & 1)
	})
	reference := append([]uint16(nil), img.Map...)
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("streaming Init returned false")
	}

	// A deterministic pseudo-random trace mixes reads, writes, chunk seams,
	// and evictions. The write count is kept below the sparse table capacity,
	// while repeated writes exercise updates rather than new insertions.
	state := uint32(0x13579bdf)
	next := func() uint32 {
		state = state*1664525 + 1013904223
		return state
	}
	for step := 0; step < 30000; step++ {
		x := int(next() % width)
		y := int(next() % height)
		index := y*width + x
		if step%7 == 0 {
			value := uint16(next() & 1)
			c.ClearMapError()
			c.Mset(x, y, int(value))
			if c.LastMapError() == MapEditFull {
				t.Fatalf("edit table filled during bounded randomized trace at step %d", step)
			}
			reference[index] = value
		} else if got, want := c.Mget(x, y), reference[index]; got != want {
			t.Fatalf("step %d Mget(%d,%d) = %d, want %d", step, x, y, got, want)
		}
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if got, want := c.Mget(x, y), reference[y*width+x]; got != want {
				t.Fatalf("final Mget(%d,%d) = %d, want %d", x, y, got, want)
			}
		}
	}
}
