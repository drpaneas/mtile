package runtime

import "unsafe"

type Kind uint8

const (
	KindNil Kind = iota
	KindBool
	KindNumber
	KindString
	KindTable
	KindStaticString
)

type Ref uint16

const invalidRef Ref = 0xFFFF

type Value struct {
	Kind Kind
	Num  Number
	Ref  Ref
	SRef uint16
}

const (
	valueBytes = 12
	// MaxStaticStrings and MaxStringBytes are exported so the transpiler can
	// reject cartridges which cannot be represented by the fixed runtime
	// tables instead of letting InternStatic/NewString fail silently.
	MaxStaticStrings      = 256
	MaxStringBytes        = 4096
	MaxTableArrayCapacity = 256
	MaxTableHashCapacity  = 128
	maxArrayCap           = MaxTableArrayCapacity
	maxHashCap            = MaxTableHashCapacity
)

type StaticString struct {
	Ptr unsafe.Pointer
	Len uint16
	// A checked lookup hint occupies existing alignment padding. It is only
	// accepted when the current table contains this exact static ID there.
	slotHint uint8
	hintSet  bool
	// Static strings never change; reuse their exact FNV hash for table probes.
	// This preserves the hash layout and pairs order of heap-backed strings.
	hash uint32
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(Value{})-valueBytes]
)

func IntVal(n int) Value {
	return Value{Kind: KindNumber, Num: FromInt(n)}
}

func NumVal(n Number) Value {
	return Value{Kind: KindNumber, Num: n}
}

func BoolVal(b bool) Value {
	v := Value{Kind: KindBool}
	if b {
		v.Num = 1
	}
	return v
}

func NilVal() Value {
	return Value{}
}

func (v Value) Truthy() bool {
	if v.Kind == KindNil {
		return false
	}
	if v.Kind == KindBool {
		return v.Num != 0
	}
	return true
}

func (v Value) IsTrue() bool {
	return v.Truthy()
}

func (v Value) Int() int {
	if v.Kind != KindNumber {
		return 0
	}
	return v.Num.Int()
}

func (v Value) IntOr(def int) int {
	if v.Kind != KindNumber {
		return def
	}
	return v.Num.Int()
}

func (v Value) NumberOr(def Number) Number {
	if v.Kind != KindNumber {
		return def
	}
	return v.Num
}

func isStringKind(k Kind) bool {
	return k == KindString || k == KindStaticString
}

func (a Value) Add(b Value) Value {
	if a.Kind != KindNumber || b.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(a.Num + b.Num)
}

func (a Value) Sub(b Value) Value {
	if a.Kind != KindNumber || b.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(a.Num - b.Num)
}

func (a Value) Mul(b Value) Value {
	if a.Kind != KindNumber || b.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(a.Num.Mul(b.Num))
}

func (a Value) Div(b Value) Value {
	if a.Kind != KindNumber || b.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(a.Num.Div(b.Num))
}

func (a Value) IDiv(b Value) Value {
	if a.Kind != KindNumber || b.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(a.Num.IDiv(b.Num))
}

func (v Value) Flr() Value {
	if v.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(v.Num.Flr())
}

func (v Value) Ceil() Value {
	if v.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(v.Num.Ceil())
}

func (v Value) Abs() Value {
	if v.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(v.Num.Abs())
}

func (v Value) Sgn() Value {
	if v.Kind != KindNumber {
		return NilVal()
	}
	return NumVal(v.Num.Sgn())
}

func (c *Context) InternStatic(s string) Value {
	if len(s) > MaxStringBytes {
		return NilVal()
	}
	for i := uint16(0); i < c.staticN; i++ {
		if c.staticHold[i] == s {
			return Value{Kind: KindStaticString, SRef: i}
		}
	}
	if int(c.staticN) >= len(c.staticTab) {
		return NilVal()
	}
	i := c.staticN
	c.staticHold[i] = s
	c.staticTab[i].Len = uint16(len(s))
	if len(s) > 0 {
		c.staticTab[i].Ptr = unsafe.Pointer(unsafe.StringData(s))
	}
	c.staticTab[i].hash = hashBytes(unsafe.Slice((*byte)(c.staticTab[i].Ptr), len(s)))
	c.staticN++
	return Value{Kind: KindStaticString, SRef: i}
}

func (c *Context) stringBytes(v Value) []byte {
	switch v.Kind {
	case KindStaticString:
		return c.staticStringBytes(v)
	case KindString:
		return c.heapStringBytes(v.Ref)
	default:
		return nil
	}
}

// staticStringBytes deliberately has no dynamic-string branch. Generated
// literal-only paths call it directly so TinyGo does not retain the Lua heap
// merely because the generic string decoder can also read KindString values.
func (c *Context) staticStringBytes(v Value) []byte {
	if v.Kind != KindStaticString || int(v.SRef) >= int(c.staticN) {
		return nil
	}
	e := c.staticTab[v.SRef]
	if e.Len == 0 || e.Ptr == nil {
		return nil
	}
	return unsafe.Slice((*byte)(e.Ptr), int(e.Len))
}

func (c *Context) stringLen(v Value) int {
	switch v.Kind {
	case KindStaticString:
		if int(v.SRef) >= int(c.staticN) {
			return 0
		}
		return int(c.staticTab[v.SRef].Len)
	case KindString:
		b := c.heapStringBytes(v.Ref)
		return len(b)
	default:
		return 0
	}
}

func (c *Context) ValuesEqual(a, b Value) bool {
	// InternStatic is the sole writer of the static registry and deduplicates
	// by contents. Valid static IDs therefore already represent string equality;
	// decoding and comparing their ROM bytes again costs nothing useful.
	// Retain the bytewise fallback for invalid refs and mixed storage kinds.
	if a.Kind == KindStaticString && b.Kind == KindStaticString &&
		a.SRef < c.staticN && b.SRef < c.staticN {
		return a.SRef == b.SRef
	}
	if isStringKind(a.Kind) && isStringKind(b.Kind) {
		return bytesEqual(c.stringBytes(a), c.stringBytes(b))
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindNil:
		return true
	case KindBool, KindNumber:
		return a.Num == b.Num
	case KindTable:
		return a.Ref == b.Ref
	default:
		return false
	}
}

// CompareStrings performs bytewise lexicographic ordering for both ROM-backed
// static strings and heap strings. The boolean is false for non-string values.
func (c *Context) CompareStrings(a, b Value) (int, bool) {
	if !isStringKind(a.Kind) || !isStringKind(b.Kind) {
		return 0, false
	}
	left, right := c.stringBytes(a), c.stringBytes(b)
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		if left[i] < right[i] {
			return -1, true
		}
		if left[i] > right[i] {
			return 1, true
		}
	}
	if len(left) < len(right) {
		return -1, true
	}
	if len(left) > len(right) {
		return 1, true
	}
	return 0, true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *Context) hashValue(v Value) uint32 {
	if v.Kind == KindStaticString && v.SRef < c.staticN {
		return c.staticTab[v.SRef].hash
	}
	if isStringKind(v.Kind) {
		return hashBytes(c.stringBytes(v))
	}
	switch v.Kind {
	case KindNumber:
		// Table capacities are small powers of two. The low 16 bits of an
		// integer Number are zero, so hashing the raw payload sends every
		// integer outside the array part to the same bucket. Fold in the
		// integer bits while retaining the fractional bits for nonintegers.
		raw := uint32(v.Num)
		return raw ^ (raw >> 16)
	case KindBool:
		if v.Num != 0 {
			return 2
		}
		return 1
	case KindTable:
		return uint32(v.Ref) ^ 0x9E3779B9
	default:
		return 0
	}
}

func hashBytes(b []byte) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(b); i++ {
		h ^= uint32(b[i])
		h *= 16777619
	}
	return h
}
