package runtime

// DebugMailboxSize is a fixed preview-only EWRAM contract shared with the
// headless mGBA helper. Production builds do not emit a mailbox variable.
const (
	DebugMailboxSize = 512
	DebugWatchSlots  = 16
)

const (
	debugSequenceOffset = 8
	debugWatchCount     = 68
	debugWatchBase      = 72
	debugWatchSize      = 24
	// Heap telemetry is appended after all legacy watch slots. Keeping the
	// original header and watch offsets stable lets old editors ignore these
	// fields and new editors read zeroes from old preview ROMs.
	debugHeapBase        = debugWatchBase + DebugWatchSlots*debugWatchSize
	debugHeapCapacity    = debugHeapBase
	debugHeapAllocated   = debugHeapBase + 4
	debugHeapLive        = debugHeapBase + 8
	debugHeapPeakLive    = debugHeapBase + 12
	debugHeapHandles     = debugHeapBase + 16
	debugHeapHandleLimit = debugHeapBase + 20
	debugHeapError       = debugHeapBase + 24
)

// DebugMailbox is deliberately a byte array: its wire layout is independent
// of Go/TinyGo struct padding and can be read directly through the GBA bus.
type DebugMailbox [DebugMailboxSize]byte

func debugPut16(buffer []byte, offset int, value uint16) {
	buffer[offset] = byte(value)
	buffer[offset+1] = byte(value >> 8)
}

func debugPut32(buffer []byte, offset int, value uint32) {
	buffer[offset] = byte(value)
	buffer[offset+1] = byte(value >> 8)
	buffer[offset+2] = byte(value >> 16)
	buffer[offset+3] = byte(value >> 24)
}

func debugGet32(buffer []byte, offset int) uint32 {
	return uint32(buffer[offset]) | uint32(buffer[offset+1])<<8 | uint32(buffer[offset+2])<<16 | uint32(buffer[offset+3])<<24
}

// Begin captures real runtime counters and starts a new coherent snapshot.
func (m *DebugMailbox) Begin(stats PerformanceStats) {
	if m == nil {
		return
	}
	sequence := debugGet32(m[:], debugSequenceOffset) + 1
	copy(m[0:4], "GLUD")
	debugPut16(m[:], 4, 1)
	debugPut16(m[:], 6, DebugMailboxSize)
	debugPut32(m[:], debugSequenceOffset, sequence|1)
	values := [...]uint32{
		stats.Frames, stats.MissedFrames, stats.LastFrame, stats.MaxFrame,
		stats.LastUpdate, stats.MaxUpdate, stats.LastDraw, stats.MaxDraw,
		stats.GCCount, stats.LastGC, stats.MaxGC, stats.LastBlit,
	}
	for index, value := range values {
		debugPut32(m[:], 12+index*4, value)
	}
	debugPut16(m[:], 60, stats.PresentStart)
	debugPut16(m[:], 62, stats.PresentEnd)
	debugPut32(m[:], 64, stats.DiagnosticSkips)
	debugPut32(m[:], debugHeapCapacity, stats.HeapCapacity)
	debugPut32(m[:], debugHeapAllocated, stats.HeapAllocated)
	debugPut32(m[:], debugHeapLive, stats.HeapLive)
	debugPut32(m[:], debugHeapPeakLive, stats.HeapPeakLive)
	debugPut32(m[:], debugHeapHandles, stats.HeapHandles)
	debugPut32(m[:], debugHeapHandleLimit, stats.HeapHandleLimit)
	debugPut32(m[:], debugHeapError, uint32(stats.HeapError))
	m[debugWatchCount] = 0
}

// Add records one named top-level Lua global. Numbers retain their raw 16:16
// fixed-point representation; booleans use 0/1; other kinds remain truthful
// type-only watches rather than fabricated string/table previews.
func (m *DebugMailbox) Add(name string, value Value) {
	if m == nil || int(m[debugWatchCount]) >= DebugWatchSlots {
		return
	}
	index := int(m[debugWatchCount])
	offset := debugWatchBase + index*debugWatchSize
	for at := 0; at < 16; at++ {
		m[offset+at] = 0
	}
	for at := 0; at < len(name) && at < 15; at++ {
		m[offset+at] = name[at]
	}
	m[offset+16] = byte(value.Kind)
	debugPut32(m[:], offset+20, uint32(int32(value.Num)))
	m[debugWatchCount]++
}

// End publishes the snapshot by making the sequence even.
func (m *DebugMailbox) End() {
	if m == nil {
		return
	}
	sequence := debugGet32(m[:], debugSequenceOffset)
	debugPut32(m[:], debugSequenceOffset, (sequence+1)&^1)
}
