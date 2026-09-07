package runtime

import (
	"unsafe"
)

const (
	heapSize = 56 * 1024
	// MaxLocalRoots is exported so the transpiler can reject generated call
	// sites whose statically known roots cannot fit in Context's fixed stack.
	// Keeping the compiler and runtime on the same constant prevents a silent
	// nil result if that capacity changes in either package.
	MaxLocalRoots = 512
	maxHandles    = heapHandleSlots
	maxGlobals    = heapGlobalSlots
	maxLocals     = heapLocalSlots
	maxTemps      = heapTempSlots
)

// heapSpacePair is used only by the host implementation to give each test
// Context its own semispaces. TinyGo uses the two package-level EWRAM arrays
// in heap_storage_tinygo.go so a large heap is not embedded in Context.
type heapSpacePair struct {
	a [heapSize]byte
	b [heapSize]byte
}

type HandleState uint8

const (
	HandleFree      HandleState = 0
	HandleAllocated HandleState = 1
	HandleForwarded HandleState = 2
)

type ObjectKind uint8

const (
	ObjNone        ObjectKind = 0
	ObjTable       ObjectKind = 1
	ObjString      ObjectKind = 2
	ObjNumberArray ObjectKind = 3
)

type ObjectHeader struct {
	KindState uint8
	_         uint8
	Size      uint16
	Offset    uint32
}

func (h *ObjectHeader) State() HandleState {
	return HandleState(h.KindState & 0x03)
}

func (h *ObjectHeader) Kind() ObjectKind {
	return ObjectKind((h.KindState >> 2) & 0x03)
}

func (h *ObjectHeader) SetState(s HandleState) {
	h.KindState = (h.KindState & 0xFC) | uint8(s)
}

type HeapError uint8

const (
	HeapOK         HeapError = 0
	HeapOOM        HeapError = 1
	HeapCorrupt    HeapError = 2
	HeapCorruptRef HeapError = 3
	HeapBusy       HeapError = 4
)

type RootSet struct {
	globalPtrs  [maxGlobals]*Value
	globalCount int
	locals      [maxLocals]Value
	localSP     int
	temps       [maxTemps]Value
	tempCount   int
}

type Heap struct {
	localSpaces        *heapSpacePair
	active             int
	cursor             uint32
	handles            [maxHandles]ObjectHeader
	freeHead           Ref
	scanQueue          [maxHandles]Ref
	scanLen            uint16
	collecting         bool
	toCursor           uint32
	lastLiveBytes      uint32
	peakLiveBytes      uint32
	copiedN            uint16
	handleN            uint16
	terminal           HeapError
	roots              RootSet
	measureCollections bool
	collectCount       uint32
	lastCollectCycles  uint32
	maxCollectCycles   uint32
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(ObjectHeader{})-8]
)

func alignUp4(n uint32) uint32 {
	return (n + 3) &^ 3
}

func (h *Heap) initHeap() {
	h.initSpaces()
	for i := 0; i < maxHandles-1; i++ {
		h.handles[i].SetState(HandleFree)
		h.handles[i].Offset = uint32(i + 1)
	}
	h.handles[maxHandles-1].SetState(HandleFree)
	h.handles[maxHandles-1].Offset = uint32(invalidRef)
	h.freeHead = 0
	h.active = 0
	h.cursor = 0
	h.collecting = false
	h.scanLen = 0
	h.toCursor = 0
	h.lastLiveBytes = 0
	h.peakLiveBytes = 0
	h.copiedN = 0
	h.handleN = 0
	h.terminal = HeapOK
}

func (c *Context) ensureHeap() {
	if c.heapReady {
		return
	}
	c.heap.initHeap()
	c.heap.measureCollections = c.profileEnabled
	c.heapReady = true
}

func (c *Context) LastHeapError() HeapError {
	return c.lastHeapErr
}

func (c *Context) Collect() HeapError {
	c.ensureHeap()
	err := c.heap.collect()
	if err != HeapOK {
		c.lastHeapErr = err
	}
	return err
}

func (c *Context) CollectIfNeeded() {
	if !c.heapReady {
		return
	}
	if c.heap.cursor <= uint32(heapSize)*3/4 {
		return
	}
	if err := c.heap.collect(); err != HeapOK {
		c.lastHeapErr = err
	}
}

func (c *Context) RegisterGlobal(p *Value) {
	c.ensureHeap()
	if p == nil || c.heap.roots.globalCount >= maxGlobals {
		return
	}
	c.heap.roots.globalPtrs[c.heap.roots.globalCount] = p
	c.heap.roots.globalCount++
}

func (c *Context) PushLocal(v Value) bool {
	c.ensureHeap()
	if c.heap.roots.localSP >= maxLocals {
		return false
	}
	c.heap.roots.locals[c.heap.roots.localSP] = v
	c.heap.roots.localSP++
	return true
}

// Keep root unwinding shared: inlining this small routine into every generated
// helper duplicates it through IWRAM callers, consuming their fixed code budget.
//
//go:noinline
func (c *Context) PopLocals(n int) {
	if !c.heapReady || n <= 0 {
		return
	}
	sp := c.heap.roots.localSP - n
	if sp < 0 {
		sp = 0
	}
	// The collector scans only the active prefix, and PushLocal overwrites a
	// slot before extending it. Values hold integer handles, not Go pointers,
	// so clearing the dead tail cannot release any additional object.
	c.heap.roots.localSP = sp
}

func (c *Context) PushTemp(v Value) bool {
	c.ensureHeap()
	if c.heap.roots.tempCount >= maxTemps {
		return false
	}
	c.heap.roots.temps[c.heap.roots.tempCount] = v
	c.heap.roots.tempCount++
	return true
}

// Keep temp unwinding shared with PopLocals: inlining duplicates the same
// small routine through IWRAM callers and consumes their fixed code budget.
//
//go:noinline
func (c *Context) PopTemps(n int) {
	if !c.heapReady || n <= 0 {
		return
	}
	sp := c.heap.roots.tempCount - n
	if sp < 0 {
		sp = 0
	}
	// As with locals, only the active prefix is a root. PushTemp replaces the
	// entire Value before making an inactive slot visible to the collector.
	c.heap.roots.tempCount = sp
}

func (c *Context) PopTemp() {
	c.PopTemps(1)
}

// Allocation initializes every byte of ordinary table records. Keep its
// validation and word-clear loop in fast internal RAM on the GBA.
//
//go:noinline
//go:section .iwram
func (h *Heap) allocObject(kind ObjectKind, dataSize uint16) (Ref, HeapError) {
	if h.terminal != HeapOK {
		return 0, h.terminal
	}
	if h.collecting {
		return 0, HeapBusy
	}
	aligned := alignUp4(uint32(dataSize))
	if aligned > uint32(heapSize) {
		return 0, HeapOOM
	}
	if h.cursor > uint32(heapSize) || aligned > uint32(heapSize)-h.cursor || h.freeHead == invalidRef {
		if err := h.prepareAllocation(aligned); err != HeapOK {
			return 0, err
		}
	}
	ref := h.freeHead
	if ref >= maxHandles {
		h.terminal = HeapCorrupt
		return 0, HeapCorrupt
	}
	hdr := &h.handles[ref]
	if hdr.State() != HandleFree {
		h.terminal = HeapCorrupt
		return 0, HeapCorrupt
	}
	h.freeHead = Ref(hdr.Offset)
	off := h.cursor
	space := h.space(h.active)
	// Bounds were proved above. Allocated payloads are normally word-aligned;
	// clear in native words so a large table does not pay one EWRAM store and
	// one loop/bounds check per byte. The fallback retains exact behavior for
	// an unusual in-bounds unaligned cursor. Padding is still cleared too.
	if aligned != 0 {
		base := unsafe.Add(unsafe.Pointer(&space[0]), off)
		at := uint32(0)
		if uintptr(base)&3 == 0 {
			for aligned-at >= 16 {
				pointer := unsafe.Add(base, at)
				*(*uint32)(pointer) = 0
				*(*uint32)(unsafe.Add(pointer, 4)) = 0
				*(*uint32)(unsafe.Add(pointer, 8)) = 0
				*(*uint32)(unsafe.Add(pointer, 12)) = 0
				at += 16
			}
			for aligned-at >= 4 {
				*(*uint32)(unsafe.Add(base, at)) = 0
				at += 4
			}
		}
		for at < aligned {
			*(*byte)(unsafe.Add(base, at)) = 0
			at++
		}
	}
	hdr.KindState = (uint8(kind) << 2) | uint8(HandleAllocated)
	hdr.Size = dataSize
	hdr.Offset = off
	h.cursor += aligned
	h.handleN++
	return ref, HeapOK
}

// allocObjectFilled reserves the same handle and payload as allocObject, then
// copies a complete image over the payload instead of clearing it first.
// Constructor images already define every byte of hdr.Size; skipping the
// EWRAM zero-fill avoids writing the same range twice. Padding up to the
// 4-byte cursor rounding is still cleared, matching allocObject. Stay in ROM:
// inlining a copy loop into IWRAM callers would spend their fixed code budget.
//
//go:noinline
func (h *Heap) allocObjectFilled(kind ObjectKind, image []byte) (Ref, HeapError) {
	if h.terminal != HeapOK {
		return 0, h.terminal
	}
	if h.collecting {
		return 0, HeapBusy
	}
	dataSize := uint16(len(image))
	aligned := alignUp4(uint32(dataSize))
	if aligned > uint32(heapSize) {
		return 0, HeapOOM
	}
	if h.cursor > uint32(heapSize) || aligned > uint32(heapSize)-h.cursor || h.freeHead == invalidRef {
		if err := h.prepareAllocation(aligned); err != HeapOK {
			return 0, err
		}
	}
	ref := h.freeHead
	if ref >= maxHandles {
		h.terminal = HeapCorrupt
		return 0, HeapCorrupt
	}
	hdr := &h.handles[ref]
	if hdr.State() != HandleFree {
		h.terminal = HeapCorrupt
		return 0, HeapCorrupt
	}
	h.freeHead = Ref(hdr.Offset)
	off := h.cursor
	space := h.space(h.active)
	base := unsafe.Add(unsafe.Pointer(&space[0]), off)
	if dataSize != 0 {
		copy(unsafe.Slice((*byte)(base), int(dataSize)), image)
	}
	for at := uint32(dataSize); at < aligned; at++ {
		*(*byte)(unsafe.Add(base, at)) = 0
	}
	hdr.KindState = (uint8(kind) << 2) | uint8(HandleAllocated)
	hdr.Size = dataSize
	hdr.Offset = off
	h.cursor += aligned
	h.handleN++
	return ref, HeapOK
}

// Collection is infrequent compared with bump allocation. Keep its retry
// branches in ROM so the allocator's checked common path fits in IWRAM.
// The space-first, handles-second order matches allocObject's original error
// behavior, including the second collection when handles remain exhausted.
//
//go:noinline
func (h *Heap) prepareAllocation(aligned uint32) HeapError {
	if h.cursor > uint32(heapSize) || aligned > uint32(heapSize)-h.cursor {
		if err := h.collect(); err != HeapOK {
			return err
		}
		if h.cursor > uint32(heapSize) || aligned > uint32(heapSize)-h.cursor {
			h.terminal = HeapOOM
			return HeapOOM
		}
	}
	if h.freeHead == invalidRef {
		if err := h.collect(); err != HeapOK {
			return err
		}
		if h.freeHead == invalidRef {
			h.terminal = HeapOOM
			return HeapOOM
		}
	}
	return HeapOK
}

func (h *Heap) collect() HeapError {
	if !h.measureCollections {
		return h.collectCore()
	}
	cycleStart := startShortCycleMeasurement()
	err := h.collectCore()
	cycles := stopShortCycleMeasurement(cycleStart)
	h.collectCount++
	h.lastCollectCycles = cycles
	if cycles > h.maxCollectCycles {
		h.maxCollectCycles = cycles
	}
	return err
}

// collectCore contains the collector itself. Keeping timing outside this
// function avoids defer and its runtime.alloc dependency in -gc=none builds.
func (h *Heap) collectCore() HeapError {
	if h.terminal != HeapOK {
		return h.terminal
	}
	if h.collecting {
		return HeapBusy
	}
	h.collecting = true
	h.toCursor = 0
	h.scanLen = 0
	h.copiedN = 0
	if h.roots.globalCount > maxGlobals || h.roots.localSP > maxLocals || h.roots.tempCount > maxTemps {
		return h.failCollection(HeapCorrupt)
	}
	for i := 0; i < h.roots.globalCount; i++ {
		p := h.roots.globalPtrs[i]
		if p == nil {
			continue
		}
		if err := h.copyRoot(*p); err != HeapOK {
			return h.failCollection(err)
		}
	}
	for i := 0; i < h.roots.localSP; i++ {
		if err := h.copyRoot(h.roots.locals[i]); err != HeapOK {
			return h.failCollection(err)
		}
	}
	for i := 0; i < h.roots.tempCount; i++ {
		if err := h.copyRoot(h.roots.temps[i]); err != HeapOK {
			return h.failCollection(err)
		}
	}
	if err := h.scanCopied(); err != HeapOK {
		return h.failCollection(err)
	}
	for i := 0; i < maxHandles; i++ {
		hdr := &h.handles[i]
		switch hdr.State() {
		case HandleAllocated:
			hdr.SetState(HandleFree)
			hdr.Offset = uint32(h.freeHead)
			h.freeHead = Ref(i)
		case HandleForwarded:
			// Tracing is finished: restoring copied handles while reclaiming
			// dead ones preserves the same free-list order in one sweep.
			hdr.SetState(HandleAllocated)
		}
	}
	h.active = 1 - h.active
	h.cursor = h.toCursor
	h.lastLiveBytes = h.cursor
	if h.lastLiveBytes > h.peakLiveBytes {
		h.peakLiveBytes = h.lastLiveBytes
	}
	h.handleN = h.copiedN
	h.collecting = false
	return HeapOK
}

func (h *Heap) failCollection(err HeapError) HeapError {
	if err != HeapBusy {
		h.terminal = err
	}
	h.collecting = false
	return err
}

func (h *Heap) copyRoot(v Value) HeapError {
	if v.Kind == KindTable || v.Kind == KindString {
		return h.copyObject(v.Ref)
	}
	return HeapOK
}

func (h *Heap) scanCopied() HeapError {
	to := h.space(1 - h.active)
	scanHead := 0
	for scanHead < int(h.scanLen) {
		ref := h.scanQueue[scanHead]
		scanHead++
		hdr := &h.handles[ref]
		if hdr.Kind() != ObjTable {
			continue
		}
		if hdr.Size < 8 {
			return HeapCorrupt
		}
		if hdr.Offset > uint32(heapSize) || uint32(hdr.Size) > uint32(heapSize)-hdr.Offset {
			return HeapCorrupt
		}
		arrayCap := readU16(to, hdr.Offset+2)
		hashCap := readU16(to, hdr.Offset+6)
		computed, slots, ok := tableLayoutSize(arrayCap, hashCap)
		if !ok || computed > uint32(hdr.Size) {
			return HeapCorrupt
		}
		base := hdr.Offset + 8
		for i := uint32(0); i < slots; i++ {
			slot := readValue(to, base+i*uint32(valueBytes))
			if slot.Kind == KindTable || slot.Kind == KindString {
				if err := h.copyObject(slot.Ref); err != HeapOK {
					return err
				}
			}
		}
	}
	return HeapOK
}

func (h *Heap) copyObject(ref Ref) HeapError {
	if ref >= maxHandles || ref == invalidRef {
		return HeapCorruptRef
	}
	hdr := &h.handles[ref]
	if hdr.State() == HandleForwarded {
		return HeapOK
	}
	if hdr.State() != HandleAllocated {
		return HeapCorruptRef
	}
	if hdr.Offset > uint32(heapSize) || uint32(hdr.Size) > uint32(heapSize)-hdr.Offset {
		return HeapCorrupt
	}
	aligned := alignUp4(uint32(hdr.Size))
	if h.toCursor > uint32(heapSize) || aligned > uint32(heapSize)-h.toCursor {
		return HeapOOM
	}
	if h.scanLen >= maxHandles {
		return HeapOOM
	}
	from := h.space(h.active)
	to := h.space(1 - h.active)
	srcOff := hdr.Offset
	dstOff := h.toCursor
	size := uint32(hdr.Size)
	// The bounds above validate both ranges, and semispaces are disjoint.
	// Normal object offsets are four-byte aligned. Copy whole words without
	// repeating slice bounds checks for every byte, then preserve the exact
	// payload tail (odd-sized strings must not copy neighboring padding).
	// The fallback retains byte-copy behavior for an unusual unaligned offset.
	src := unsafe.Add(unsafe.Pointer(&from[0]), srcOff)
	dst := unsafe.Add(unsafe.Pointer(&to[0]), dstOff)
	remaining := size
	if (uintptr(src)|uintptr(dst))&3 == 0 {
		for remaining >= 16 {
			*(*uint32)(dst) = *(*uint32)(src)
			*(*uint32)(unsafe.Add(dst, 4)) = *(*uint32)(unsafe.Add(src, 4))
			*(*uint32)(unsafe.Add(dst, 8)) = *(*uint32)(unsafe.Add(src, 8))
			*(*uint32)(unsafe.Add(dst, 12)) = *(*uint32)(unsafe.Add(src, 12))
			src = unsafe.Add(src, 16)
			dst = unsafe.Add(dst, 16)
			remaining -= 16
		}
		for remaining >= 4 {
			*(*uint32)(dst) = *(*uint32)(src)
			src = unsafe.Add(src, 4)
			dst = unsafe.Add(dst, 4)
			remaining -= 4
		}
	}
	for remaining > 0 {
		*(*byte)(dst) = *(*byte)(src)
		src = unsafe.Add(src, 1)
		dst = unsafe.Add(dst, 1)
		remaining--
	}
	hdr.Offset = dstOff
	hdr.SetState(HandleForwarded)
	h.scanQueue[h.scanLen] = ref
	h.scanLen++
	h.copiedN++
	h.toCursor += aligned
	return HeapOK
}

func (h *Heap) checkLive(ref Ref) HeapError {
	if ref >= maxHandles || ref == invalidRef {
		return HeapCorruptRef
	}
	if h.handles[ref].State() == HandleFree {
		return HeapCorruptRef
	}
	return HeapOK
}

func (c *Context) activeSpace() []byte {
	return c.heap.space(c.heap.active)
}

// freshObjectBytes is only used immediately after allocObject succeeds,
// before any allocation/collection or handle mutation. That success already
// proves the live handle and complete payload range checked by objectBytes.
func (c *Context) freshObjectBytes(ref Ref) []byte {
	header := &c.heap.handles[ref]
	return c.activeSpace()[header.Offset : header.Offset+uint32(header.Size)]
}

func (c *Context) objectBytes(ref Ref) ([]byte, HeapError) {
	if c.heap.terminal != HeapOK {
		return nil, c.heap.terminal
	}
	if err := c.heap.checkLive(ref); err != HeapOK {
		return nil, err
	}
	hdr := &c.heap.handles[ref]
	if hdr.Offset > uint32(heapSize) || uint32(hdr.Size) > uint32(heapSize)-hdr.Offset {
		return nil, HeapCorrupt
	}
	return c.activeSpace()[hdr.Offset : hdr.Offset+uint32(hdr.Size)], HeapOK
}

func readU16(b []byte, off uint32) uint16 {
	return uint16(b[off]) | uint16(b[off+1])<<8
}

func writeU16(b []byte, off uint32, v uint16) {
	b[off] = byte(v)
	b[off+1] = byte(v >> 8)
}

func readValue(b []byte, off uint32) Value {
	return *(*Value)(unsafe.Pointer(&b[off]))
}

func writeValue(b []byte, off uint32, v Value) {
	*(*Value)(unsafe.Pointer(&b[off])) = v
}
