package runtime

// TableInitStatic is the generated constructor path for an as-yet unexposed
// table. The compiler has proved a unique static key and its exact ordinary
// open-addressed slot if all preceding planned fields were non-nil. A nil
// value or a lower hash count takes the ordinary TableSet path instead.
// This keeps the normal table layout (and pairs order) without hashing each
// key twice. It is not a replacement for stores to existing Lua tables.
//
// plan packs array capacity in bits 0..8, hash capacity in 9..16, slot in
// 17..23, and the preceding hash length in 24..31. Capacities use the same
// limits as NewTableWithCaps; a constructor can precede at most 127 inserts.
func (c *Context) TableInitStatic(t Value, keyID uint16, value Value, plan uint32) HeapError {
	view := c.BorrowTableLiteral(t)
	return view.InitStatic(keyID, value, plan)
}

// TableLiteralView is an internal compiler borrow of a newly allocated table.
// It may not survive allocation/collection or any change to this table's
// identity, metadata, or fields except its own InitStatic calls. Generated
// code roots the table before borrowing and proves all values non-allocating.
// It retains no pointers after any generic fallback, which may grow/collect.
type TableLiteralView struct {
	c     *Context
	table Value
	data  []byte
	err   HeapError
}

// BorrowTableLiteral performs the ordinary checked validation once. A failed
// borrow is reported by InitStatic, after the field's value has evaluated in
// its original position. Borrowing itself has no allocation or error effects.
func (c *Context) BorrowTableLiteral(t Value) TableLiteralView {
	data, err := c.tableData(t)
	return TableLiteralView{c: c, table: t, data: data, err: err}
}

// InitStatic keeps the exact TableInitStatic guards and ordinary insertion
// layout while reusing the validated table bytes across a complete literal.
// Invalidate before TableSet: malformed plans/occupied slots can force growth,
// and a nil value makes every later planned insertion's count unreliable.
//
//go:noinline
//go:section .iwram
func (v *TableLiteralView) InitStatic(keyID uint16, value Value, plan uint32) HeapError {
	if v.err != HeapOK {
		return v.err
	}
	key := Value{Kind: KindStaticString, SRef: keyID}
	data := v.data
	arrayCap := uint16(plan & 511)
	hashCap := uint16((plan >> 9) & 255)
	slot := int((plan >> 17) & 127)
	previous := uint16(plan >> 24)
	if data != nil && value.Kind != KindNil && keyID < v.c.staticN &&
		arrayCap == readU16(data, 2) && hashCap == readU16(data, 6) &&
		previous == readU16(data, 4) && slot < int(hashCap) && previous < hashCap {
		keyOff, valOff := hashKeyOff(arrayCap, slot), hashValOff(arrayCap, hashCap, slot)
		if readValue(data, keyOff).Kind == KindNil && readValue(data, valOff).Kind == KindNil {
			writeValue(data, keyOff, key)
			writeValue(data, valOff, value)
			writeU16(data, 4, previous+1)
			return HeapOK
		}
	}
	v.data = nil
	return v.c.TableSet(v.table, key, value)
}
