package emulator

import (
	"encoding/binary"
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
)

func TestOBJTelemetryRuntimeWireRoundTrip(t *testing.T) {
	if debugMailboxSize != rt.DebugMailboxSize || debugOBJPoseIDs+8 != debugMailboxSize || telemetryPayloadSize != debugMailboxOffset+debugMailboxSize {
		t.Fatal("OBJ extension changed the player mailbox/payload contract")
	}
	for _, tc := range []struct {
		name                string
		available, resident bool
	}{
		{"mode0 resident", true, true},
		{"mode0 staged", true, false},
		{"mode4 unavailable", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mailbox rt.DebugMailbox
			mailbox.Begin(rt.PerformanceStats{Frames: 12, HeapCapacity: 56 * 1024, HeapError: rt.HeapOOM,
				OBJTelemetryValid: true, OBJStreamAvailable: tc.available, OBJStreamResidencyValid: tc.resident,
				OBJStreamResidentBytes: 384, OBJStreamResidentSlots: 6, OBJUploadedBytes: 704,
				OBJStreamResidentPoseIDs: [4]uint16{65535, 10, 20, 30}})
			for index := 0; index < rt.DebugWatchSlots; index++ {
				mailbox.Add("pose", rt.IntVal(index))
			}
			mailbox.End()
			payload := testTelemetryPayload()
			copy(payload[debugMailboxOffset:], mailbox[:])
			got := decodeTelemetry(payload)
			if !got.DebugValid || !got.Stats.OBJTelemetryValid || got.Stats.OBJStreamAvailable != tc.available || got.Stats.OBJStreamResidencyValid != tc.resident {
				t.Fatalf("capability/state decode: %+v", got.Stats)
			}
			if got.Stats.OBJStreamResidentBytes != 384 || got.Stats.OBJStreamResidentSlots != 6 || got.Stats.OBJUploadedBytes != 704 || got.Stats.OBJStreamResidentPoseIDs != [4]uint16{65535, 10, 20, 30} {
				t.Fatalf("OBJ fields decode: %+v", got.Stats)
			}
			if got.Stats.HeapError != HeapOOM || got.Stats.HeapCapacity != 56*1024 || len(got.Watches) != 16 || got.Watches[15].Raw != int32(rt.FromInt(15)) {
				t.Fatal("OBJ extension damaged legacy heap/watch fields")
			}
		})
	}
}

func TestOBJTelemetryOldAndUnknownExtensionsStayUnavailable(t *testing.T) {
	for _, marker := range []uint32{0, 0x324a424f} { // old zero tail, future OBJ2
		payload := testTelemetryPayload()
		mailbox := payload[debugMailboxOffset:]
		binary.LittleEndian.PutUint32(mailbox[debugOBJBase:], marker)
		binary.LittleEndian.PutUint32(mailbox[debugOBJFlags:], ^uint32(0))
		binary.LittleEndian.PutUint32(mailbox[debugOBJResident:], 32000)
		got := decodeTelemetry(payload)
		if !got.DebugValid || got.Stats.OBJTelemetryValid || got.Stats.OBJStreamAvailable || got.Stats.OBJStreamResidencyValid || got.Stats.OBJStreamResidentBytes != 0 {
			t.Fatalf("unknown marker%08x invented live OBJ values: %+v", marker, got.Stats)
		}
		if got.Stats.HeapError != HeapOOM || len(got.Watches) != 1 {
			t.Fatal("unknown OBJ extension invalidated legacy telemetry")
		}
	}
}
