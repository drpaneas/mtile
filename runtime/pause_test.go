package runtime

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

func pauseKeys(c *Context, keys uint16) {
	hw.SetKeyInputForTest(0x3ff &^ keys)
	c.PollInput()
	c.PollPauseMenu()
}

func TestPauseRestoresGraphicsAndConsumesMenuKeys(t *testing.T) {
	for _, mode := range []uint16{0x1f40, 0x1454} {
		hw.ResetForTest()
		var c Context
		c.PauseEnableV(BoolVal(true))
		hw.SetReg16(hw.DISPCNT, mode)
		hw.SetReg16(pauseBlendControl, 0x1234)
		for i := 0; i < 512; i++ {
			hw.SetVRAM32(pauseTileAddress+uintptr(i*4), uint32(i)*98765+123)
			hw.SetPaletteRAM16(i, uint16(i*63+9))
		}
		for i := 0; i < 256; i++ {
			hw.SetReg32(hw.OAM+uintptr(i*4), uint32(i)*4567+8)
		}
		pauseKeys(&c, 8)
		c.PresentPauseMenu()
		if !c.PauseActive() || c.updateTick != 0 {
			t.Fatal("menu did not open independently of game ticks")
		}
		pauseKeys(&c, 8)
		if c.pause.closing {
			t.Fatal("held Start immediately resumed")
		}
		pauseKeys(&c, 0)
		pauseKeys(&c, 8|2)
		c.PresentPauseMenu()
		if c.PauseActive() || hw.GetReg16(hw.DISPCNT) != mode || hw.GetReg16(pauseBlendControl) != 0x1234 {
			t.Fatal("display state not restored")
		}
		for i := 0; i < 512; i++ {
			if hw.ReadVRAM32(pauseTileAddress+uintptr(i*4)) != uint32(i)*98765+123 || hw.ReadPaletteRAM16(i) != uint16(i*63+9) {
				t.Fatalf("tiles/palette %d changed", i)
			}
		}
		for i := 0; i < 256; i++ {
			if hw.GetReg32(hw.OAM+uintptr(i*4)) != uint32(i)*4567+8 {
				t.Fatalf("OAM %d changed", i)
			}
		}
		pauseKeys(&c, 2)
		if c.Btn(ButtonB) {
			t.Fatal("resume key leaked into game")
		}
		pauseKeys(&c, 0)
		pauseKeys(&c, 2)
		if !c.Btn(ButtonB) {
			t.Fatal("fresh game key was swallowed")
		}
	}
}

func TestPauseOnlyStartResumes(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.PauseEnableV(BoolVal(true))
	pauseKeys(&c, 8)
	c.PreparePauseMenu()
	c.PresentPauseMenu()
	original := c.pause.video.pixels
	for _, keys := range []uint16{0, 1, 2, 0x40, 0x80, 0x10, 0x20, 0x100, 0x200} {
		pauseKeys(&c, keys)
		c.PreparePauseMenu()
		c.PresentPauseMenu()
		if !c.PauseActive() || original != c.pause.video.pixels {
			t.Fatal("non-Start input changed pause")
		}
	}
	pauseKeys(&c, 8)
	c.PresentPauseMenu()
	if c.PauseActive() {
		t.Fatal("second Start did not resume")
	}
}

func TestSaveWriteStartsIndependent150PollReminder(t *testing.T) {
	hw.ResetForTest()
	hw.ResetSRAMForTest(0xff)
	c := saveContext()
	c.PauseEnableV(BoolVal(true))
	if c.SavePending() || c.SaveWrite("direct-v1") != SaveOK || !c.SavePending() || c.PauseActive() {
		t.Fatal("direct write did not start reminder")
	}
	for poll := 1; poll <= 150; poll++ {
		// A successful save must never prevent Start from toggling pause.
		keys := uint16(0)
		if poll == 1 || poll == 3 {
			keys = 8
		}
		pauseKeys(c, keys)
		if c.PauseActive() {
			c.PreparePauseMenu()
			c.PresentPauseMenu()
		}
		if poll == 3 && c.PauseActive() {
			t.Fatal("save reminder blocked resume")
		}
		if c.SavePending() != (poll < 150) {
			t.Fatalf("wrong pending result at poll %d", poll)
		}
	}
	if c.SaveWrite("direct-v1") != SaveOK {
		t.Fatal("repeat write")
	}
	pauseKeys(c, 0)
	hw.SetSRAMWriteLimitForTest(0)
	if c.SaveWrite("direct-v1") != SaveIOError || c.saveWait != 149 {
		t.Fatal("failed save cleared or restarted prior reminder")
	}
	hw.SetSRAMWriteLimitForTest(-1)
	if c.SaveWrite("direct-v1") != SaveOK || c.saveWait != 150 {
		t.Fatal("successful retry did not restart reminder")
	}
}

func TestPauseOverlayPreservesFrozenLayersAndObjectOrder(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.PauseEnableV(BoolVal(true))
	hw.SetReg16(hw.DISPCNT, 0x1f40)
	// A full valid game allocation uses 127 entries; entry 127 is private.
	for i := 0; i < 127; i++ {
		hw.SetReg32(hw.OAM+uintptr(i*8), uint32(0x2000+i)|uint32(i*3)<<16)
		hw.SetReg32(hw.OAM+uintptr(i*8+4), uint32(i*2)|uint32(i+7)<<16)
	}
	hw.SetReg16(hw.OAM+127*8, 0x0200)
	for i := 0; i < 512; i++ {
		hw.SetPaletteRAM16(i, uint16(i*31+3))
	}
	hw.SetVRAM32(hw.VRAM+0x2000, 0x12345678)
	pauseKeys(&c, 8)
	c.PreparePauseMenu()
	c.PresentPauseMenu()
	if hw.GetReg16(hw.DISPCNT) != 0x1f40 || hw.ReadVRAM32(hw.VRAM+0x2000) != 0x12345678 {
		t.Fatal("frozen BGs changed")
	}
	for i := 0; i < 127; i++ {
		if hw.GetReg32(hw.OAM+uintptr((i+1)*8)) != uint32(0x2000+i)|uint32(i*3)<<16 || hw.GetReg32(hw.OAM+uintptr((i+1)*8+4)) != uint32(i*2)|uint32(i+7)<<16 {
			t.Fatalf("game object %d changed or disappeared", i)
		}
	}
	for i := 0; i < 512; i++ {
		if i >= 256+PauseOBJPaletteStart && i < 256+PauseOBJPaletteStart+PauseOBJPaletteColors {
			continue
		}
		if hw.ReadPaletteRAM16(i) != uint16(i*31+3) {
			t.Fatalf("unowned color %d changed", i)
		}
	}
}

func TestPauseOverlayContainsOnlyCenteredSpacedLabel(t *testing.T) {
	var c Context
	c.PauseEnableV(BoolVal(true))
	c.PreparePauseMenu()
	pixel := func(x, y int) uint32 { return c.pause.video.pixels[(y/8*8+x/8)*8+y%8] >> uint(x%8*4) & 15 }
	ink, shadow := 0, 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := pixel(x, y)
			if v == 0 {
				continue
			}
			if x < 7 || x > 57 || y < 28 || y > 35 {
				t.Fatalf("ink outside centered message: %d,%d", x, y)
			}
			switch v {
			case 4:
				ink++
			case 1:
				shadow++
			default:
				t.Fatal("unexpected panel or highlight")
			}
		}
	}
	if ink < 80 || shadow < 20 {
		t.Fatal("missing message or shadow")
	}
	for letter := 0; letter < 5; letter++ {
		for y := 28; y <= 35; y++ {
			if pixel(7+letter*9+7, y) != 0 {
				t.Fatal("missing inter-letter spacing")
			}
		}
	}
	// Project font selection cannot restyle the system label.
	original := c.pause.video.pixels
	c.font = &Font{}
	c.PreparePauseMenu()
	if original != c.pause.video.pixels {
		t.Fatal("project font changed system label")
	}
}
