package runtime

import (
	goruntime "runtime"
	"unsafe"
)

const (
	defaultTableArrayCap = 16
	defaultTableHashCap  = 8
	numberArrayHeader    = 4
	// Numeric payloads retain their existing little-endian byte layout. The
	// GBA and usual host targets can access aligned words directly; other
	// architectures compile only the portable bytewise path.
	littleEndianNumberWords = goruntime.GOARCH == "386" || goruntime.GOARCH == "amd64" ||
		goruntime.GOARCH == "arm" || goruntime.GOARCH == "arm64" || goruntime.GOARCH == "loong64" ||
		goruntime.GOARCH == "mipsle" || goruntime.GOARCH == "mips64le" || goruntime.GOARCH == "ppc64le" ||
		goruntime.GOARCH == "riscv64" || goruntime.GOARCH == "wasm"
)

// NewNumberArray allocates a fixed-length, number-only Lua array. It retains
// KindTable so the GC roots it like every other table, but its payload uses one
// 4-byte Number per slot instead of a 12-byte Value. Generated code selects
// this representation only for non-escaping arrays with monomorphic numeric
// elements and uses the matching accessors below.
func (c *Context) NewNumberArray(length int) (Value, HeapError) {
	if length < 0 || length > maxArrayCap {
		return NilVal(), HeapOOM
	}
	size := numberArrayHeader + length*4
	if size > 0xFFFF {
		return NilVal(), HeapOOM
	}
	c.ensureHeap()
	ref, err := c.heap.allocObject(ObjNumberArray, uint16(size))
	if err != HeapOK {
		return NilVal(), err
	}
	data := c.freshObjectBytes(ref)
	writeU16(data, 0, uint16(length))
	return Value{Kind: KindTable, Ref: ref}, HeapOK
}

func (c *Context) numberArrayData(array Value) ([]byte, int, HeapError) {
	if array.Kind != KindTable || array.Ref >= maxHandles || array.Ref == invalidRef {
		return nil, 0, HeapCorruptRef
	}
	if c.heap.handles[array.Ref].Kind() != ObjNumberArray {
		return nil, 0, HeapCorruptRef
	}
	data, err := c.objectBytes(array.Ref)
	if err != HeapOK || len(data) < numberArrayHeader {
		return nil, 0, HeapCorrupt
	}
	length := int(readU16(data, 0))
	if numberArrayHeader+length*4 > len(data) {
		return nil, 0, HeapCorrupt
	}
	return data, length, HeapOK
}

// NumberArrayGet returns zero for an invalid object or out-of-range index.
// Indices are one-based to preserve Lua's array convention.
//
//go:noinline
//go:section .iwram
func (c *Context) NumberArrayGet(array Value, index int) Number {
	data, length, err := c.numberArrayData(array)
	if err != HeapOK || index < 1 || index > length {
		return 0
	}
	offset := numberArrayHeader + (index-1)*4
	// numberArrayData validated the whole payload, and the index check above
	// proves that all four bytes exist. Real heap objects are aligned; one
	// native word avoids four EWRAM byte reads and repeated bounds checks.
	// Preserve the bytewise path for unusual in-bounds, unaligned headers.
	pointer := unsafe.Pointer(&data[offset])
	if littleEndianNumberWords && uintptr(pointer)&3 == 0 {
		return Number(*(*int32)(pointer))
	}
	raw := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
	return Number(int32(raw))
}

// NumberArraySet stores a native Number and ignores out-of-range indices,
// matching the experiment's fixed-capacity contract without risking a panic.
//
//go:noinline
//go:section .iwram
func (c *Context) NumberArraySet(array Value, index int, number Number) HeapError {
	data, length, err := c.numberArrayData(array)
	if err != HeapOK {
		return err
	}
	if index < 1 || index > length {
		return HeapOK
	}
	offset := numberArrayHeader + (index-1)*4
	pointer := unsafe.Pointer(&data[offset])
	if littleEndianNumberWords && uintptr(pointer)&3 == 0 {
		*(*int32)(pointer) = int32(number)
		return HeapOK
	}
	raw := uint32(number)
	data[offset] = byte(raw)
	data[offset+1] = byte(raw >> 8)
	data[offset+2] = byte(raw >> 16)
	data[offset+3] = byte(raw >> 24)
	return HeapOK
}

func (c *Context) NumberArrayLen(array Value) int {
	_, length, err := c.numberArrayData(array)
	if err != HeapOK {
		return 0
	}
	return length
}

// TableFieldCache is the one-based byte offset of a stable table field.
// Zero means that the access site has not resolved its field yet.
//
// Generated code owns one cache per static field access site. The first
// access performs the complete table/key checks; later accesses deliberately
// assume that the value is still a table with the same layout. This is the
// monomorphic fast path used by game state records.
type TableFieldCache uint16

func tableLayoutSize(arrayCap, hashCap uint16) (uint32, uint32, bool) {
	if arrayCap > MaxTableArrayCapacity || hashCap > MaxTableHashCapacity {
		return 0, 0, false
	}
	slots := uint32(arrayCap) + 2*uint32(hashCap)
	return 8 + slots*uint32(valueBytes), slots, true
}

func tableDataSize(arrayCap, hashCap uint16) uint16 {
	size, _, ok := tableLayoutSize(arrayCap, hashCap)
	if !ok || size > 0xFFFF {
		return 0
	}
	return uint16(size)
}

func (c *Context) NewTable() (Value, HeapError) {
	return c.NewTableWithCaps(defaultTableArrayCap, defaultTableHashCap)
}

// NewTableWithCaps allocates a table sized for the caller's expected usage.
// Generated code should provide compile-time capacity hints; the default
// NewTable keeps hand-written callers from paying for the maximum table.
func (c *Context) NewTableWithCaps(arrayCap, hashCap int) (Value, HeapError) {
	if arrayCap < 0 || arrayCap > maxArrayCap || hashCap < 0 || hashCap > maxHashCap {
		return NilVal(), HeapOOM
	}
	c.ensureHeap()
	arrayCap16 := uint16(arrayCap)
	hashCap16 := uint16(hashCap)
	size := tableDataSize(arrayCap16, hashCap16)
	ref, err := c.heap.allocObject(ObjTable, size)
	if err != HeapOK {
		return NilVal(), err
	}
	data := c.freshObjectBytes(ref)
	writeU16(data, 2, arrayCap16)
	writeU16(data, 6, hashCap16)
	return Value{Kind: KindTable, Ref: ref}, HeapOK
}

// Table reads and writes share these validation checks. Keep the repeated
// instruction fetches in fast internal RAM on the GBA.
//
//go:noinline
//go:section .iwram
func (c *Context) tableData(t Value) ([]byte, HeapError) {
	if t.Kind != KindTable {
		return nil, HeapCorruptRef
	}
	// Keep objectBytes' validation order, but retain the checked handle here.
	// Calling it and then looking up Kind again makes ARM validate and address
	// the same handle twice on every uncached table access.
	if c.heap.terminal != HeapOK {
		return nil, c.heap.terminal
	}
	if t.Ref >= maxHandles || t.Ref == invalidRef {
		return nil, HeapCorruptRef
	}
	header := &c.heap.handles[t.Ref]
	if header.State() == HandleFree {
		return nil, HeapCorruptRef
	}
	offset, size := header.Offset, uint32(header.Size)
	if offset > uint32(heapSize) || size > uint32(heapSize)-offset {
		return nil, HeapCorrupt
	}
	data := c.activeSpace()[offset : offset+size]
	if header.Kind() != ObjTable {
		return nil, HeapCorruptRef
	}
	if len(data) < 8 {
		return nil, HeapCorrupt
	}
	var arrayLen, arrayCap, hashLen, hashCap uint16
	if littleEndianNumberWords && uintptr(unsafe.Pointer(&data[0]))&1 == 0 {
		// One alignment check covers the complete, already bounds-checked
		// header. Halfword reads fit the GBA's 16-bit EWRAM bus directly.
		words := (*[4]uint16)(unsafe.Pointer(&data[0]))
		arrayLen, arrayCap, hashLen, hashCap = words[0], words[1], words[2], words[3]
	} else {
		arrayLen, arrayCap = readU16(data, 0), readU16(data, 2)
		hashLen, hashCap = readU16(data, 4), readU16(data, 6)
	}
	need, _, ok := tableLayoutSize(arrayCap, hashCap)
	if !ok || uint32(len(data)) < need || arrayLen > arrayCap || hashLen > hashCap {
		return nil, HeapCorrupt
	}
	return data, HeapOK
}

func arraySlotOff(i int) uint32 {
	return 8 + uint32(i)*uint32(valueBytes)
}

func hashKeyOff(arrayCap uint16, i int) uint32 {
	return 8 + uint32(arrayCap)*uint32(valueBytes) + uint32(i)*uint32(valueBytes)
}

func hashValOff(arrayCap, hashCap uint16, i int) uint32 {
	return 8 + uint32(arrayCap)*uint32(valueBytes) + uint32(hashCap)*uint32(valueBytes) + uint32(i)*uint32(valueBytes)
}

func integerKey(v Value) (int, bool) {
	if v.Kind != KindNumber {
		return 0, false
	}
	if v.Num != v.Num.Flr() {
		return 0, false
	}
	n := v.Num.Int()
	return n, true
}

func isTombstone(key, val Value) bool {
	return key.Kind == KindNil && val.Kind == KindBool && val.Num != 0
}

func (c *Context) TableLen(t Value) int {
	data, err := c.tableData(t)
	if err != HeapOK {
		return 0
	}
	return tableArrayLen(data, int(readU16(data, 2)))
}

// tableArrayLen takes a slice and capacity already checked by tableData.
// Only the Kind byte of each slot determines the first nil boundary. A byte
// stride avoids rechecking a slice bound for every slot of every list call;
// keep the pointer at the object base so no one-past-end pointer is formed.
func tableArrayLen(data []byte, arrayCap int) int {
	base := unsafe.Pointer(&data[0])
	n := 0
	for n < arrayCap {
		if Kind(*(*byte)(unsafe.Add(base, arraySlotOff(n)))) == KindNil {
			break
		}
		n++
	}
	return n
}

func (c *Context) TableGetInt(t Value, i int) Value {
	return c.TableGet(t, IntVal(i))
}

func (c *Context) TableGet(t, key Value) Value {
	data, err := c.tableData(t)
	if err != HeapOK {
		return NilVal()
	}
	arrayCap := int(readU16(data, 2))
	if n, ok := integerKey(key); ok && n >= 1 && n <= arrayCap {
		return readValue(data, arraySlotOff(n-1))
	}
	idx, found := c.hashFind(data, key)
	if !found {
		return NilVal()
	}
	hashCap := readU16(data, 6)
	arrayCapU := readU16(data, 2)
	return readValue(data, hashValOff(arrayCapU, hashCap, idx))
}

// TableGetStable resolves a static field normally on its first access, then
// reads the cached byte offset without repeating type, handle-kind, capacity,
// hash, or key-equality checks. The table may move during GC because its stable
// handle is resolved on every access; its type and layout must not change.
//
//go:noinline
//go:section .iwram
func (c *Context) TableGetStable(t, key Value, cache *TableFieldCache) Value {
	if *cache == 0 {
		return c.tableGetStableFirst(t, key, cache)
	}
	hdr := &c.heap.handles[t.Ref]
	return readValue(c.activeSpace(), hdr.Offset+uint32(*cache)-1)
}

// Keep first-use validation in ROM; warm field access is the IWRAM hot path.
//
//go:noinline
func (c *Context) tableGetStableFirst(t, key Value, cache *TableFieldCache) Value {
	data, err := c.tableData(t)
	if err != HeapOK {
		return NilVal()
	}
	off, found := c.tableFieldOffset(data, key)
	if !found {
		return NilVal()
	}
	*cache = TableFieldCache(off + 1)
	return readValue(data, off)
}

// TableGetStableNumber is the numeric monomorphic field path. Its first read
// validates both the table slot and the field's number kind. Warm reads load
// the fixed-point payload only while the base is still KindTable; nil and
// other kinds take the cold path instead of indexing handle 0.
//
//go:noinline
//go:section .iwram
func (c *Context) TableGetStableNumber(t, key Value, cache *TableFieldCache) Number {
	if *cache == 0 || t.Kind != KindTable {
		return c.tableGetStableNumberFirst(t, key, cache)
	}
	hdr := &c.heap.handles[t.Ref]
	return readValue(c.activeSpace(), hdr.Offset+uint32(*cache)-1).Num
}

//go:noinline
func (c *Context) tableGetStableNumberFirst(t, key Value, cache *TableFieldCache) Number {
	data, err := c.tableData(t)
	if err != HeapOK {
		return 0
	}
	off, found := c.tableFieldOffset(data, key)
	if !found {
		return 0
	}
	value := readValue(data, off)
	if value.Kind != KindNumber {
		return 0
	}
	*cache = TableFieldCache(off + 1)
	return value.Num
}

func (c *Context) TableSet(t, key, val Value) HeapError {
	if key.Kind == KindNil {
		return HeapOK
	}
	data, err := c.tableData(t)
	if err != HeapOK {
		return err
	}
	arrayCap := int(readU16(data, 2))
	if n, ok := integerKey(key); ok && n >= 1 && n <= arrayCap {
		writeValue(data, arraySlotOff(n-1), val)
		return HeapOK
	}
	idx, found := c.hashFind(data, key)
	arrayCapU := readU16(data, 2)
	hashCap := readU16(data, 6)
	hashLen := readU16(data, 4)
	if found {
		if val.Kind == KindNil {
			writeValue(data, hashKeyOff(arrayCapU, idx), NilVal())
			writeValue(data, hashValOff(arrayCapU, hashCap, idx), BoolVal(true))
			if hashLen > 0 {
				writeU16(data, 4, hashLen-1)
			}
			return HeapOK
		}
		writeValue(data, hashValOff(arrayCapU, hashCap, idx), val)
		return HeapOK
	}
	if val.Kind == KindNil {
		return HeapOK
	}
	slot, ok := c.hashInsertSlot(data, key)
	if !ok {
		newHash, ok := growCap(int(hashCap), int(hashCap)+1, maxHashCap)
		if !ok {
			return HeapOOM
		}
		if err := c.growTableWithRoots(t, arrayCap, newHash, key, val); err != HeapOK {
			return err
		}
		data, err = c.tableData(t)
		if err != HeapOK {
			return err
		}
		slot, ok = c.hashInsertSlot(data, key)
		if !ok {
			return HeapOOM
		}
		arrayCapU = readU16(data, 2)
		hashCap = readU16(data, 6)
		hashLen = readU16(data, 4)
	}
	writeValue(data, hashKeyOff(arrayCapU, slot), key)
	writeValue(data, hashValOff(arrayCapU, hashCap, slot), val)
	writeU16(data, 4, hashLen+1)
	return HeapOK
}

// TableSetStable is the write counterpart to TableGetStable. The first write
// uses normal table semantics and caches the resulting field offset. Later
// writes update that slot only while the base is still KindTable.
//
//go:noinline
//go:section .iwram
func (c *Context) TableSetStable(t, key, val Value, cache *TableFieldCache) HeapError {
	if *cache == 0 || t.Kind != KindTable {
		return c.tableSetStableFirst(t, key, val, cache)
	}
	hdr := &c.heap.handles[t.Ref]
	writeValue(c.activeSpace(), hdr.Offset+uint32(*cache)-1, val)
	return HeapOK
}

//go:noinline
func (c *Context) tableSetStableFirst(t, key, val Value, cache *TableFieldCache) HeapError {
	if err := c.TableSet(t, key, val); err != HeapOK {
		return err
	}
	data, err := c.tableData(t)
	if err != HeapOK {
		return err
	}
	off, found := c.tableFieldOffset(data, key)
	if found {
		*cache = TableFieldCache(off + 1)
	}
	return HeapOK
}

// TableSetStableNumber writes a native fixed-point payload into a stable field
// while preserving the table's Value storage for GC and generic Lua access.
func (c *Context) TableSetStableNumber(t, key Value, number Number, cache *TableFieldCache) HeapError {
	return c.TableSetStable(t, key, NumVal(number), cache)
}

// TableSetCached and TableSetCachedNumber are compiler-only warm stores. The
// caller has checked a nonzero stable offset and KindTable, retaining the same
// table-layout proof as TableSetStable. Neither can allocate or collect. Compact
// arguments avoid passing the unused key and table Value payload over the ARM stack.
//
//go:noinline
//go:section .iwram
func (c *Context) TableSetCached(ref Ref, cache TableFieldCache, value Value) {
	hdr := &c.heap.handles[ref]
	writeValue(c.activeSpace(), hdr.Offset+uint32(cache)-1, value)
}

//go:noinline
//go:section .iwram
func (c *Context) TableSetCachedNumber(ref Ref, cache TableFieldCache, number Number) {
	hdr := &c.heap.handles[ref]
	writeValue(c.activeSpace(), hdr.Offset+uint32(cache)-1, NumVal(number))
}

func (c *Context) tableFieldOffset(data []byte, key Value) (uint32, bool) {
	arrayCap := int(readU16(data, 2))
	if n, ok := integerKey(key); ok && n >= 1 && n <= arrayCap {
		return arraySlotOff(n - 1), true
	}
	idx, found := c.hashFind(data, key)
	if !found {
		return 0, false
	}
	return hashValOff(readU16(data, 2), readU16(data, 6), idx), true
}

// hashBucket preserves modulo for arbitrary capacities, but avoids software
// division on the GBA for the common power-of-two tables. Callers have already
// checked that capacity is nonzero.
func hashBucket(hash uint32, capacity uint16) int {
	mask := uint32(capacity) - 1
	if uint32(capacity)&mask == 0 {
		return int(hash & mask)
	}
	return int(hash % uint32(capacity))
}

// Run the lookup/probe loop in fast internal RAM, including the common
// existing-key path used by both TableGet and TableSet.
//
//go:noinline
//go:section .iwram
func (c *Context) hashFind(data []byte, key Value) (int, bool) {
	hashCap := int(readU16(data, 6))
	if hashCap == 0 {
		return 0, false
	}
	arrayCap := readU16(data, 2)
	var hint *StaticString
	if key.Kind == KindStaticString && key.SRef < c.staticN {
		hint = &c.staticTab[key.SRef]
		slot := int(hint.slotHint)
		if hint.hintSet && slot < hashCap {
			candidate := readValue(data, hashKeyOff(arrayCap, slot))
			if candidate.Kind == KindStaticString && candidate.SRef == key.SRef {
				return slot, true
			}
		}
	}
	start := hashBucket(c.hashValue(key), uint16(hashCap))
	i := start
	for {
		k := readValue(data, hashKeyOff(arrayCap, i))
		if k.Kind == KindNil {
			// Only a nil key needs the value slot (tombstone test), so the
			// common occupied-probe case skips one 12-byte read.
			v := readValue(data, hashValOff(arrayCap, readU16(data, 6), i))
			if !isTombstone(k, v) {
				return 0, false
			}
		} else if (key.Kind == KindNumber && k.Kind == KindNumber && k.Num == key.Num) ||
			(key.Kind != KindNumber && c.ValuesEqual(k, key)) {
			// Numeric keys need only their exact fixed-point payload. Keep
			// this common comparison inside the IWRAM probe loop instead of
			// copying two Values into the generic ROM string/table decoder.
			if hint != nil {
				hint.slotHint, hint.hintSet = uint8(i), true
			}
			return i, true
		}
		i++
		if i >= hashCap {
			i = 0
		}
		if i == start {
			return 0, false
		}
	}
}

func (c *Context) hashInsertSlot(data []byte, key Value) (int, bool) {
	hashCap := int(readU16(data, 6))
	if hashCap == 0 {
		return 0, false
	}
	arrayCap := readU16(data, 2)
	hashCapU := readU16(data, 6)
	start := hashBucket(c.hashValue(key), uint16(hashCap))
	i := start
	tomb := -1
	for {
		k := readValue(data, hashKeyOff(arrayCap, i))
		if k.Kind == KindNil {
			v := readValue(data, hashValOff(arrayCap, hashCapU, i))
			if isTombstone(k, v) {
				if tomb < 0 {
					tomb = i
				}
			} else {
				if tomb >= 0 {
					return tomb, true
				}
				return i, true
			}
		} else if c.ValuesEqual(k, key) {
			return i, true
		}
		i++
		if i >= hashCap {
			i = 0
		}
		if i == start {
			if tomb >= 0 {
				return tomb, true
			}
			return 0, false
		}
	}
}

func (c *Context) TableAdd(t, val, index Value) HeapError {
	data, err := c.tableData(t)
	if err != HeapOK {
		return err
	}
	arrayCap := int(readU16(data, 2))
	n := tableArrayLen(data, arrayCap)
	at := n + 1
	if index.Kind == KindNumber {
		i := index.Int()
		if i >= 1 && i <= n+1 {
			at = i
		}
	}
	for n >= arrayCap {
		// Indexed insertion needs space for the shifted last element too.
		// Growth can promote integer hash keys and extend the dense prefix,
		// so recheck after rebuilding before writing any shifted elements.
		newArray, ok := growCap(arrayCap, n+1, maxArrayCap)
		if !ok {
			return HeapOOM
		}
		hashCap := int(readU16(data, 6))
		if err := c.growTableWithRoots(t, newArray, hashCap, val); err != HeapOK {
			return err
		}
		data, err = c.tableData(t)
		if err != HeapOK {
			return err
		}
		arrayCap = int(readU16(data, 2))
		n = tableArrayLen(data, arrayCap)
	}
	for i := n; i >= at; i-- {
		writeValue(data, arraySlotOff(i), readValue(data, arraySlotOff(i-1)))
	}
	writeValue(data, arraySlotOff(at-1), val)
	return HeapOK
}

func growCap(cur, need, max int) (int, bool) {
	if need > max {
		return 0, false
	}
	n := cur
	if n < 1 {
		n = 1
	}
	for n < need {
		next := n * 2
		if next > max || next <= n {
			n = max
			break
		}
		n = next
	}
	if n < need {
		return 0, false
	}
	return n, true
}

// growTableWithRoots keeps values that have not yet been written into the
// table alive while growTable allocates. The allocator may collect during
// that allocation, and Go locals are not runtime GC roots by themselves.
func (c *Context) growTableWithRoots(t Value, newArray, newHash int, values ...Value) HeapError {
	rooted := 0
	for _, v := range values {
		if !c.PushLocal(v) {
			c.PopLocals(rooted)
			return HeapOOM
		}
		rooted++
	}
	err := c.growTable(t, newArray, newHash)
	c.PopLocals(rooted)
	return err
}

// growTable reallocates table t to newArray/newHash while keeping t.Ref.
// Callers pass t by value, so a new handle would be invisible; the old
// handle adopts the new storage and the temporary handle is freed.
func (c *Context) growTable(t Value, newArray, newHash int) HeapError {
	if newArray < 0 || newArray > maxArrayCap || newHash < 0 || newHash > maxHashCap {
		return HeapOOM
	}
	old, err := c.tableData(t)
	if err != HeapOK {
		return err
	}
	oldArray := int(readU16(old, 2))
	oldHash := int(readU16(old, 6))
	if newArray < oldArray {
		newArray = oldArray
	}
	if newHash < oldHash {
		newHash = oldHash
	}
	if newArray == oldArray && newHash == oldHash {
		return HeapOOM
	}
	if !c.PushLocal(t) {
		return HeapOOM
	}
	size := tableDataSize(uint16(newArray), uint16(newHash))
	freshRef, err := c.heap.allocObject(ObjTable, size)
	if err != HeapOK {
		c.PopLocals(1)
		return err
	}
	old, err = c.tableData(t)
	if err != HeapOK {
		return c.dropGrowHandle(freshRef, err)
	}
	fresh, err := c.objectBytes(freshRef)
	if err != HeapOK {
		return c.dropGrowHandle(freshRef, err)
	}
	writeU16(fresh, 0, 0)
	writeU16(fresh, 2, uint16(newArray))
	writeU16(fresh, 4, 0)
	writeU16(fresh, 6, uint16(newHash))
	copyN := oldArray
	if copyN > newArray {
		copyN = newArray
	}
	for i := 0; i < copyN; i++ {
		writeValue(fresh, arraySlotOff(i), readValue(old, arraySlotOff(i)))
	}
	hashLen := uint16(0)
	oldArrayU := uint16(oldArray)
	oldHashU := uint16(oldHash)
	for i := 0; i < oldHash; i++ {
		k := readValue(old, hashKeyOff(oldArrayU, i))
		v := readValue(old, hashValOff(oldArrayU, oldHashU, i))
		if k.Kind == KindNil {
			continue
		}
		if n, ok := integerKey(k); ok && n >= 1 && n <= newArray {
			writeValue(fresh, arraySlotOff(n-1), v)
			continue
		}
		slot, ok := c.hashInsertSlot(fresh, k)
		if !ok {
			return c.dropGrowHandle(freshRef, HeapOOM)
		}
		freshArray := uint16(newArray)
		freshHash := uint16(newHash)
		writeValue(fresh, hashKeyOff(freshArray, slot), k)
		writeValue(fresh, hashValOff(freshArray, freshHash, slot), v)
		hashLen++
	}
	writeU16(fresh, 4, hashLen)
	c.adoptTableStorage(t.Ref, freshRef)
	c.PopLocals(1)
	return HeapOK
}

func (c *Context) dropGrowHandle(freshRef Ref, err HeapError) HeapError {
	c.abandonHandle(freshRef)
	c.PopLocals(1)
	return err
}

func (c *Context) abandonHandle(ref Ref) {
	hdr := &c.heap.handles[ref]
	hdr.SetState(HandleFree)
	hdr.Size = 0
	hdr.Offset = uint32(c.heap.freeHead)
	c.heap.freeHead = ref
	if c.heap.handleN > 0 {
		c.heap.handleN--
	}
}

func (c *Context) adoptTableStorage(keep, drop Ref) {
	c.heap.handles[keep].Offset = c.heap.handles[drop].Offset
	c.heap.handles[keep].Size = c.heap.handles[drop].Size
	c.abandonHandle(drop)
}

func (c *Context) TableDel(t, val Value) Value {
	data, err := c.tableData(t)
	if err != HeapOK {
		return NilVal()
	}
	arrayCap := int(readU16(data, 2))
	n := tableArrayLen(data, arrayCap)
	for i := 0; i < n; i++ {
		got := readValue(data, arraySlotOff(i))
		if c.ValuesEqual(got, val) {
			for j := i; j < n-1; j++ {
				writeValue(data, arraySlotOff(j), readValue(data, arraySlotOff(j+1)))
			}
			if n-1 < arrayCap {
				writeValue(data, arraySlotOff(n-1), NilVal())
			}
			return got
		}
	}
	hashCap := int(readU16(data, 6))
	arrayCapU := readU16(data, 2)
	hashCapU := readU16(data, 6)
	hashLen := readU16(data, 4)
	for i := 0; i < hashCap; i++ {
		k := readValue(data, hashKeyOff(arrayCapU, i))
		v := readValue(data, hashValOff(arrayCapU, hashCapU, i))
		if k.Kind == KindNil {
			continue
		}
		if c.ValuesEqual(v, val) {
			writeValue(data, hashKeyOff(arrayCapU, i), NilVal())
			writeValue(data, hashValOff(arrayCapU, hashCapU, i), BoolVal(true))
			if hashLen > 0 {
				writeU16(data, 4, hashLen-1)
			}
			return v
		}
	}
	return NilVal()
}

func (c *Context) TableDeli(t, index Value) Value {
	data, err := c.tableData(t)
	if err != HeapOK {
		return NilVal()
	}
	n := tableArrayLen(data, int(readU16(data, 2)))
	at := n
	if index.Kind == KindNumber {
		at = index.Int()
	}
	if at < 1 || at > n {
		return NilVal()
	}
	got := readValue(data, arraySlotOff(at-1))
	for i := at - 1; i < n-1; i++ {
		writeValue(data, arraySlotOff(i), readValue(data, arraySlotOff(i+1)))
	}
	writeValue(data, arraySlotOff(n-1), NilVal())
	return got
}

func (c *Context) TableCount(t, val Value) int {
	if val.Kind == KindNil {
		return c.TableLen(t)
	}
	data, err := c.tableData(t)
	if err != HeapOK {
		return 0
	}
	n := 0
	arrayCap := int(readU16(data, 2))
	for i := 0; i < arrayCap; i++ {
		got := readValue(data, arraySlotOff(i))
		if got.Kind != KindNil && c.ValuesEqual(got, val) {
			n++
		}
	}
	hashCap := int(readU16(data, 6))
	arrayCapU := readU16(data, 2)
	hashCapU := readU16(data, 6)
	for i := 0; i < hashCap; i++ {
		k := readValue(data, hashKeyOff(arrayCapU, i))
		v := readValue(data, hashValOff(arrayCapU, hashCapU, i))
		if k.Kind == KindNil {
			continue
		}
		if c.ValuesEqual(v, val) {
			n++
		}
	}
	return n
}

func (c *Context) TableSlotCount(t Value) int {
	data, err := c.tableData(t)
	if err != HeapOK {
		return 0
	}
	return int(readU16(data, 2)) + int(readU16(data, 6))
}

func (c *Context) TableKeyAt(t Value, idx int) Value {
	data, err := c.tableData(t)
	if err != HeapOK {
		return NilVal()
	}
	arrayCap := int(readU16(data, 2))
	if idx < 0 {
		return NilVal()
	}
	if idx < arrayCap {
		v := readValue(data, arraySlotOff(idx))
		if v.Kind == KindNil {
			return NilVal()
		}
		return IntVal(idx + 1)
	}
	hidx := idx - arrayCap
	hashCap := int(readU16(data, 6))
	if hidx < 0 || hidx >= hashCap {
		return NilVal()
	}
	return readValue(data, hashKeyOff(readU16(data, 2), hidx))
}

func (c *Context) TableValueAt(t Value, idx int) Value {
	data, err := c.tableData(t)
	if err != HeapOK {
		return NilVal()
	}
	arrayCap := int(readU16(data, 2))
	if idx < 0 {
		return NilVal()
	}
	if idx < arrayCap {
		return readValue(data, arraySlotOff(idx))
	}
	hidx := idx - arrayCap
	hashCap := int(readU16(data, 6))
	if hidx < 0 || hidx >= hashCap {
		return NilVal()
	}
	return readValue(data, hashValOff(readU16(data, 2), readU16(data, 6), hidx))
}
