//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func requireTileCanvasPanic(t *testing.T, fragment string, call func()) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil || !strings.Contains(fmt.Sprint(value), fragment) {
			t.Fatalf("panic=%v, want message containing %q", value, fragment)
		}
	}()
	call()
}

func TestTileCanvasRejectsForegroundWithoutClear(t *testing.T) {
	for _, draw := range []struct {
		name string
		call func(*Context)
	}{
		{"opaque-zero", func(c *Context) { c.Pset(5, 6, 0) }},
		{"span", func(c *Context) { c.Rectfill(3, 4, 16, 9, 42) }},
		{"sprite", func(c *Context) { c.Spr(0, 5, 6, 1, 1, false, false) }},
		{"scaled-sprite", func(c *Context) { c.SSpr(0, 0, 8, 8, 5, 6, 13, 11, false, false) }},
	} {
		t.Run(draw.name, func(t *testing.T) {
			c := objectCanvasDrawContext(true)
			c.BeginRenderFrame()
			draw.call(c)
			requireTileCanvasPanic(t, "frame-initial cls", c.PrepareRender)
		})
	}
}

func TestTileCanvasRejectsOversizedMapRatherThanDroppingPixels(t *testing.T) {
	for _, shape := range []struct{ w, h, camera int }{{65, 20, 0}, {30, 33, 0}, {0, 20, 280}} {
		c := objectCanvasMapContext(1)
		c.BeginRenderFrame()
		c.Cls(0)
		c.Camera(shape.camera, 0)
		requireTileCanvasPanic(t, "64x32", func() { c.MapBG(0, 0, 0, 0, 0, shape.w, shape.h, 0) })
	}
	// Preserve nil/absent-asset no-ops before touching private renderer state.
	var absent *Context
	absent.MapBG(0, 0, 0, 0, 0, 30, 20, 0)
	new(Context).MapBG(0, 0, 0, 0, 0, 30, 20, 0)
}

func TestTileCanvasRejectsMapOrderingAndStateChanges(t *testing.T) {
	for _, test := range []struct {
		name, message string
		prepare       func(*Context)
	}{
		{"missing-clear", "ordered backgrounds", func(c *Context) {}},
		{"foreground-first", "ordered backgrounds", func(c *Context) { c.Cls(0); c.Pset(4, 4, 0) }},
		{"draw-remap", "identity draw palette", func(c *Context) { c.Cls(0); c.Pal(11, 14, 0) }},
		{"opaque-key-zero", "key-zero transparency", func(c *Context) { c.Cls(0); c.PaltColor(0, false) }},
		{"nonzero-key", "key-zero transparency", func(c *Context) { c.Cls(0); c.PaltColor(11, true) }},
		{"clip", "full clip", func(c *Context) { c.Cls(0); c.Clip(0, 0, 239, 160) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := objectCanvasMapContext(1)
			c.BeginRenderFrame()
			test.prepare(c)
			requireTileCanvasPanic(t, test.message, func() { c.MapBG(0, 0, 0, 0, 0, 30, 20, 0) })
		})
	}
	c := objectCanvasMapContext(1)
	c.BeginRenderFrame()
	c.Cls(0)
	requireTileCanvasPanic(t, "one frame-initial cls", func() { c.Cls(0) })
	requireTileCanvasPanic(t, "pixel readback", func() { c.Pget(0, 0) })
}

func TestTileCanvasHardwareInitializationAndFrozenFrame(t *testing.T) {
	// Poison all 128 OAM entries: the reusable backend may use only 96, but
	// inactive entries from an earlier program must never remain on screen.
	for slot := 0; slot < 128; slot++ {
		hw.SetReg16(hw.OAM+uintptr(slot*8), 0)
	}
	c := objectCanvasMapContext(15)
	for slot := 0; slot < 128; slot++ {
		if attr := hw.GetReg16(hw.OAM + uintptr(slot*8)); attr&0x0300 != 0x0200 {
			t.Fatalf("OAM%d not disabled on init: %04x", slot, attr)
		}
	}
	c.BeginRenderFrame()
	c.Pal(3, 17, 0)
	c.Cls(3)
	c.PalLayerReset(0)
	for bg := 0; bg < 4; bg++ {
		c.Camera(bg*3, bg)
		c.MapBG(bg, 0, 0, 0, 0, 48, 20, 0)
	}
	c.CameraReset()
	c.Pset(5, 6, 0)
	c.Pset(6, 6, 255)
	c.Pal(17, 23, 1)
	c.Pal(0, 42, 1)
	c.Pal(255, 11, 1)
	c.PrepareRender()
	hw.SetReg16(hw.VCOUNT, 160)
	c.PresentRender()
	if got := hw.GetReg16(hw.DISPCNT); got&0x1f07 != 0x1f00 {
		t.Fatalf("four BGs plus OBJ not enabled: %04x", got)
	}
	if c.canvasResult.objects != 1 || c.canvasResult.opaqueZero == 0 || c.canvasResult.opaqueZero == 255 {
		t.Fatalf("opaque-zero foreground allocation: %+v", c.canvasResult)
	}
	for _, check := range []struct {
		index int
		want  uint16
	}{
		{0, c.palette[23]}, // Remapped cls backdrop is independent of logical zero.
		{256 + int(c.canvasResult.opaqueZero), c.palette[42]},
		{255, c.palette[11]}, {256 + 255, c.palette[11]},
	} {
		if got := hw.GetReg16(hw.PALRAM + uintptr(check.index*2)); got != check.want {
			t.Fatalf("palette[%d]=%04x want%04x", check.index, got, check.want)
		}
	}
	before := tileCanvasHardwareSnapshot()
	objects := c.TileCanvasOBJCount()
	for freeze := 0; freeze < 3; freeze++ {
		c.BeginRenderFrame()
		c.PrepareRender()
		c.PresentRender()
		if c.lastBlitCycles != 0 || c.TileCanvasOBJCount() != objects || !reflect.DeepEqual(before, tileCanvasHardwareSnapshot()) {
			t.Fatalf("empty draw changed committed image during freeze frame%d", freeze)
		}
	}
	// A later cleared frame removes old OBJ visibility even though unused
	// tile bytes need not be erased or uploaded.
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrepareRender()
	hw.SetReg16(hw.VCOUNT, 160)
	c.PresentRender()
	if c.TileCanvasOBJCount() != 0 || hw.GetReg16(hw.OAM)&0x0300 != 0x0200 {
		t.Fatal("clear after freeze retained previous foreground object")
	}
}

func tileCanvasHardwareSnapshot() []uint16 {
	var out []uint16
	for _, area := range []struct {
		start uintptr
		bytes int
	}{
		{hw.VRAM, 96 * 1024}, {hw.PALRAM, 1024}, {hw.OAM, 1024},
	} {
		for offset := 0; offset < area.bytes; offset += 2 {
			out = append(out, hw.GetReg16(area.start+uintptr(offset)))
		}
	}
	out = append(out, hw.GetReg16(hw.DISPCNT))
	for offset := 0; offset < 24; offset += 2 {
		out = append(out, hw.GetReg16(hw.BG0CNT+uintptr(offset)))
	}
	return out
}
