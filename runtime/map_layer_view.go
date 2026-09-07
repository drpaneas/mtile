package runtime

import "unsafe"

// MapLayerView is a compiler-internal borrow of one resident authored plane.
// The cartridge's metadata/backing and renderer mapping/camera/clip state
// must remain unchanged until the view expires. Cell/flag mutations and Lua
// GC are allowed. The compiler excludes drawing and other renderer-state
// calls throughout the borrow. No public Lua API exposes a view.
type MapLayerView struct {
	mapLayerViewRenderState
	MapLayerReadView
}

// MapLayerReadView retains just the authored plane. Read-only loops do not
// need to copy the writable view's renderer patch metadata onto their stack.
// Its cartridge metadata/backing has the same lifetime as MapLayerView.
type MapLayerReadView struct {
	context       *Context
	data          *uint16
	layer         int
	width, height int
	spriteN       int
}

// BorrowMapLayer checks the resident plane once. Streaming and malformed or
// absent backing retain the ordinary accessor path, including its validation
// order, chunk eviction, sparse edits and sticky MapError behavior.
func (c *Context) BorrowMapLayer(layer int) MapLayerView {
	view := MapLayerView{MapLayerReadView: c.BorrowMapLayerRead(layer)}
	if view.data != nil {
		view.borrowRenderState(c, layer)
	}
	return view
}

// BorrowMapLayerRead shares the writable view's validation and Get behavior,
// with no renderer cache preparation for loops that only read map cells.
func (c *Context) BorrowMapLayerRead(layer int) MapLayerReadView {
	view := MapLayerReadView{context: c, layer: layer}
	if c == nil || c.cart == nil || layer < 0 || layer >= c.cart.MapLayers ||
		mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming {
		return view
	}
	cart := c.cart
	slot, present := mapPlaneSlot(cart.MapPlaneMask, layer, cart.MapLayers)
	if !present || cart.MapW <= 0 || cart.MapH <= 0 || cart.MapW > len(cart.Map)/cart.MapH {
		return view
	}
	cells := cart.MapW * cart.MapH
	if slot >= len(cart.Map)/cells {
		return view
	}
	base := slot * cells
	view.data = &cart.Map[base]
	view.width, view.height, view.spriteN = cart.MapW, cart.MapH, cart.SpriteN
	return view
}

//go:inline
func (view *MapLayerReadView) Get(x, y int) uint16 {
	if view.data == nil {
		return view.getFallback(x, y)
	}
	if uint(x) >= uint(view.width) || uint(y) >= uint(view.height) {
		return 0
	}
	// Borrow proved the complete rectangular slice and the coordinate checks
	// bound this product; avoid another index check for each cell in a loop.
	pointer := unsafe.Add(unsafe.Pointer(view.data), (y*view.width+x)*2)
	return *(*uint16)(pointer)
}

//go:noinline
func (view *MapLayerReadView) getFallback(x, y int) uint16 {
	return view.context.MgetLayer(view.layer, x, y)
}

// Keep the resident loop body visible to LLVM without pulling the streaming
// and malformed-backing paths into each caller. Those paths retain the
// ordinary API checks and error effects in a shared ROM helper.
//
//go:inline
func (view *MapLayerView) Set(x, y, id int) {
	if view.data == nil {
		view.setFallback(x, y, id)
		return
	}
	if id < 0 || id >= view.spriteN || uint(x) >= uint(view.width) || uint(y) >= uint(view.height) {
		return
	}
	pointer := (*uint16)(unsafe.Add(unsafe.Pointer(view.data), (y*view.width+x)*2))
	value := uint16(id)
	if *pointer == value {
		return
	}
	*pointer = value
	view.mapLayerViewChanged(x, y, value)
}

//go:noinline
func (view *MapLayerView) setFallback(x, y, id int) {
	view.context.MsetLayer(view.layer, x, y, id)
}
