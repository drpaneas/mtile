package runtime

import "github.com/drpaneas/mtile/runtime/hw"

// LoadPalette installs a complete native BGR555 palette. A nil pointer is a
// no-op. Entries not otherwise remapped are written exactly as supplied.
func (c *Context) LoadPalette(colors *[256]uint16) {
	c.ensurePaletteState()
	if colors == nil {
		return
	}
	copy(c.palette[:], colors[:])
	c.paletteDirty = true
}

// PaletteEntry exposes the active native color for host tooling and tests.
func (c *Context) PaletteEntry(index int) uint16 {
	if index < 0 || index >= 256 {
		return 0
	}
	c.ensurePaletteState()
	return c.palette[index]
}

// Pal remaps color from onto to on one palette layer. layer 0 is draw-time;
// layer 1 is display-time. Any other layer (including pal(2) / fillp) is
// rejected. The transpiler supplies defaults, so this is never variadic.
func (c *Context) Pal(from, to, layer int) {
	if from < 0 || from > 255 || to < 0 || to > 255 || (layer != 0 && layer != 1) {
		return
	}
	c.ensurePaletteState()
	if layer == 0 {
		c.drawPal[from] = byte(to)
		c.refreshSpriteFastFlags()
		return
	}
	c.displayPal[from] = byte(to)
	c.paletteDirty = true
}

// PalLayerReset restores identity on one remap layer (the pal(p) form).
func (c *Context) PalLayerReset(layer int) {
	if layer != 0 && layer != 1 {
		return
	}
	c.ensurePaletteState()
	if layer == 0 {
		for i := 0; i < 256; i++ {
			c.drawPal[i] = byte(i)
		}
		c.drawPalIdentity = true
		return
	}
	for i := 0; i < 256; i++ {
		c.displayPal[i] = byte(i)
	}
	c.paletteDirty = true
}

// PalReset restores both remap layers and the default transparency table.
func (c *Context) PalReset() {
	c.ensurePaletteState()
	for i := 0; i < 256; i++ {
		c.drawPal[i] = byte(i)
		c.displayPal[i] = byte(i)
		c.transp[i] = false
	}
	c.transp[0] = true
	c.paletteDirty = true
	c.transpKey0 = true
	c.drawPalIdentity = true
}

// PaltColor changes source-color transparency for sprite, sspr, and map
// operations. It does not affect primitives or cls.
func (c *Context) PaltColor(color int, transparent bool) {
	if color < 0 || color > 255 {
		return
	}
	c.ensurePaletteState()
	c.transp[color] = transparent
	c.refreshSpriteFastFlags()
}

func (c *Context) PaltReset() {
	c.ensurePaletteState()
	for i := range c.transp {
		c.transp[i] = false
	}
	c.transp[0] = true
	c.transpKey0 = true
}

func (c *Context) refreshSpriteFastFlags() {
	identity := true
	key0 := c.transp[0]
	for i := 0; i < 256; i++ {
		if c.drawPal[i] != byte(i) {
			identity = false
		}
		if i > 0 && c.transp[i] {
			key0 = false
		}
	}
	c.drawPalIdentity = identity
	c.transpKey0 = key0
}

// ApplyDisplayPalette updates all 256 hardware entries. It uses 16-bit writes
// because byte writes to GBA palette RAM duplicate across the halfword.
func (c *Context) ApplyDisplayPalette() {
	c.ensurePaletteState()
	if !c.paletteDirty {
		return
	}
	for i := 0; i < 256; i += 2 {
		lo := c.palette[c.displayPal[i]]
		hi := c.palette[c.displayPal[i+1]]
		hw.SetPaletteRAM32(i, lo, hi)
	}
	c.paletteDirty = false
}
