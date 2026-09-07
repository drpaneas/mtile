package runtime

import "unsafe"

// NumberArrayView is a compiler-internal borrow of a fixed numeric array.
// It must not survive a collection, an array binding change, or a change to
// heap/object metadata. The transpiler only borrows across a region proved
// free of allocation/collection and such changes. Element writes are allowed.
// This is not a Lua API and is not a replacement for the checked general
// accessors when the compiler cannot establish that lifetime.
type NumberArrayView struct {
	words []Number
	data  []byte
	err   HeapError
}

// BorrowNumberArray checks the complete object exactly as NumberArrayGet/Set
// do. It performs no allocation and does not change heap error state. Borrow
// only after evaluating loop bounds, which may themselves allocate or collect.
func (c *Context) BorrowNumberArray(array Value) NumberArrayView {
	data, length, err := c.numberArrayData(array)
	if err != HeapOK {
		return NumberArrayView{err: err}
	}
	data = data[numberArrayHeader : numberArrayHeader+length*4]
	if length != 0 && littleEndianNumberWords && uintptr(unsafe.Pointer(&data[0]))&3 == 0 {
		return NumberArrayView{words: unsafe.Slice((*Number)(unsafe.Pointer(&data[0])), length)}
	}
	// Keep the old byte representation for unusual unaligned headers and
	// big-endian hosts; real GBA heap allocations use the aligned word path.
	return NumberArrayView{data: data}
}

func (v *NumberArrayView) Get(index int) Number {
	index--
	if v.words != nil {
		if uint(index) < uint(len(v.words)) {
			return v.words[index]
		}
		return 0
	}
	if uint(index) >= uint(len(v.data)/4) {
		return 0
	}
	offset := index * 4
	raw := uint32(v.data[offset]) | uint32(v.data[offset+1])<<8 |
		uint32(v.data[offset+2])<<16 | uint32(v.data[offset+3])<<24
	return Number(int32(raw))
}

func (v *NumberArrayView) Set(index int, number Number) HeapError {
	if v.err != HeapOK {
		return v.err
	}
	index--
	if v.words != nil {
		if uint(index) < uint(len(v.words)) {
			v.words[index] = number
		}
		return HeapOK
	}
	if uint(index) >= uint(len(v.data)/4) {
		return HeapOK
	}
	offset := index * 4
	raw := uint32(number)
	v.data[offset], v.data[offset+1] = byte(raw), byte(raw>>8)
	v.data[offset+2], v.data[offset+3] = byte(raw>>16), byte(raw>>24)
	return HeapOK
}
