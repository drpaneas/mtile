package runtime

import "testing"

func TestValueAPIOptionalDrawingDefaults(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}

	// Missing dw/dh default to the source rectangle dimensions. The source
	// pixel at (1,0) must therefore be copied to the destination unchanged.
	c.SSprV(IntVal(1), IntVal(0), IntVal(1), IntVal(1), IntVal(20), IntVal(20), Value{}, Value{}, Value{}, Value{})
	if got := c.Pget(20, 20); got != 7 {
		t.Fatalf("SSprV default destination size drew %d, want 7", got)
	}

	// Missing clip dimensions reset to the native display rectangle.
	c.ClipV(Value{}, Value{}, Value{}, Value{})
	if x, y, w, h := c.ClipRect(); x != 0 || y != 0 || w != ScreenWidth || h != ScreenHeight {
		t.Fatalf("ClipV defaults = %d,%d,%d,%d, want 0,0,%d,%d", x, y, w, h, ScreenWidth, ScreenHeight)
	}
}
