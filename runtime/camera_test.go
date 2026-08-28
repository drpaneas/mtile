package runtime

import "testing"

func TestCameraOffsetsPsetAndPget(t *testing.T) {
	t.Parallel()
	var c Context
	c.Camera(10, 4)
	c.Pset(10, 4, 7)
	if c.Pget(10, 4) != 7 {
		t.Fatal("Pget did not apply camera")
	}
	c.CameraReset()
	if c.Pget(0, 0) != 7 {
		t.Fatal("camera offset did not land at screen origin")
	}
}

func TestClipRejectsPixelsOutsideRectangle(t *testing.T) {
	t.Parallel()
	var c Context
	c.Clip(2, 2, 4, 4)
	c.Pset(1, 2, 7)
	c.Pset(2, 2, 8)
	c.Pset(5, 5, 9)
	c.Pset(6, 2, 10)
	if c.Pget(1, 2) != 0 || c.Pget(6, 2) != 0 {
		t.Fatal("clip leaked an outside pixel")
	}
	if c.Pget(2, 2) != 8 || c.Pget(5, 5) != 9 {
		t.Fatal("clip rejected an inside pixel")
	}
	x, y, w, h := c.ClipRect()
	if x != 2 || y != 2 || w != 4 || h != 4 {
		t.Fatalf("clip rect = %d,%d,%d,%d", x, y, w, h)
	}
}

func TestClsRestoresFullClip(t *testing.T) {
	t.Parallel()
	var c Context
	c.Clip(0, 0, 8, 8)
	c.Cls(3)
	_, _, w, h := c.ClipRect()
	if w != ScreenWidth || h != ScreenHeight {
		t.Fatal("cls did not reset clip")
	}
	if c.Pget(20, 20) != 3 {
		t.Fatal("cls did not fill outside the previous clip")
	}
}

func TestLineRectCircHonorClipAndOpaqueZero(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(4)
	c.Line(0, 0, 3, 0, 0)
	if c.Pget(1, 0) != 0 {
		t.Fatal("line color 0 was not opaque")
	}
	c.Rect(10, 10, 13, 13, 7)
	if c.Pget(10, 10) != 7 || c.Pget(12, 10) != 7 || c.Pget(11, 11) != 4 {
		t.Fatal("rect outline filled the interior")
	}
	c.Circ(20, 20, 0, 8)
	if c.Pget(20, 20) != 8 {
		t.Fatal("circ r=0 did not plot the center")
	}
	c.Circ(30, 30, -1, 8)
	if c.Pget(30, 30) != 4 {
		t.Fatal("circ r<0 was not a no-op")
	}
	c.Circfill(40, 40, 2, 9)
	if c.Pget(40, 40) != 9 || c.Pget(41, 40) != 9 {
		t.Fatal("circfill missed the interior")
	}
	c.Clip(0, 0, 5, 5)
	c.Line(0, 0, 20, 0, 11)
	if c.Pget(6, 0) != 4 {
		t.Fatal("line escaped clip")
	}
}

// TestRectAndCircfillInitDrawStateWithoutPriorCall guards the hlineWorld
// optimization: hlineWorld itself no longer calls ensureDrawState (it used
// to, on every span - up to 4 times per midpoint-circle step for Circfill,
// and twice per Rect), relying on Rect/Circfill to have called it once
// already. This exercises that on a Context that has never touched draw
// state at all, so a missing call would surface as a wrong default clip
// (zero-value clipX1/clipY1 would clip everything away).
func TestRectAndCircfillInitDrawStateWithoutPriorCall(t *testing.T) {
	t.Parallel()
	var c Context
	c.Rect(100, 100, 103, 103, 7)
	if c.Pget(100, 100) != 7 || c.Pget(103, 100) != 7 {
		t.Fatal("Rect on a fresh Context did not draw; default clip was not initialized")
	}
	var d Context
	d.Circfill(50, 50, 3, 9)
	if d.Pget(50, 50) != 9 || d.Pget(52, 50) != 9 {
		t.Fatal("Circfill on a fresh Context did not draw; default clip was not initialized")
	}
}

func TestCameraDefaults(t *testing.T) {
	t.Parallel()
	var c Context
	c.ensureDrawState()
	x, y := c.CameraPos()
	if x != 0 || y != 0 {
		t.Fatal("default camera is not 0,0")
	}
	cx, cy, w, h := c.ClipRect()
	if cx != 0 || cy != 0 || w != ScreenWidth || h != ScreenHeight {
		t.Fatal("default clip is not 0,0,240,160")
	}
}
