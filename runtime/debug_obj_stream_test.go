//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_pspr

package runtime

import "testing"

func TestDebugOBJResidencyWaitsForTransferAndPersistsWhenUndrawn(t *testing.T) {
	c := plannedMode0PoseFixture(t, rectangularPose(2, 2))
	if stats := c.DebugSnapshot(); !stats.OBJTelemetryValid || !stats.OBJStreamAvailable || !stats.OBJStreamResidencyValid || stats.OBJStreamResidentBytes != 0 {
		t.Fatalf("empty initialized stream: %+v", stats)
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 4, 0, 10, 10, false, false)
	c.PrepareRender()
	if stats := c.DebugSnapshot(); stats.OBJStreamResidencyValid || stats.OBJStreamResidentSlots != 0 {
		t.Fatalf("staged pixels reported as resident before DMA: %+v", stats)
	}
	c.PresentRender()
	stats := c.DebugSnapshot()
	if !stats.OBJStreamResidencyValid || stats.OBJStreamResidentBytes != 256 || stats.OBJStreamResidentSlots != 1 || stats.OBJUploadedBytes != 256 || stats.OBJStreamResidentPoseIDs != [4]uint16{0, 0xffff, 0xffff, 0xffff} {
		t.Fatalf("completed stream: %+v", stats)
	}
	// Cache residency is deliberately independent of this frame's OAM list.
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrepareRender()
	c.PresentRender()
	stats = c.DebugSnapshot()
	if stats.OBJStreamResidentBytes != 256 || stats.OBJStreamResidentSlots != 1 || stats.OBJUploadedBytes != 0 {
		t.Fatalf("undrawn cached pose was lost or reuploaded: %+v", stats)
	}
	if c.PerformanceStats().OBJTelemetryValid {
		t.Fatal("ordinary profiler snapshot should not scan or claim OBJ telemetry")
	}
	// An invalid pose clears the cache entry even though old physical pixels
	// remain in VRAM. Those pixels are no longer a reusable tracked pose.
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 4, -1, 10, 10, false, false)
	c.PresentRender()
	stats = c.DebugSnapshot()
	if stats.OBJStreamResidentBytes != 0 || stats.OBJStreamResidentSlots != 0 || stats.OBJStreamResidentPoseIDs != [4]uint16{0xffff, 0xffff, 0xffff, 0xffff} {
		t.Fatalf("invalidated window still counted: %+v", stats)
	}
}

func TestDebugOBJResidencyExcludesStaleTailAndRelocatedTiles(t *testing.T) {
	c := plannedMode0PoseFixture(t, rectangularPose(2, 2))
	c.cart.PoseIndex = append(c.cart.PoseIndex, PoseRange{Count: 1})
	c.cart.PoseOBJIndex = append(c.cart.PoseOBJIndex, PoseOBJRange{Start: 1, Count: 1, TileCount: 1})
	c.cart.PoseOBJParts = append(c.cart.PoseOBJParts, PoseOBJ{Width: 8, Height: 8})
	for _, frame := range []struct{ id, base, bytes int }{{0, 4, 256}, {1, 4, 64}, {1, 10, 64}} {
		c.BeginRenderFrame()
		c.Cls(0)
		c.PSprSite(0, frame.base, frame.id, 10, 10, false, false)
		c.PrepareRender()
		c.PresentRender()
		stats := c.DebugSnapshot()
		if stats.OBJStreamResidentBytes != uint32(frame.bytes) || stats.OBJStreamResidentSlots != 1 || stats.OBJStreamResidentPoseIDs[0] != uint16(frame.id) {
			t.Fatalf("pose%d at%d: %+v", frame.id, frame.base, stats)
		}
	}
}

func TestDebugOBJUploadsIncludeCoalescedGapsAndPoseIDsTruncateBySlot(t *testing.T) {
	c := plannedMode0PoseFixture(t, rectangularPose(1, 1))
	c.cart.PoseIndex = make([]PoseRange, 6)
	c.cart.PoseOBJIndex = make([]PoseOBJRange, 6)
	for id := range c.cart.PoseIndex {
		c.cart.PoseIndex[id] = PoseRange{Count: 1}
		c.cart.PoseOBJIndex[id] = PoseOBJRange{Count: 1, TileCount: 1}
	}
	c.BeginRenderFrame()
	c.Cls(0)
	for site := 0; site < 6; site++ {
		c.PSprSite(site, site*2, site, 10+site*20, 10, false, false)
	}
	c.PrepareRender()
	c.PresentRender()
	stats := c.DebugSnapshot()
	if stats.OBJStreamResidentBytes != 6*64 || stats.OBJUploadedBytes != 11*64 || stats.OBJStreamResidentSlots != 6 || stats.OBJStreamResidentPoseIDs != [4]uint16{0, 1, 2, 3} {
		t.Fatalf("resident windows, transfer gaps or ID prefix incorrect: %+v", stats)
	}
}

func TestDebugOBJResidentPose65535IsNotAnEmptySlot(t *testing.T) {
	c := plannedMode0PoseFixture(t, rectangularPose(1, 1))
	c.cart.PoseIndex = make([]PoseRange, 65536)
	c.cart.PoseOBJIndex = make([]PoseOBJRange, 65536)
	c.cart.PoseIndex[65535] = PoseRange{Count: 1}
	c.cart.PoseOBJIndex[65535] = PoseOBJRange{Count: 1, TileCount: 1}
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 65535, 10, 10, false, false)
	c.PrepareRender()
	c.PresentRender()
	stats := c.DebugSnapshot()
	if stats.OBJStreamResidentSlots != 1 || stats.OBJStreamResidentBytes != 64 || stats.OBJStreamResidentPoseIDs[0] != 65535 {
		t.Fatalf("maximum pose ID was mistaken for an unused ID: %+v", stats)
	}
}
