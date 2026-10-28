package runtime

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

func pauseKeys(c *Context, keys uint16) PauseAction {
	hw.SetKeyInputForTest(0x3ff &^ keys)
	c.PollInput()
	return c.PollPauseMenu()
}

func TestPauseRestoresGraphicsAndConsumesMenuKeys(t *testing.T) {
	for _, mode := range []uint16{0x1f40, 0x1454} {
		hw.ResetForTest()
		var c Context
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
		pauseKeys(&c, 2)
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

func TestSaveWriteOutsidePauseBlocksResumeFor150Polls(t *testing.T) {
	hw.ResetForTest()
	hw.ResetSRAMForTest(0xff)
	c := saveContext()
	if result := c.SaveWrite("pause-direct-v1"); result != SaveOK {
		t.Fatalf("SaveWrite failed: %d", result)
	}
	if c.PauseActive() {
		t.Fatal("SaveWrite opened the pause menu")
	}

	// The first Start opens the menu. Alternate B and Start afterward so
	// every remaining poll attempts to resume with a fresh button edge.
	// No PauseSaveResult call may supply the wait on SaveWrite's behalf.
	for poll := 1; poll <= 150; poll++ {
		keys := uint16(8)
		if poll%2 == 0 {
			keys = 2
		}
		pauseKeys(c, keys)
		c.PreparePauseMenu()
		c.PresentPauseMenu()
		if poll < 150 && !c.PauseActive() {
			t.Fatalf("resumed on poll %d before the 150-poll wait expired", poll)
		}
		if poll == 150 && c.PauseActive() {
			t.Fatal("resume still blocked after 150 polls")
		}
	}
}

func TestPauseActionsCooldownAndExit(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.ConfigurePauseMenu()
	pauseKeys(&c, 8)
	pauseKeys(&c, 0)
	pauseKeys(&c, 0x80)
	pauseKeys(&c, 0)
	if action := pauseKeys(&c, 1); action != PauseSave {
		t.Fatal(action)
	}
	c.PauseSaveResult(SaveOK)
	if pauseKeys(&c, 1) != PauseNone {
		t.Fatal("held A repeated save")
	}
	for i := 0; i < 100; i++ {
		pauseKeys(&c, 0)
	}
	pauseKeys(&c, 2)
	if c.pause.closing {
		t.Fatal("resumed before Omega wait")
	}
	pauseKeys(&c, 0)
	if pauseKeys(&c, 1) != PauseSave {
		t.Fatal("second save")
	}
	c.PauseSaveResult(SaveOK)
	for i := 0; i < 149; i++ {
		pauseKeys(&c, 0)
	}
	if c.pause.wait != 1 {
		t.Fatal("save did not restart full wait")
	}
	pauseKeys(&c, 0)
	if c.pause.wait != 0 {
		t.Fatal("wait stuck")
	}
	c.pause.wait = 10
	c.PauseSaveResult(SaveIOError)
	for i := 0; i < 11; i++ {
		pauseKeys(&c, 0)
	}
	if c.pause.result != -1 {
		t.Fatal("wait expiry hid failure")
	}
	pauseKeys(&c, 0x40)
	pauseKeys(&c, 0)
	if pauseKeys(&c, 1) != PauseLoad {
		t.Fatal("load action")
	}
	c.PauseLoadResult(SaveEmpty)
	if c.pause.result != 3 {
		t.Fatal("empty save feedback")
	}
	pauseKeys(&c, 0)
	pauseKeys(&c, 0x80)
	pauseKeys(&c, 0)
	pauseKeys(&c, 0x80)
	pauseKeys(&c, 0)
	c.pause.wait = 5
	if pauseKeys(&c, 1) != PauseExit || !c.pause.ended {
		t.Fatal("exit did not end session")
	}
	for i := 0; i < 8; i++ {
		pauseKeys(&c, 8|2)
		c.PresentPauseMenu()
	}
	if !c.PauseActive() || !c.pause.ended || c.pause.wait != 0 {
		t.Fatal("ended session resumed or wait stuck")
	}
}

func TestPauseUnavailableSaving(t *testing.T) {
	var c Context
	pauseKeys(&c, 8)
	pauseKeys(&c, 0)
	pauseKeys(&c, 0x80)
	pauseKeys(&c, 0)
	if pauseKeys(&c, 1) != PauseNone || c.pause.result != 10 {
		t.Fatal("unconfigured game attempted load")
	}
}

func TestPauseStartTogglesAndDefaultSelectionIsLoad(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.ConfigurePauseMenu()
	pauseKeys(&c, 8)
	c.PreparePauseMenu()
	c.PresentPauseMenu()
	pauseKeys(&c, 0)
	if pauseKeys(&c, 1) != PauseLoad {
		t.Fatal("first menu item must be Load")
	}
	pauseKeys(&c, 0)
	pauseKeys(&c, 8)
	c.PresentPauseMenu()
	if c.PauseActive() {
		t.Fatal("second Start did not resume")
	}
}

func TestPauseOverlayPreservesFrozenLayersAndObjectOrder(t *testing.T) {
	hw.ResetForTest()
	var c Context
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

func TestPauseOverlayHasCompactCenteredPanel(t *testing.T) {
	var c Context
	c.ConfigurePauseMenu()
	c.PreparePauseMenu()
	pixel := func(x, y int) uint32 { return c.pause.video.pixels[(y/8*8+x/8)*8+y%8] >> uint(x%8*4) & 15 }
	// The common 64x48 panel is centered at (120,80); all other pixels of
	// its 64x64 OBJ stay transparent. Heading PAUSED occupies centered x=17..45.
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if y < 8 || y >= 56 {
				if pixel(x, y) != 0 {
					t.Fatal("panel exceeds compact bounds")
				}
			}
			if y >= 14 && y < 19 && pixel(x, y) == 5 && (x < 17 || x > 45) {
				t.Fatal("heading is not centered")
			}
		}
	}
	if pixel(4, 24) != 3 || pixel(59, 24) != 2 {
		t.Fatal("missing thin selection bar or row highlight")
	}
}
