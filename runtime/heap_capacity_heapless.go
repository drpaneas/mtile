//go:build mtile_heapless

package runtime

// A heapless target cannot reach any allocator or GC-root operation. Retain
// one slot of each kind so the shared runtime still type-checks, while the
// linker removes the unreachable methods and the large metadata arrays do not
// consume EWRAM inside Context.
const (
	heapHandleSlots = 1
	heapGlobalSlots = 1
	heapLocalSlots  = 1
	heapTempSlots   = 1
)
