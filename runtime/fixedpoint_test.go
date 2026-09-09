package runtime

import "testing"

func TestNumberArithmetic(t *testing.T) {
	if got := FromInt(3).Mul(FromInt(2)); got != FromInt(6) {
		t.Fatalf("3*2 = %v, want %v", got, FromInt(6))
	}
	if got := FromInt(3).Div(FromInt(2)); got != Number(3<<15) {
		t.Fatalf("3/2 = %v, want %v", got, Number(3<<15))
	}
	if got := Number(-3 * int(One) / 2).Flr(); got != FromInt(-2) {
		t.Fatalf("floor(-1.5) = %v, want -2", got)
	}
	if got := FromInt(-3).IDiv(FromInt(2)); got != FromInt(-2) {
		t.Fatalf("-3\\2 = %v, want -2", got)
	}
}

func TestNumberDivisionByZero(t *testing.T) {
	if got := Number(0).Div(0); got != Number(0x7FFFFFFF) {
		t.Fatalf("0/0 = %#x, want max positive", got)
	}
	if got := FromInt(-1).Div(0); got != -Number(0x7FFFFFFF) {
		t.Fatalf("negative/0 = %#x, want max negative", got)
	}
	if got := FromInt(1).Div(0); got != Number(0x7FFFFFFF) {
		t.Fatalf("positive/0 = %#x, want max positive", got)
	}
}

func TestNumberWrapsInt32(t *testing.T) {
	if got := FromInt(32768); got != Number(-2147483648) {
		t.Fatalf("32768 fixed value = %#x, want int32 wrap", got)
	}
}

func TestFromFloatHost(t *testing.T) {
	if got := FromFloat(1.5); got != Number(3<<15) {
		t.Fatalf("1.5 = %#x, want %#x", got, Number(3<<15))
	}
}

func wideningDiv(a, b Number) Number {
	if b == 0 {
		if a < 0 {
			return -Number(0x7FFFFFFF)
		}
		return Number(0x7FFFFFFF)
	}
	return Number((int64(a) << 16) / int64(b))
}

func luaMod(a, z Number) Number {
	if z == 0 {
		return 0
	}
	r := a % z
	if r != 0 && ((r < 0) != (z < 0)) {
		r += z
	}
	return r
}

func TestNumberDivMatchesWidening(t *testing.T) {
	check := func(a, b Number) {
		t.Helper()
		if got, want := a.Div(b), wideningDiv(a, b); got != want {
			t.Fatalf("%d/%d got %d want %d", a, b, got, want)
		}
	}
	divisors := []Number{
		FromInt(1), FromInt(-1), FromInt(2), FromInt(-2), FromInt(7), FromInt(-7),
		FromInt(8), FromInt(30), FromInt(60), FromInt(240), FromInt(-30),
		FromFloat(0.5), FromFloat(-0.5), FromFloat(1.5), FromFloat(-1.5),
		FromFloat(8.00002), Number(1), Number(-1), Number(-1 << 31),
	}
	numerators := []Number{0, 1, -1, FromInt(16), FromInt(-16), FromInt(32767), FromInt(-32767), Number(-1 << 31), Number(0x7FFFFFFF)}
	for _, a := range numerators {
		for _, b := range divisors {
			check(a, b)
		}
	}
	seed := uint32(0x87654321)
	for i := 0; i < 200000; i++ {
		seed = seed*1664525 + 1013904223
		a := Number(seed)
		seed = seed*1664525 + 1013904223
		b := Number(seed)
		check(a, b)
		check(a, FromInt(30))
		check(a, FromInt(8))
		check(a, FromFloat(1.5))
	}
}

func TestNumberModMatchesLua(t *testing.T) {
	check := func(a, z Number) {
		t.Helper()
		if got, want := a.Mod(z), luaMod(a, z); got != want {
			t.Fatalf("%d%%%d got %d want %d", a, z, got, want)
		}
	}
	moduli := []Number{
		0, FromInt(1), FromInt(-1), FromInt(2), FromInt(8), FromInt(-8),
		FromInt(30), FromInt(60), FromInt(240), FromInt(-30), Number(-1 << 31),
	}
	numerators := []Number{0, 1, -1, FromInt(16), FromInt(-16), Number(-1 << 31), Number(0x7FFFFFFF)}
	for _, a := range numerators {
		for _, z := range moduli {
			check(a, z)
		}
	}
	seed := uint32(0x87654321)
	for i := 0; i < 200000; i++ {
		seed = seed*1664525 + 1013904223
		a := Number(seed)
		seed = seed*1664525 + 1013904223
		z := Number(seed)
		check(a, z)
		check(a, FromInt(8))
		check(a, FromInt(30))
		check(a, FromInt(-8))
	}
}
