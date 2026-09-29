package runtime

// DebugSnapshot adds preview-only asset residency to the normal profiler
// snapshot. Generated release loops never call it, so the scan and its code
// are absent from release render work.
func (c *Context) DebugSnapshot() PerformanceStats {
	stats := c.PerformanceStats()
	stats.OBJTelemetryValid = true
	for index := range stats.OBJStreamResidentPoseIDs {
		stats.OBJStreamResidentPoseIDs[index] = 0xffff
	}
	c.populateOBJDebugStats(&stats)
	return stats
}
