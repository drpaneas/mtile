package runtime

import "testing"

func TestDebugMailboxStableLayout(t *testing.T) {
	if debugHeapBase != debugWatchBase+DebugWatchSlots*debugWatchSize || debugHeapError+4 != debugOBJBase || debugOBJPoseIDs+8 != DebugMailboxSize {
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

func TestDebugMailboxOBJExtensionPreservesAllWatchesAndHeap(t *testing.T) {
	var mailbox DebugMailbox
	stats := PerformanceStats{HeapCapacity: 56 * 1024, HeapError: HeapOOM,
		OBJTelemetryValid: true, OBJStreamAvailable: true, OBJStreamResidencyValid: true,
		OBJStreamResidentBytes: 320, OBJStreamResidentSlots: 5, OBJUploadedBytes: 768,
		OBJStreamResidentPoseIDs: [4]uint16{65535, 2, 3, 4}}
	mailbox.Begin(stats)
	for index := 0; index < DebugWatchSlots; index++ {
		mailbox.Add("pose", IntVal(index))
	}
	mailbox.End()
	if debugGet32(mailbox[:], debugOBJBase) != debugOBJMagic || debugGet32(mailbox[:], debugOBJFlags) != debugOBJAvailable|debugOBJResidencySet {
		t.Fatal("OBJ extension marker or flags missing")
	}
	if debugGet32(mailbox[:], debugOBJResident) != 320 || debugGet32(mailbox[:], debugOBJSlots) != 5 || debugGet32(mailbox[:], debugOBJUploaded) != 768 {
		t.Fatal("OBJ byte/slot counters did not round trip")
	}
	for index, want := range stats.OBJStreamResidentPoseIDs {
		offset := debugOBJPoseIDs + index*2
		if got := uint16(mailbox[offset]) | uint16(mailbox[offset+1])<<8; got != want {
			t.Fatalf("pose ID%d=%d want%d", index, got, want)
		}
	}
	if mailbox[debugWatchCount] != DebugWatchSlots || debugGet32(mailbox[:], debugWatchBase+15*debugWatchSize+20) != uint32(FromInt(15)) || debugGet32(mailbox[:], debugHeapError) != uint32(HeapOOM) {
		t.Fatal("OBJ extension overwrote legacy watches or heap telemetry")
	}
	mailbox.Begin(PerformanceStats{})
	mailbox.End()
	for _, value := range mailbox[debugOBJBase:] {
		if value != 0 {
			t.Fatal("unavailable extension retained stale data")
		}
	}
}
