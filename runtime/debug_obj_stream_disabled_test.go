//go:build !mtile_mode0 || !mtile_mode0_obj || !mtile_mode0_pspr

package runtime

import "testing"

func TestDebugSnapshotWithoutStreamDistinguishesUnavailable(t *testing.T) {
	var c Context
	stats := c.DebugSnapshot()
	if !stats.OBJTelemetryValid || stats.OBJStreamAvailable || stats.OBJStreamResidencyValid || stats.OBJStreamResidentBytes != 0 || stats.OBJUploadedBytes != 0 {
		t.Fatalf("non-streamed renderer must report unavailable, not an empty stream: %+v", stats)
	}
	if stats.OBJStreamResidentPoseIDs != [4]uint16{0xffff, 0xffff, 0xffff, 0xffff} {
		t.Fatalf("unused IDs = %v", stats.OBJStreamResidentPoseIDs)
	}
}
