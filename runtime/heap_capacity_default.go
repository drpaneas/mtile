//go:build !mtile_heapless

package runtime

const (
	heapHandleSlots = 2048
	heapGlobalSlots = 256
	heapLocalSlots  = MaxLocalRoots
	heapTempSlots   = 64
)
