//go:build mtile_mode0

package runtime

import "github.com/drpaneas/mtile/runtime/audio"

// mode0RequiredCommitCycles bounds the immutable, clean-palette commit by its
// pending map transfers. Native first-scan fixtures cover four BGs and all
// 128 OBJs, including overlapping OAM compaction at intermediate counts.
// A warm commit measured below 16,400 cycles and each complete map below
// 8,100 extra cycles; these bounds retain slack for code layout/control work.
// Native Audio v2 can preempt a commit with one 256 Hz IRQ, or two when the
// warm work itself can span a service period minus one IRQ budget. Mutable
// tiles, cold work, palette changes and pending packed tiles retain the full
// window because those transfers are not covered by the warm map bound.
func (c *Context) mode0RequiredCommitCycles() uint32 {
	if c == nil || !mode0SheetImmutable || !c.paletteReady || c.paletteDirty ||
		c.mode0ColdCycles != 0 || c.hasMode0PackedUploads() || c.hasMode0PsprUploads() {
		return mode0CommitReserve
	}
	cycles := uint32(22000)
	for layer := 0; layer < mode0LayerCount; layer++ {
		if c.mode0MapSeen&(1<<uint(layer)) != 0 && c.mode0MapDirty[layer] {
			cycles += 9000
		}
	}
	if c.nativeAudio != nil {
		return mode0CommitWithNativeAudio(cycles)
	}
	return cycles
}

func mode0CommitWithNativeAudio(warm uint32) uint32 {
	irqs := uint32(1)
	if warm > audio.NativeServicePeriodCycles-audio.NativeServiceCycleBudget {
		irqs = 2
	}
	reserve := warm + irqs*audio.NativeServiceCycleBudget
	if reserve < warm || reserve > mode0CommitReserve {
		return mode0CommitReserve
	}
	return reserve
}
