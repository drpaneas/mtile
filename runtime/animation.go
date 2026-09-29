package runtime

// AnimationClip names immutable pose IDs, independent of their tile storage.
type AnimationClip struct {
	Name         string
	Start, Count uint32
	FPS          uint8
	Loop         bool
	// Union bounds of all nonempty poses in this clip, computed at build time.
	// A single reflection area prevents frame widths from moving the anchor.
	FlipOffsetX, FlipOffsetY int32
}

func animationIndexValid(image *AssetImage) bool {
	for _, clip := range image.AnimationClips {
		if clip.Name == "" || clip.FPS < 1 || clip.FPS > 60 || uint64(clip.Start)+uint64(clip.Count) > uint64(len(image.AnimationFrames)) {
			return false
		}
	}
	for _, id := range image.AnimationFrames {
		if int(id) >= len(image.PoseIndex) {
			return false
		}
	}
	return true
}

// animationPose selects without allocating or maintaining per-entity state.
// Nil elapsed follows the update clock; an explicit value starts at zero.
func (c *Context) animationPose(name, elapsed Value) int {
	return c.animationPoseID(c.animationClip(name), elapsed)
}

func (c *Context) animationClip(name Value) int {
	if c == nil || c.cart == nil {
		return -1
	}
	return c.animationClipNamed(c.stringBytes(name))
}

func (c *Context) animationPoseStatic(name, elapsed Value) int {
	return c.animationPoseID(c.animationClipStatic(name), elapsed)
}

func (c *Context) animationClipStatic(name Value) int {
	if c == nil || c.cart == nil {
		return -1
	}
	return c.animationClipNamed(c.staticStringBytes(name))
}

func (c *Context) animationClipNamed(name []byte) int {
	for id, clip := range c.cart.AnimationClips {
		if len(name) != len(clip.Name) {
			continue
		}
		match := true
		for i, b := range name {
			if b != clip.Name[i] {
				match = false
				break
			}
		}
		if match {
			return id
		}
	}
	return -1
}

// The compiler resolves literal clip names to this allocation-free path.
// Keeping it separate from name decoding lets the linker discard string/heap
// machinery entirely for pose-only cartridges.
func (c *Context) animationPoseID(id int, elapsed Value) int {
	if c == nil || c.cart == nil || id < 0 || id >= len(c.cart.AnimationClips) {
		return -1
	}
	clip := c.cart.AnimationClips[id]
	if clip.Count == 0 || clip.FPS == 0 || uint64(clip.Start)+uint64(clip.Count) > uint64(len(c.cart.AnimationFrames)) {
		return -1
	}
	var frame uint64
	if elapsed.Kind == KindNil {
		rate := c.tickRate
		if rate == 0 {
			rate = 60
		}
		frame = uint64(c.updateTick) * uint64(clip.FPS) / uint64(rate)
	} else if elapsed.Num > 0 && elapsed.Kind == KindNumber {
		frame = uint64(elapsed.Num) * uint64(clip.FPS) >> 16
	}
	if clip.Loop {
		frame %= uint64(clip.Count)
	} else if frame >= uint64(clip.Count) {
		frame = uint64(clip.Count) - 1
	}
	return int(c.cart.AnimationFrames[uint64(clip.Start)+frame])
}

func (c *Context) AnimV(name, x, y, flipX, flipY, elapsed Value) {
	c.animSite(0, 0, c.animationClip(name), x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) AnimSiteV(site, windowBase, name, x, y, flipX, flipY, elapsed Value) {
	c.animSite(site.Int(), windowBase.Int(), c.animationClip(name), x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) AnimSiteFromBaseV(base, windowBase, stride, name, x, y, flipX, flipY, elapsed Value) {
	c.animSiteFromBase(base.Int(), windowBase.Int(), stride.Int(), c.animationClip(name), x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

// AnimIDV and its site variants are internal compiler targets: clip IDs use
// native integers, independent of Lua's signed 16.16 numeric range.
func (c *Context) AnimIDV(id int, x, y, flipX, flipY, elapsed Value) {
	c.animSite(0, 0, id, x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) AnimIDSiteV(site, windowBase Value, id int, x, y, flipX, flipY, elapsed Value) {
	c.animSite(site.Int(), windowBase.Int(), id, x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) AnimIDSiteFromBaseV(base, windowBase, stride Value, id int, x, y, flipX, flipY, elapsed Value) {
	c.animSiteFromBase(base.Int(), windowBase.Int(), stride.Int(), id, x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

// Heapless programs may still choose among static names dynamically. This
// decoder deliberately cannot reach allocated strings or the Lua semispaces.
func (c *Context) AnimStaticV(name, x, y, flipX, flipY, elapsed Value) {
	c.animSite(0, 0, c.animationClipStatic(name), x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) AnimStaticSiteV(site, windowBase, name, x, y, flipX, flipY, elapsed Value) {
	c.animSite(site.Int(), windowBase.Int(), c.animationClipStatic(name), x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) AnimStaticSiteFromBaseV(base, windowBase, stride, name, x, y, flipX, flipY, elapsed Value) {
	c.animSiteFromBase(base.Int(), windowBase.Int(), stride.Int(), c.animationClipStatic(name), x.Int(), y.Int(), flipX.IsTrue(), flipY.IsTrue(), elapsed)
}

func (c *Context) animSite(site, windowBase, clipID, x, y int, flipX, flipY bool, elapsed Value) {
	poseID := c.animationPoseID(clipID, elapsed)
	if poseID >= 0 && (flipX || flipY) {
		clip := &c.cart.AnimationClips[clipID]
		if flipX {
			x += int(clip.FlipOffsetX)
		}
		if flipY {
			y += int(clip.FlipOffsetY)
		}
	}
	// Apply the clip's common reflection area exactly once. Calling PSpr here
	// would add the current pose's narrower bounds and move the image again.
	c.drawPoseSite(site, windowBase, poseID, x, y, flipX, flipY)
}

func (c *Context) animSiteFromBase(base, windowBase, stride, clipID, x, y int, flipX, flipY bool, elapsed Value) {
	site := c.TakePSprSite(base)
	window := windowBase + (site-base)*stride
	c.animSite(site, window, clipID, x, y, flipX, flipY, elapsed)
}
