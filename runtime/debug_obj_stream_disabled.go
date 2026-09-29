//go:build !mtile_mode0 || !mtile_mode0_obj || !mtile_mode0_pspr

package runtime

func (c *Context) populateOBJDebugStats(stats *PerformanceStats) {}
