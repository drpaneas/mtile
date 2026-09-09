package runtime

import "math/bits"

// Number is 16:16 fixed point. Arithmetic deliberately converts
// back to int32 so overflow wraps like the target's two's-complement values.
type Number int32

const One Number = 1 << 16

func FromInt(n int) Number {
	return Number(int64(n) << 16)
}

func (n Number) Int() int {
	return int(n >> 16)
}

func (a Number) Mul(b Number) Number {
	return Number((int64(a) * int64(b)) >> 16)
}

func (a Number) Div(b Number) Number {
	if b == 0 {
		if a < 0 {
			return -Number(0x7FFFFFFF)
		}
		return Number(0x7FFFFFFF)
	}
	ub := uint32(b)
	if b < 0 && b != Number(-1<<31) {
		ub = uint32(-b)
	}
	// Power-of-two encodings (integer 2,4,8,... and fractions 0.5,0.25,...)
	// are a shift. Integer non-pow2 (30, 60, 7, ...) is 32-bit idiv, so a
	// leftover runtime divisor uses the 32-iter IWRAM loop instead of
	// 64-bit ldivmod. +/-1.5 is UMULL. Compile-time integer literals are
	// lowered separately so LLVM can emit a magic multiply at the call site.
	if ub&(ub-1) == 0 {
		q := divPow2Raw(a, bits.TrailingZeros32(ub))
		if b < 0 {
			return -q
		}
		return q
	}
	if ub&0xFFFF == 0 {
		return Number(int32(a) / int32(b>>16))
	}
	if ub == 98304 {
		q := divThreeHalves(a)
		if b < 0 {
			return -q
		}
		return q
	}
	return Number((int64(a) << 16) / int64(b))
}

// Mod is Lua remainder: the result takes the sign of the divisor.
func (a Number) Mod(z Number) Number {
	if z == 0 {
		return 0
	}
	if z > 0 {
		uz := uint32(z)
		if uz&(uz-1) == 0 {
			shift := bits.TrailingZeros32(uz)
			return a - ((a >> shift) << shift)
		}
	}
	r := a % z
	if r != 0 && ((r < 0) != (z < 0)) {
		r += z
	}
	return r
}

func divThreeHalves(n Number) Number {
	magnitude := uint32(n)
	if n < 0 {
		magnitude = 0 - magnitude
	}
	quotient := Number(uint32((uint64(magnitude) * 0xaaaaaaab) >> 32))
	if n < 0 {
		return -quotient
	}
	return quotient
}

func divPow2Raw(a Number, tz int) Number {
	if tz >= 16 {
		return towardZeroShift(a, tz-16)
	}
	return a << (16 - tz)
}

func towardZeroShift(n Number, shift int) Number {
	if shift <= 0 {
		return n
	}
	if n >= 0 {
		return n >> shift
	}
	return (n + Number((uint32(1)<<shift)-1)) >> shift
}

// Flr rounds toward negative infinity. Go's signed right shift has exactly
// the required behavior for this fixed-point representation.
func (n Number) Flr() Number {
	return Number((int64(n) >> 16) << 16)
}

func (a Number) IDiv(b Number) Number {
	return a.Div(b).Flr()
}
