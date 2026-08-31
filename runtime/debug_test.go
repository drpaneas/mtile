package runtime

import "testing"

func TestDebugMailboxStableLayout(t *testing.T) {
	if debugHeapBase != debugWatchBase+DebugWatchSlots*debugWatchSize || debugHeapError+4 > DebugMailboxSize {
		t.Fatalf("heap telemetry overlaps legacy watches or mailbox end: base=%d end=%d", debugHeapBase, debugHeapError+4)
	}
	var mailbox DebugMailbox
	stats := PerformanceStats{
		Frames: 7, MissedFrames: 2, LastFrame: 99, LastUpdate: 11, LastDraw: 22, DiagnosticSkips: 3,
		HeapCapacity: 56 * 1024, HeapAllocated: 41 * 1024, HeapLive: 24 * 1024, HeapPeakLive: 28 * 1024,
		HeapHandles: 321, HeapHandleLimit: 2048, HeapError: HeapOOM,
	}
	mailbox.Begin(stats)
	mailbox.Add("score", NumVal(Number(3*One+One/2)))
	mailbox.Add("alive", BoolVal(true))
	mailbox.End()
	if string(mailbox[:4]) != "GLUD" || debugGet32(mailbox[:], 12) != 7 || debugGet32(mailbox[:], 16) != 2 {
		t.Fatalf("mailbox header/stats were not encoded: %x", mailbox[:24])
	}
	if debugGet32(mailbox[:], debugSequenceOffset)&1 != 0 {
		t.Fatal("published mailbox sequence is odd")
	}
	if mailbox[debugWatchCount] != 2 || string(mailbox[debugWatchBase:debugWatchBase+5]) != "score" {
		t.Fatalf("watch metadata = %x", mailbox[debugWatchBase:debugWatchBase+48])
	}
	if got := int32(debugGet32(mailbox[:], debugWatchBase+20)); got != int32(3*One+One/2) {
		t.Fatalf("fixed point watch = %d", got)
	}
	if got := debugGet32(mailbox[:], debugHeapCapacity); got != 56*1024 {
		t.Fatalf("heap capacity = %d", got)
	}
	if got := debugGet32(mailbox[:], debugHeapAllocated); got != 41*1024 {
		t.Fatalf("heap allocated = %d", got)
	}
	if got := debugGet32(mailbox[:], debugHeapLive); got != 24*1024 {
		t.Fatalf("heap live = %d", got)
	}
	if got := debugGet32(mailbox[:], debugHeapPeakLive); got != 28*1024 {
		t.Fatalf("heap peak live = %d", got)
	}
	if got := debugGet32(mailbox[:], debugHeapHandles); got != 321 {
		t.Fatalf("heap handles = %d", got)
	}
	if got := debugGet32(mailbox[:], debugHeapHandleLimit); got != 2048 {
		t.Fatalf("heap handle limit = %d", got)
	}
	if got := HeapError(debugGet32(mailbox[:], debugHeapError)); got != HeapOOM {
		t.Fatalf("heap error = %d", got)
	}
}
