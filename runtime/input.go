package runtime

import "github.com/drpaneas/mtile/runtime/hw"

// Button indices preserve Mtile's original PICO-style directions/B/A
// mapping at 0..5 and append the remaining native GBA controls at 6..9.
// They are exported for Go cartridges and documented as the numeric btn()/
// btnp() indices used by Lua games.
const (
	ButtonLeft = iota
	ButtonRight
	ButtonUp
	ButtonDown
	ButtonB
	ButtonA
	ButtonStart
	ButtonSelect
	ButtonL
	ButtonR
	ButtonCount
)

var buttonMasks = [ButtonCount]uint16{
	ButtonLeft:   0x0020, // KEYINPUT bit 5
	ButtonRight:  0x0010, // KEYINPUT bit 4
	ButtonUp:     0x0040, // KEYINPUT bit 6
	ButtonDown:   0x0080, // KEYINPUT bit 7
	ButtonB:      0x0002, // KEYINPUT bit 1
	ButtonA:      0x0001, // KEYINPUT bit 0
	ButtonStart:  0x0008, // KEYINPUT bit 3
	ButtonSelect: 0x0004, // KEYINPUT bit 2
	ButtonL:      0x0200, // KEYINPUT bit 9
	ButtonR:      0x0100, // KEYINPUT bit 8
}

// PollInput reads all ten active-low KEYINPUT bits.
func (c *Context) PollInput() {
	c.keys = ^hw.GetReg16(hw.KEYINPUT) & 0x03FF
}

func (c *Context) Btn(index int) bool {
	if index < 0 || index >= len(buttonMasks) {
		return false
	}
	return c.keys&buttonMasks[index] != 0
}

// AdvanceTick updates btnp repeat state once per update tick (30 Hz).
// First press is 1, the first repeat is 15, and later repeats are every
// four ticks - PICO-8's native btnp schedule at PICO-8's native rate, so
// held-button feel matches the original console in real time.
func (c *Context) AdvanceTick() {
	for i, mask := range buttonMasks {
		if c.keys&mask == 0 {
			c.hold[i] = 0
			continue
		}
		if c.prevKeys&mask == 0 {
			c.hold[i] = 1
		} else if c.hold[i] == ^uint16(0) {
			// 65535-15 is divisible by 4, so a saturated counter would make
			// Btnp true every update. Wrap to 16 to keep the 4-tick cadence.
			c.hold[i] = 16
		} else {
			c.hold[i]++
		}
	}
	c.prevKeys = c.keys
	c.updateTick++
}

func (c *Context) Btnp(index int) bool {
	if index < 0 || index >= len(buttonMasks) {
		return false
	}
	hold := c.hold[index]
	return hold == 1 || hold == 15 || (hold > 15 && (hold-15)%4 == 0)
}

func (c *Context) UpdateTick() uint32 {
	return c.updateTick
}
