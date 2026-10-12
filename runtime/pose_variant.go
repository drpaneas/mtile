package runtime

// PVariantV selects a reusable pose color variant for subsequent pspr/anim
// calls. Nil or zero resets it. Unknown names/IDs reset to the default.
// It does not affect shared sprites, maps, or the global drawing palette.
func (c *Context) PVariantV(value Value) {
	if c == nil {
		return
	}
	c.poseVariant = 0
	if c.cart == nil {
		return
	}
	if value.Kind == KindNumber {
		id := value.Int()
		if id > 0 && id <= len(c.cart.PoseVariantNames) {
			c.poseVariant = id
		}
		return
	}
	name := c.stringBytes(value)
	for i, v := range c.cart.PoseVariantNames {
		if len(name) != len(v) {
			continue
		}
		equal := true
		for j := range name {
			if name[j] != v[j] {
				equal = false
				break
			}
		}
		if equal {
			c.poseVariant = i + 1
			return
		}
	}
}

// PVariantID is the compiler target for literal names and numeric selections.
func (c *Context) PVariantID(id int) {
	if c == nil {
		return
	}
	c.poseVariant = 0
	if c.cart != nil && id > 0 && id <= len(c.cart.PoseVariantNames) {
		c.poseVariant = id
	}
}
func (c *Context) PVariantStaticV(value Value) {
	if value.Kind != KindStaticString {
		c.PVariantID(value.Int())
		return
	}
	name := c.staticStringBytes(value)
	c.PVariantID(0)
	if c.cart == nil {
		return
	}
	for i, v := range c.cart.PoseVariantNames {
		if len(name) != len(v) {
			continue
		}
		equal := true
		for j := range name {
			if name[j] != v[j] {
				equal = false
				break
			}
		}
		if equal {
			c.PVariantID(i + 1)
			return
		}
	}
}
