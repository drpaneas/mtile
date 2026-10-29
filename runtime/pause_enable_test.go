package runtime

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

func TestPauseDisabledByDefaultDeliversStartToGame(t *testing.T) {
	hw.ResetForTest()
	var c Context
	pauseKeys(&c, 8)
	if c.PauseActive() || !c.Btn(ButtonStart) {
		t.Fatal("default pause intercepted the menu's Start button")
	}
	c.AdvanceTick()
	if !c.Btnp(ButtonStart) {
		t.Fatal("default pause swallowed the Start press edge")
	}
	pauseKeys(&c, 8)
	c.AdvanceTick()
	if c.Btnp(ButtonStart) {
		t.Fatal("disabled pause changed held-button repeat")
	}
}

func TestPauseEnablingDoesNotCaptureHeldMenuStart(t *testing.T) {
	hw.ResetForTest()
	var c Context
	pauseKeys(&c, 8) // The menu uses Start to launch gameplay.
	c.PauseEnableV(BoolVal(true))
	if c.PauseActive() {
		t.Fatal("enabling paused immediately")
	}
	pauseKeys(&c, 8)
	if c.PauseActive() || c.Btn(ButtonStart) {
		t.Fatal("held menu Start triggered pause or leaked into gameplay")
	}
	pauseKeys(&c, 0)
	pauseKeys(&c, 8)
	if !c.PauseActive() {
		t.Fatal("fresh Start did not pause")
	}
	c.PreparePauseMenu()
	c.PresentPauseMenu()
	pauseKeys(&c, 0)
	pauseKeys(&c, 8)
	c.PresentPauseMenu()
	if c.PauseActive() {
		t.Fatal("fresh Start did not resume")
	}
	c.PauseEnableV(BoolVal(false))
	pauseKeys(&c, 8)
	if c.Btn(ButtonStart) {
		t.Fatal("held resume key leaked into menu")
	}
	pauseKeys(&c, 0)
	pauseKeys(&c, 8)
	if c.PauseActive() || !c.Btn(ButtonStart) {
		t.Fatal("disabled pause still owns Start")
	}
}

func TestDisablingActivePauseRestoresAtPresentation(t *testing.T) {
	hw.ResetForTest()
	var c Context
	hw.SetReg16(hw.DISPCNT, 0x1454)
	hw.SetReg32(hw.OAM, 0x12345678)
	c.PauseEnableV(BoolVal(true))
	pauseKeys(&c, 8)
	c.PreparePauseMenu()
	c.PresentPauseMenu()
	c.PauseEnableV(BoolVal(false))
	pauseKeys(&c, 1)
	if !c.PauseActive() || !c.pause.closing || c.Btn(ButtonA) {
		t.Fatal("disable resumed before restoring or leaked held input")
	}
	c.PresentPauseMenu()
	if c.PauseActive() || hw.GetReg16(hw.DISPCNT) != 0x1454 || hw.GetReg32(hw.OAM) != 0x12345678 {
		t.Fatal("disabled pause did not restore graphics")
	}
	pauseKeys(&c, 0)
	pauseKeys(&c, 8)
	if !c.Btn(ButtonStart) || c.PauseActive() {
		t.Fatal("Start did not return to menu after disable")
	}
}

func TestPauseEnableIgnoresNonBooleans(t *testing.T) {
	var c Context
	for _, enabled := range []bool{false, true} {
		c.PauseEnableV(BoolVal(enabled))
		for _, invalid := range []Value{NilVal(), IntVal(0), IntVal(1), c.InternStatic("true")} {
			c.PauseEnableV(invalid)
			if c.pause.enabled != enabled {
				t.Fatal("non-boolean changed setting", invalid)
			}
		}
	}
}
