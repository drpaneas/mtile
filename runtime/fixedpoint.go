package runtime

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
	return Number((int64(a) << 16) / int64(b))
}

// Flr rounds toward negative infinity. Go's signed right shift has exactly
// the required behavior for this fixed-point representation.
func (n Number) Flr() Number {
	return Number((int64(n) >> 16) << 16)
}

func (a Number) IDiv(b Number) Number {
	return a.Div(b).Flr()
}
