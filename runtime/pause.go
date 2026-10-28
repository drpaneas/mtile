package runtime

import "github.com/drpaneas/mtile/runtime/hw"

type PauseAction uint8

const (
	PauseNone PauseAction = iota
	PauseSave
	PauseLoad
	PauseExit
)

type pauseState struct {
	video                          pauseVideo
	active, closing, ended, saving bool
	previous, blocked              uint16
	selected                       uint8
	result                         int8
	wait                           uint8
}

// ConfigurePauseMenu enables the two statically bound game callbacks. The
// shared menu itself is available even when a game has no save callbacks.
func (c *Context) ConfigurePauseMenu() { c.pause.saving = true }
func (c *Context) PauseActive() bool   { return c.pause.active }

// PollPauseMenu owns Start and menu input independently of the game's tick.
// The generated loop polls it even in games that never call btn/time.
func (c *Context) PollPauseMenu() PauseAction {
	p := &c.pause
	raw := c.keys
	pressed := raw &^ p.previous
	p.previous = raw
	p.blocked &= raw
	c.keys = raw &^ p.blocked
	if p.wait > 0 {
		p.wait--
	}
	if p.ended {
		return PauseNone
	}
	if !p.active {
		if pressed&8 != 0 {
			p.active, p.closing, p.selected, p.result = true, false, 0, 0
		}
		return PauseNone
	}
	if pressed&(8|2) != 0 {
		if p.wait == 0 {
			p.closing = true
			p.blocked |= raw
			c.prevKeys = 0
			for i := range c.hold {
				c.hold[i] = 0
			}
		}
		return PauseNone
	}
	if pressed&0x40 != 0 {
		p.selected = (p.selected + 2) % 3
		p.result = 0
	}
	if pressed&0x80 != 0 {
		p.selected = (p.selected + 1) % 3
		p.result = 0
	}
	if pressed&1 == 0 {
		return PauseNone
	}
	switch p.selected {
	case 0, 1:
		if !p.saving {
			p.result = 10
			return PauseNone
		}
		if p.selected == 0 {
			return PauseLoad
		}
		return PauseSave
	case 2:
		p.ended = true
		if c.nativeAudio != nil {
			c.nativeAudio.Stop()
		} else {
			hw.StopAllPSG()
		}
		return PauseExit
	}
	return PauseNone
}

func (c *Context) PauseSaveResult(result int) {
	if result == SaveOK {
		c.pause.result = 1
		c.pause.wait = 150
	} else {
		c.pause.result = -1
	}
}

func (c *Context) PauseLoadResult(result int) {
	switch result {
	case SaveOK:
		c.pause.result = 2
	case SaveEmpty:
		c.pause.result = 3
	case SaveWrongKey:
		c.pause.result = 4
	case SaveCorrupt:
		c.pause.result = 5
	default:
		c.pause.result = -2
	}
}

// PreparePauseMenu builds the fixed system UI off screen. There are no Lua
// controls for its appearance, items, placement or Start behavior.
func (c *Context) PreparePauseMenu() {
	p := &c.pause
	if p.closing {
		return
	}
	v := &p.video
	key := uint32(p.selected) | uint32(uint8(p.result))<<8
	if p.ended {
		key |= 1 << 16
	}
	if p.saving {
		key |= 1 << 17
	}
	if p.wait > 0 {
		key |= 1 << 18
	}
	if v.prepared && v.key == key {
		return
	}
	v.key, v.prepared, v.dirty = key, true, true
	tall := p.ended || p.result != 0 || p.wait > 0
	v.panel(tall)
	if p.ended {
		v.centered("SESSION", 18, 4)
		v.centered("ENDED", 26, 4)
		if p.wait > 0 {
			v.centered("PLEASE WAIT", 44, 3)
		} else {
			v.centered("Power off", 42, 5)
			v.centered("or reset", 50, 5)
		}
		return
	}
	top := 8
	if tall {
		top = 0
	}
	v.centered("PAUSED", top+6, 5)
	for i, label := range [3]string{"Load", "Save", "Exit"} {
		y := top + 18 + i*10
		color := uint32(4)
		if i < 2 && !p.saving {
			color = 5
		}
		if uint8(i) == p.selected {
			v.rect(4, y-2, 56, 9, 2)
			v.rect(4, y-2, 1, 9, 3)
		}
		v.text(label, 10, y, color)
	}
	message := ""
	switch p.result {
	case 1:
		message = "Saved"
	case 2:
		message = "Loaded"
	case 3:
		message = "No save yet"
	case 4:
		message = "Wrong save"
	case 5:
		message = "Bad save"
	case -1:
		message = "Save failed"
	case -2:
		message = "Load failed"
	case 10:
		message = "Unavailable"
	}
	if message != "" {
		v.centered(message, 49, 4)
	}
	if p.wait > 0 {
		v.centered("Please wait", 57, 3)
	}
}

// PresentPauseMenu only transfers the prepared system OBJ at VBlank. The
// frozen game planes and logical drawing state are untouched.
func (c *Context) PresentPauseMenu() {
	// Keep a full transfer window, including one native-audio IRQ service.
	// DMA backups/uploads and restore must finish before active scanout.
	if hw.GetReg16(hw.VCOUNT) > 180 {
		hw.WaitVDraw()
		hw.WaitForPresentWindow()
	}
	start := hw.ReadFrameCycleCounter()
	p := &c.pause
	if p.closing {
		p.video.restore()
		p.active, p.closing = false, false
	} else {
		if !p.video.saved {
			p.video.capture()
		}
		p.video.present()
	}
	end := hw.ReadFrameCycleCounter()
	c.accountPostPaceWork(end-start, end)
}
