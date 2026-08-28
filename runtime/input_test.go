package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestInputInvertsKEYINPUTAndMapsAllGBAButtons(t *testing.T) {
	if len(buttonMasks) != ButtonCount {
		t.Fatalf("button mask count = %d, want %d", len(buttonMasks), ButtonCount)
	}
	if ButtonCount != 10 {
		t.Fatalf("ButtonCount = %d, want all 10 GBA buttons", ButtonCount)
	}
	for index, mask := range buttonMasks {
		hw.ResetForTest()
		hw.SetKeyInputForTest(0x03FF &^ mask)
		var c Context
		c.PollInput()
		for other := 0; other < ButtonCount; other++ {
			want := other == index
			if got := c.Btn(other); got != want {
				t.Fatalf("pressed index %d: Btn(%d) = %v, want %v", index, other, got, want)
			}
		}
		c.AdvanceTick()
		if !c.Btnp(index) {
			t.Fatalf("first press of index %d did not produce btnp", index)
		}
	}

	hw.ResetForTest()
	var c Context
	c.PollInput()
	if c.Btn(-1) || c.Btn(ButtonCount) || c.Btnp(-1) || c.Btnp(ButtonCount) {
		t.Fatal("out-of-range button index was reported pressed")
	}
}

func TestInputHeldRepeatAppliesToAllGBAButtons(t *testing.T) {
	for index, mask := range buttonMasks {
		hw.ResetForTest()
		hw.SetKeyInputForTest(0x03FF &^ mask)
		var c Context
		c.PollInput()
		c.AdvanceTick()

		// Holding for 14 more update ticks reaches the first repeat at 15.
		for i := 0; i < 14; i++ {
			c.PollInput()
			c.AdvanceTick()
		}
		if !c.Btnp(index) {
			t.Fatalf("button %d: 15th held update did not produce btnp repeat", index)
		}
		for i := 0; i < 4; i++ {
			c.PollInput()
			c.AdvanceTick()
		}
		if !c.Btnp(index) {
			t.Fatalf("button %d: 4-tick held repeat did not produce btnp", index)
		}
	}
}

func TestInputReleaseClearsButtonRepeat(t *testing.T) {
	hw.ResetForTest()
	var c Context
	hw.SetKeyInputForTest(0x03FF &^ buttonMasks[ButtonA])
	c.PollInput()
	c.AdvanceTick()
	hw.SetKeyInputForTest(0x03FF)
	c.PollInput()
	c.AdvanceTick()
	if c.Btn(ButtonA) || c.Btnp(ButtonA) {
		t.Fatal("released button remained down or repeating")
	}
}

func TestInputHoldWrapKeepsRepeatCadence(t *testing.T) {
	hw.ResetForTest()
	hw.SetKeyInputForTest(0x03FF &^ buttonMasks[ButtonLeft])
	var c Context
	c.PollInput()
	c.hold[ButtonLeft] = ^uint16(0)
	c.prevKeys = c.keys
	c.AdvanceTick()
	if c.hold[ButtonLeft] != 16 {
		t.Fatalf("hold wrap = %d, want 16", c.hold[ButtonLeft])
	}
	if c.Btnp(ButtonLeft) {
		t.Fatal("wrap tick produced a spurious btnp")
	}
}
