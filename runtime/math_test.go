package runtime

import (
	"math"
	"testing"
)

func TestNumberCeilAbsSgnMid(t *testing.T) {
	t.Parallel()
	if FromInt(2).Ceil() != FromInt(2) {
		t.Fatal("ceil of integer changed")
	}
	if Number(One+One/2).Ceil() != FromInt(2) {
		t.Fatal("ceil(1.5) != 2")
	}
	if Number(-3*int(One)/2).Ceil() != FromInt(-1) {
		t.Fatal("ceil(-1.5) != -1")
	}
	if FromInt(-4).Abs() != FromInt(4) {
		t.Fatal("abs")
	}
	if Number(0).Sgn() != One {
		t.Fatal("sgn(0) != 1")
	}
	if FromInt(-2).Sgn() != -One {
		t.Fatal("sgn negative")
	}
	if Mid(FromInt(3), FromInt(1), FromInt(2)) != FromInt(2) {
		t.Fatal("mid is not the median")
	}
	if Min(FromInt(2), FromInt(5)) != FromInt(2) || Max(FromInt(2), FromInt(5)) != FromInt(5) {
		t.Fatal("min/max")
	}
}

func TestSinCosLUTMatchesCardinals(t *testing.T) {
	t.Parallel()
	if Number(0).Sin() != 0 {
		t.Fatal("sin(0) != 0")
	}
	if (One / 4).Sin() != -One {
		t.Fatalf("sin(0.25) = %d, want -1", (One / 4).Sin())
	}
	if (One / 2).Sin() != 0 {
		t.Fatal("sin(0.5) != 0")
	}
	if (3 * One / 4).Sin() != One {
		t.Fatalf("sin(0.75) = %d, want 1", (3 * One / 4).Sin())
	}
	if Number(0).Cos() != One {
		t.Fatalf("cos(0) = %d, want 1", Number(0).Cos())
	}
	if sinLUT[64] != -One || sinLUT[192] != One {
		t.Fatal("LUT cardinals drifted from -sin(2π i/256)")
	}
}

func TestSinLUTMatchesNegSinFormula(t *testing.T) {
	t.Parallel()
	for i := 0; i < 256; i++ {
		want := Number(math.Round(-math.Sin(2*math.Pi*float64(i)/256.0) * 65536.0))
		if sinLUT[i] != want {
			t.Fatalf("sinLUT[%d] = %d, want %d from -sin(2π i/256)", i, sinLUT[i], want)
		}
	}
}

func TestSinMatchesFractionalReference(t *testing.T) {
	t.Parallel()
	// These values are the 16:16 reference outputs for fractional angles.
	// The runtime uses a compact interpolated LUT, so allow a small
	// fixed-point error rather than requiring bit-identical LUT internals.
	tests := []struct {
		name string
		arg  Number
		want Number
	}{
		{"0.001", Number(0x00000041), Number(-402)},
		{"0.01", Number(0x0000028f), Number(-4119)},
		{"0.025", Number(0x00000666), Number(-10262)},
		{"0.075", Number(0x00001333), Number(-29757)},
	}
	for _, tt := range tests {
		got := tt.arg.Sin()
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 32 {
			t.Errorf("sin(%s) = %d, want %d ±32", tt.name, got, tt.want)
		}
	}
}

func TestAtan2ReturnsTurns(t *testing.T) {
	t.Parallel()
	if Atan2(0, 0) != One/4 {
		t.Fatal("atan2(0,0) != 0.25")
	}
	if Atan2(One, 0) != 0 {
		t.Fatal("atan2(1,0) != 0")
	}
	if Atan2(0, -One) != One/4 {
		t.Fatalf("atan2(0,-1) = %d, want 0.25", Atan2(0, -One))
	}
	if Atan2(-One, 0) != One/2 {
		t.Fatalf("atan2(-1,0) = %d, want 0.5", Atan2(-One, 0))
	}
	if Atan2(0, One) != 3*One/4 {
		t.Fatalf("atan2(0,1) = %d, want 0.75", Atan2(0, One))
	}
	if Atan2(One, One) != Number(0xE000) {
		t.Fatalf("atan2(1,1) = %#x, want 0xe000", uint32(Atan2(One, One)))
	}
	atan := Atan2(Number(123), Number(47))
	atanDiff := atan - Number(0xF124)
	if atanDiff < 0 {
		atanDiff = -atanDiff
	}
	if atanDiff > 32 {
		t.Fatalf("atan2(123,47) = %#x, want 0xf124 ±32", uint32(atan))
	}
}

// sqrtReferenceFull16 is the pre-optimization Sqrt: no early exit, always
// runs all 16 Newton-Raphson iterations. TestSqrtEarlyExitMatchesFull16
// pins that the early-exit version in math.go returns bit-identical
// results, since the two must never diverge - the loop body is a pure
// function of x, so once next==x the remaining iterations are provably
// no-ops.
func sqrtReferenceFull16(n Number) Number {
	if n <= 0 {
		return 0
	}
	x := n
	if x < One {
		x = One
	}
	for i := 0; i < 16; i++ {
		if x == 0 {
			return 0
		}
		x = Number((int64(x) + int64(n.Div(x))) / 2)
	}
	return x
}

func TestSqrtEarlyExitMatchesFull16(t *testing.T) {
	t.Parallel()
	// A mix of small values (where the fixed 16-iteration cap actually
	// matters, per the doc comment on Sqrt), perfect squares, and a wide
	// swept range covering both sub-One and large fixed-point magnitudes.
	cases := []Number{1, 2, 3, 4, 5, 100, One, One - 1, One + 1, FromInt(2), FromInt(4), FromInt(1000), FromInt(1 << 20)}
	for _, n := range cases {
		if got, want := n.Sqrt(), sqrtReferenceFull16(n); got != want {
			t.Fatalf("Sqrt(%d) = %d, want %d (full-16-iteration reference)", n, got, want)
		}
	}
	for i := int64(-100); i < 5_000_000; i += 17 {
		n := Number(i)
		if got, want := n.Sqrt(), sqrtReferenceFull16(n); got != want {
			t.Fatalf("Sqrt(%d) = %d, want %d (full-16-iteration reference)", n, got, want)
		}
	}
}

func TestSqrtAndBitwiseAndRnd(t *testing.T) {
	t.Parallel()
	if FromInt(4).Sqrt() != FromInt(2) {
		t.Fatalf("sqrt(4) = %d", FromInt(4).Sqrt())
	}
	if Number(-One).Sqrt() != 0 {
		t.Fatal("sqrt negative != 0")
	}
	half := Number(One / 2)
	if Number(One+half).Band(Number(0xFFFF)) != half {
		t.Fatal("band should operate on fraction bits")
	}
	if FromInt(1).Shl(FromInt(1)) != FromInt(2) {
		t.Fatal("shl(1,1) != 2")
	}
	var c Context
	c.Srand(FromInt(1))
	a := c.Rnd(FromInt(10))
	c.Srand(FromInt(1))
	b := c.Rnd(FromInt(10))
	if a != b {
		t.Fatal("srand did not replay the sequence")
	}
	if a < 0 || a >= FromInt(10) {
		t.Fatalf("rnd not in [0,10): %d", a)
	}
	if c.Rnd(0) != 0 {
		t.Fatal("rnd(0) != 0")
	}
}

func TestTimeUsesUpdateTick(t *testing.T) {
	t.Parallel()
	var c Context
	c.AdvanceTick()
	c.AdvanceTick()
	c.AdvanceTick()
	want := FromInt(3).Div(FromInt(60))
	if c.Time() != want {
		t.Fatalf("time = %d, want %d", c.Time(), want)
	}
	if c.T() != c.Time() {
		t.Fatal("t() is not an alias of time()")
	}
}
