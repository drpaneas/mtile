package runtime

// TableLiteralImageField is one planned hash slot in a ROM constructor image.
// HasValue false keeps the key occupied and the value nil so a later
// StorePlanned overlay can fill a dynamic field without changing hash layout.
type TableLiteralImageField struct {
	Slot     int
	KeyID    uint16
	Value    Value
	HasValue bool
}

// BuildTableLiteralImage writes the ordinary table header, unique static keys,
// and any constant values using the same little-endian Value layout as
// TableSet. Hash length equals the number of planned fields, including
// overlays whose values are still nil. Callers must pass the exact capacities
// and open-addressed slots that ordinary insertion would have produced.
func BuildTableLiteralImage(arrayCap, hashCap int, fields []TableLiteralImageField) []byte {
	if arrayCap < 0 || hashCap < 0 || arrayCap > maxArrayCap || hashCap > maxHashCap {
		return nil
	}
	size := tableDataSize(uint16(arrayCap), uint16(hashCap))
	if size == 0 {
		return nil
	}
	data := make([]byte, size)
	writeU16(data, 2, uint16(arrayCap))
	writeU16(data, 6, uint16(hashCap))
	writeU16(data, 4, uint16(len(fields)))
	for _, field := range fields {
		if field.Slot < 0 || field.Slot >= hashCap {
			return nil
		}
		writeValue(data, hashKeyOff(uint16(arrayCap), field.Slot), Value{Kind: KindStaticString, SRef: field.KeyID})
		if field.HasValue {
			writeValue(data, hashValOff(uint16(arrayCap), uint16(hashCap), field.Slot), field.Value)
		}
	}
	return data
}

// NewTableFromImage allocates a table and copies a complete constructor image.
// The image must match tableDataSize for its header capacities. A malformed
// image fails without setting a terminal heap error: generated constructors
// already treat any non-OK result as a nil table.
func (c *Context) NewTableFromImage(image []byte) (Value, HeapError) {
	if len(image) < 8 {
		return NilVal(), HeapCorrupt
	}
	arrayCap := int(readU16(image, 2))
	hashCap := int(readU16(image, 6))
	size := int(tableDataSize(uint16(arrayCap), uint16(hashCap)))
	if size == 0 || size != len(image) || int(readU16(image, 0)) > arrayCap || int(readU16(image, 4)) > hashCap {
		return NilVal(), HeapCorrupt
	}
	c.ensureHeap()
	ref, err := c.heap.allocObjectFilled(ObjTable, image)
	if err != HeapOK {
		return NilVal(), err
	}
	return Value{Kind: KindTable, Ref: ref}, HeapOK
}

// StorePlanned writes a non-nil value onto a constructor slot whose static
// key the image already placed. Unexpected occupancy, caps, or nil values
// invalidate the borrow and use TableSet, matching InitKnown's fallback.
//
//go:noinline
func (v *TableLiteralView) StorePlanned(keyID uint16, value Value, plan uint32) HeapError {
	if v.err != HeapOK {
		return v.err
	}
	if v.c.heap.terminal != HeapOK {
		v.data = nil
		return v.c.heap.terminal
	}
	key := Value{Kind: KindStaticString, SRef: keyID}
	data := v.data
	arrayCap := uint16(plan & 511)
	hashCap := uint16((plan >> 9) & 255)
	slot := int((plan >> 17) & 127)
	if data != nil && value.Kind != KindNil && keyID < v.c.staticN &&
		arrayCap == readU16(data, 2) && hashCap == readU16(data, 6) && slot < int(hashCap) {
		keyOff := hashKeyOff(arrayCap, slot)
		existing := readValue(data, keyOff)
		if existing.Kind == KindStaticString && existing.SRef == keyID {
			writeValue(data, hashValOff(arrayCap, hashCap, slot), value)
			return HeapOK
		}
	}
	v.data = nil
	return v.c.TableSet(v.table, key, value)
}

// StorePlannedNumber writes a constructor overlay without boxing the number as
// a Lua Value at the call site. A Number is never nil, so the success path
// matches StorePlanned(NumVal(n)) and the fallback still uses TableSet.
//
//go:noinline
func (v *TableLiteralView) StorePlannedNumber(keyID uint16, n Number, plan uint32) HeapError {
	if v.err != HeapOK {
		return v.err
	}
	if v.c.heap.terminal != HeapOK {
		v.data = nil
		return v.c.heap.terminal
	}
	data := v.data
	arrayCap := uint16(plan & 511)
	hashCap := uint16((plan >> 9) & 255)
	slot := int((plan >> 17) & 127)
	if data != nil && keyID < v.c.staticN &&
		arrayCap == readU16(data, 2) && hashCap == readU16(data, 6) && slot < int(hashCap) {
		keyOff := hashKeyOff(arrayCap, slot)
		existing := readValue(data, keyOff)
		if existing.Kind == KindStaticString && existing.SRef == keyID {
			writeValue(data, hashValOff(arrayCap, hashCap, slot), Value{Kind: KindNumber, Num: n})
			return HeapOK
		}
	}
	v.data = nil
	return v.c.TableSet(v.table, Value{Kind: KindStaticString, SRef: keyID}, NumVal(n))
}
