//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_pspr

package runtime

func (c *Context) populateOBJDebugStats(stats *PerformanceStats) {
	if c == nil {
		return
	}
	stats.OBJStreamAvailable = true
	// This existing counter includes all OBJ tile transfers in the last
	// completed presentation: streamed poses, packed sprites, mutable shared
	// art, and unchanged gaps covered by a coalesced DMA. It excludes OAM.
	stats.OBJUploadedBytes = c.mode0UploadStats.LastOBJTileBytes
	if c.cart == nil || c.mode0DrawActive || c.hasMode0PsprUploads() {
		// Prepare records the new pose ID before its staged pixels reach VRAM.
		// Do not mistake that pending state for a loaded cache entry.
		return
	}
	stats.OBJStreamResidencyValid = true
	if len(c.cart.PoseOBJResident) != 0 {
		if !c.mode0PoseResidentReady {
			stats.OBJStreamResidencyValid = false
			return
		}
		stats.OBJStreamResidentBytes = uint32(len(c.cart.PoseOBJResident) * 4)
		for id := range c.cart.PoseOBJIndex {
			if c.cart.PoseOBJIndex[id].Count == 0 {
				continue
			}
			if stats.OBJStreamResidentSlots < uint32(len(stats.OBJStreamResidentPoseIDs)) {
				stats.OBJStreamResidentPoseIDs[stats.OBJStreamResidentSlots] = uint16(id)
			}
			stats.OBJStreamResidentSlots++
		}
		return
	}
	// Index the live windows directly. A two-value range over the fixed owner
	// array copies all 512 bytes; on TinyGo ARM that becomes a large scalar
	// aggregate with expensive extraction/spills. Only active cached windows
	// need examining, and indexed owner reads preserve their single storage.
	for site := 0; site < c.mode0PsprSiteCount; site++ {
		pending := &c.mode0PsprPending[site]
		if pending.n <= 0 || pending.dirty || pending.id < 0 || pending.id > 0xffff ||
			c.mode0PsprLastID[site] != pending.id || pending.base < 0 || pending.base > mode0PsprWindowLimit-pending.n {
			continue
		}
		// Owner/range checks exclude stale tail tiles after a smaller pose or
		// relocated window replaces an older cache entry. Counting each owned
		// destination once also handles overlapping direct-runtime windows.
		var bytes uint32
		for tile := pending.base; tile < pending.base+pending.n; tile++ {
			if c.mode0PsprOwner[tile] == uint8(site+1) {
				bytes += 64
			}
		}
		if bytes == 0 {
			continue
		}
		stats.OBJStreamResidentBytes += bytes
		if stats.OBJStreamResidentSlots < uint32(len(stats.OBJStreamResidentPoseIDs)) {
			stats.OBJStreamResidentPoseIDs[stats.OBJStreamResidentSlots] = uint16(pending.id)
		}
		stats.OBJStreamResidentSlots++
	}
}
