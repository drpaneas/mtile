//go:build !mtile_mode0 && mtile_no_map_raster

package runtime

// The compiler enables this tag only when it emits no small-map raster
// storage. Inlining these stubs removes the unused cache and its ARM helpers.
func (c *Context) invalidateMapRaster() {}
func (c *Context) PrepareMapRaster()    {}
func (c *Context) drawMapRaster(bg, storageBG, cellX, cellY, mx0, mx1, my0, my1, baseX, baseY int) bool {
	return false
}
