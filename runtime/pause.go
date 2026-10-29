package runtime

import "github.com/drpaneas/mtile/runtime/hw"

type pauseState struct {
	video             pauseVideo
	active, closing   bool
	enabled           bool
	previous, blocked uint16
}

func (c *Context) PauseActive() bool { return c.pause.active }

// PauseEnableV opts into the fixed system pause on subsequent input polls.
// It starts disabled. Non-boolean values leave the current setting intact.
func (c *Context) PauseEnableV(enabled Value) {
	if enabled.Kind == KindBool {
		c.pause.enabled = enabled.Truthy()
	}
}

func (c *Context) closePause(raw uint16) {
	c.pause.closing = true
	c.pause.blocked |= raw
	c.keys = 0
	c.prevKeys = 0
	for i := range c.hold {
		c.hold[i] = 0
	}
}

// PollPauseMenu always tracks input edges, but reserves Start only while
// enabled. Holding a menu's Start through enabling cannot trigger a pause.
func (c *Context) PollPauseMenu() {
	p := &c.pause
	raw := c.keys
	pressed := raw &^ p.previous
	p.previous = raw
	p.blocked &= raw
	c.keys = raw &^ p.blocked
	if c.saveWait > 0 {
		c.saveWait--
	}
	if p.enabled || p.active {
		c.keys &^= 8
	}
	if !p.enabled {
		if p.active {
			// Restore at the normal presentation window before gameplay runs.
			c.closePause(raw)
		}
		return
	}
	if pressed&8 == 0 {
		return
	}
	if !p.active {
		p.active, p.closing = true, false
		return
	}
	c.closePause(raw)
}

// PreparePauseMenu builds only the fixed, centered PAUSED message. Its font
// and spacing are system-owned and independent of the project's font.
func (c *Context) PreparePauseMenu() {
	if c.pause.closing || c.pause.video.prepared {
		return
	}
	v := &c.pause.video
	v.preparePaused()
	v.prepared, v.dirty = true, true
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
