package runtime

import "unsafe"

// BorrowFreshTableLiteral is an internal compiler contract, stronger than
// BorrowTableLiteral: t is the just-allocated, unexposed, zeroed constructor
// object. All fields have unique static keys, non-nil values, and exact slot
// plans for these capacities. Value evaluation cannot allocate or collect.
// Validate the complete layout once; rejected borrows retain ordinary stores.
// Like BorrowTableLiteral, errors are returned only after evaluating a field.
func (c *Context) BorrowFreshTableLiteral(t Value, arrayCap, hashCap int) TableLiteralView {
	v := c.BorrowTableLiteral(t)
	if v.err != HeapOK {
		return v
	}
	data := v.data
	if !littleEndianNumberWords || uintptr(unsafe.Pointer(&data[0]))&3 != 0 ||
		arrayCap != int(readU16(data, 2)) || hashCap != int(readU16(data, 6)) ||
		readU16(data, 0) != 0 || readU16(data, 4) != 0 {
		v.data = nil
	}
	return v
}

// InitKnown requires the complete fresh-constructor proof above. In
// particular, plan must name an in-bounds slot in the borrowed layout and
// inserts must follow the planned order without intervening table mutation.
// This leaf deliberately inlines: the compiler's constant plan becomes
// constant byte offsets, and constant Value kinds remove the nil guard.
// Unexpected nil/static-intern failures invalidate the borrow before the
// generic operation, which may allocate. No pointer then survives a fallback.
func (v *TableLiteralView) InitKnown(keyID uint16, value Value, plan uint32) HeapError {
	if v.data == nil || value.Kind == KindNil || keyID >= v.c.staticN {
		v.data = nil
		return v.InitStatic(keyID, value, plan)
	}
	base := unsafe.Pointer(&v.data[0])
	arrayCap := uint16(plan & 511)
	hashCap := uint16((plan >> 9) & 255)
	slot := int((plan >> 17) & 127)
	*(*Value)(unsafe.Add(base, hashKeyOff(arrayCap, slot))) = Value{Kind: KindStaticString, SRef: keyID}
	*(*Value)(unsafe.Add(base, hashValOff(arrayCap, hashCap, slot))) = value
	*(*uint16)(unsafe.Add(base, 4)) = uint16(plan>>24) + 1
	return HeapOK
}
