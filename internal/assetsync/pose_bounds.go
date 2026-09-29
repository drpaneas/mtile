package assetsync

// PoseFlipBounds accumulates local 8x8 tile bounds while building cartridge
// metadata. Empty poses do not introduce an origin into a clip's union bounds.
type PoseFlipBounds struct {
	minX, minY, maxX, maxY int32
	hasParts               bool
}

func (b *PoseFlipBounds) AddTile(x, y int16) {
	b.Include(PoseFlipBounds{minX: int32(x), minY: int32(y), maxX: int32(x) + 8, maxY: int32(y) + 8, hasParts: true})
}

func (b *PoseFlipBounds) Include(other PoseFlipBounds) {
	if !other.hasParts {
		return
	}
	if !b.hasParts {
		*b = other
		return
	}
	b.minX, b.minY = min(b.minX, other.minX), min(b.minY, other.minY)
	b.maxX, b.maxY = max(b.maxX, other.maxX), max(b.maxY, other.maxY)
}

// Offsets returns the translations added to the existing origin-mirror rule.
// Keep the sums in int32: an int16 part at 32767 has an offset of 65542.
func (b PoseFlipBounds) Offsets() (x, y int32) {
	if b.hasParts {
		return b.minX + b.maxX, b.minY + b.maxY
	}
	return 0, 0
}
