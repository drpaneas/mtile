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
