package runtime

// TableAppendKnown is an internal compiler path for an unexposed fresh list
// filled by one fixed-count append loop, with its final capacity reserved.
// The compiler proves that earlier slots form an untouched dense prefix; at
// is the one-based attempt number, not a Lua insertion index. Other tables
// must use TableAdd, since a single predecessor cannot prove an arbitrary
// table's complete dense prefix.
//
// Resolve the moving handle after each RHS has evaluated. No pointer survives
// a call, allocation or collection. A failed/nil earlier append leaves the
// predecessor empty, so all later attempts use the ordinary append position.
// Keep the same checked validation, and fall back for unexpected bounds or
// occupied slots before retaining any borrowed storage across possible growth.
func (c *Context) TableAppendKnown(t, value Value, at int) HeapError {
	data, err := c.tableData(t)
	if err != HeapOK {
		return err
	}
	capacity := int(readU16(data, 2))
	if value.Kind != KindNil && at >= 1 && at <= capacity &&
		data[arraySlotOff(at-1)] == byte(KindNil) &&
		(at == 1 || data[arraySlotOff(at-2)] != byte(KindNil)) {
		writeValue(data, arraySlotOff(at-1), value)
		return HeapOK
	}
	return c.TableAdd(t, value, NilVal())
}
