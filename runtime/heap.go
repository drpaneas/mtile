package runtime

import (
	"unsafe"
)

const (
	heapSize   = 56 * 1024
	maxHandles = 2048
	maxGlobals = 256
	maxLocals  = 512
	maxTemps   = 64
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
	ObjNone   ObjectKind = 0
	ObjTable  ObjectKind = 1
	ObjString ObjectKind = 2
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
	copiedN            uint16
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
	h.copiedN = 0
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

func (c *Context) PopLocals(n int) {
	if !c.heapReady || n <= 0 {
		return
	}
	sp := c.heap.roots.localSP - n
	if sp < 0 {
		sp = 0
	}
	for i := sp; i < c.heap.roots.localSP; i++ {
		c.heap.roots.locals[i] = Value{}
	}
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

func (c *Context) PopTemps(n int) {
	if !c.heapReady || n <= 0 {
		return
	}
	sp := c.heap.roots.tempCount - n
	if sp < 0 {
		sp = 0
	}
	for i := sp; i < c.heap.roots.tempCount; i++ {
		c.heap.roots.temps[i] = Value{}
	}
	c.heap.roots.tempCount = sp
}

func (c *Context) PopTemp() {
	c.PopTemps(1)
}

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
	if h.cursor > uint32(heapSize) || aligned > uint32(heapSize)-h.cursor {
		err := h.collect()
		if err != HeapOK {
			return 0, err
		}
		if h.cursor > uint32(heapSize) || aligned > uint32(heapSize)-h.cursor {
			h.terminal = HeapOOM
			return 0, HeapOOM
		}
	}
	if h.freeHead == invalidRef {
		err := h.collect()
		if err != HeapOK {
			return 0, err
		}
		if h.freeHead == invalidRef {
			h.terminal = HeapOOM
			return 0, HeapOOM
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
	for i := uint32(0); i < aligned; i++ {
		space[off+i] = 0
	}
	hdr.KindState = (uint8(kind) << 2) | uint8(HandleAllocated)
	hdr.Size = dataSize
	hdr.Offset = off
	h.cursor += aligned
	return ref, HeapOK
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
		if hdr.State() == HandleAllocated {
			hdr.SetState(HandleFree)
			hdr.Offset = uint32(h.freeHead)
			h.freeHead = Ref(i)
		}
	}
	for i := 0; i < maxHandles; i++ {
		if h.handles[i].State() == HandleForwarded {
			h.handles[i].SetState(HandleAllocated)
		}
	}
	h.active = 1 - h.active
	h.cursor = h.toCursor
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
	for i := uint32(0); i < size; i++ {
		to[dstOff+i] = from[srcOff+i]
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
