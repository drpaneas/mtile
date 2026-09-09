package runtime

import "github.com/drpaneas/mtile/runtime/audio"

// Screen dimensions are the native GBA display dimensions. FramebufferSize is
// retained as the Mode 4 backing-store size; the selected renderer decides at
// compile time whether Context actually contains that 38,400-byte array.
const (
	ScreenWidth      = 240
	ScreenHeight     = 160
	FramebufferSize  = ScreenWidth * ScreenHeight
	defaultDrawColor = 12
)

// Context owns all mutable runtime state. It must always be passed by pointer.
// renderState is build-tag selected so a Mode 0 ROM does not pay for the Mode 4
// software framebuffer in EWRAM.
type Context struct {
	cart *AssetMemory

	renderState
	drawPal         [256]byte
	displayPal      [256]byte
	transp          [256]bool
	palette         [256]uint16
	paletteReady    bool
	paletteDirty    bool
	transpKey0      bool
	drawPalIdentity bool

	keys       uint16
	prevKeys   uint16
	hold       [ButtonCount]uint16
	updateTick uint32
	tickRate   int
	frameSkips uint32

	profileEnabled   bool
	profileFrames    uint32
	profileMisses    uint32
	lastFrameCycles  uint32
	maxFrameCycles   uint32
	lastUpdateCycles uint32
	maxUpdateCycles  uint32
	lastDrawCycles   uint32
	maxDrawCycles    uint32
	profilePhaseTick uint32
	lastPresentStart uint16
	lastPresentEnd   uint16
	mapError         MapError
	mapROM           []uint16
	mapStamp         uint32
	mapEditCount     int

	blitKind       BlitKind
	lastBlitStart  uint16
	lastBlitEnd    uint16
	lastBlitCycles uint32

	drawReady bool
	camX      int
	camY      int
	clipX0    int
	clipY0    int
	clipX1    int
	clipY1    int
	cursor    drawCursor
	rng       uint32
	rngReady  bool

	heap        Heap
	heapReady   bool
	lastHeapErr HeapError
	staticTab   [staticStringSlots]StaticString
	staticHold  [staticStringSlots]string
	staticN     uint16

	nativeAudio *audio.Controller
}

// drawCursor is the current draw cursor. One draw colour is shared by
// drawing functions and print(), so keeping the colour with the cursor
// prevents the implicit print form from losing state between calls.
type drawCursor struct {
	x     int
	y     int
	color byte
}

// Init copies image into ram when assetLensExact passes. It always clears
// c.cart first so a failed re-init cannot leave a stale pointer.
func (c *Context) Init(image *AssetImage, ram *AssetMemory) bool {
	c.cart = nil
	c.resetRendererState()
	c.mapROM = nil
	c.mapStamp = 0
	c.mapEditCount = 0
	if !assetLensExact(image, ram) {
		return false
	}
	ram.SheetW, ram.SheetH = image.SheetW, image.SheetH
	ram.SpriteCols, ram.SpriteRows = image.SpriteCols, image.SpriteRows
	ram.MapW, ram.MapH = image.MapW, image.MapH
	ram.MapLayers = normalizedMapLayers(image.MapLayers)
	ram.MapPlaneMask = uint8(normalizedMapPlaneMask(image.MapPlaneMask, ram.MapLayers, len(image.Map), image.MapW*image.MapH))
	ram.SpriteN = image.SpriteN
	ram.MapStreaming = image.MapStreaming
	ram.SheetReadOnly = sheetStorageReadOnly || image.SheetReadOnly || ram.SheetReadOnly || len(ram.Sheet) == 0
	if sheetStorageReadOnly || ram.SheetReadOnly {
		// Copy only authored non-empty 8x8 cells and their logical-ID index.
		// The draw hot path stays in EWRAM; empty declared capacity consumes
		// two index bytes rather than 64 pixel bytes per cell.
		copy(ram.Sheet, image.Sheet)
		copy(ram.SheetIndex, image.SheetIndex)
	} else {
		copy(ram.Sheet, image.Sheet)
	}
	copy(ram.Flags, image.Flags)
	if mapStorageStreaming || !mapStorageFixed && image.MapStreaming {
		for i := range ram.Map {
			ram.Map[i] = 0
		}
		for i := range ram.MapChunks {
			ram.MapChunks[i] = MapChunk{}
		}
		for i := range ram.MapEdits {
			ram.MapEdits[i] = MapEdit{}
		}
		c.mapROM = image.Map
	} else {
		copy(ram.Map, image.Map)
	}
	c.mapError = MapOK
	c.cart = ram
	return true
}

// CartReady reports whether Init most recently succeeded. It is useful for a
// generated startup path that wants to fail closed without exposing the asset
// backing pointer.
func (c *Context) CartReady() bool {
	return c != nil && c.cart != nil
}

// CollectIfNeeded is implemented in heap.go. It runs the Cheney collector
// when the bump cursor has passed 75% of the active semispace.
