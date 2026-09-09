//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

type mapLayerViewRenderState struct{} //nolint:unused // Empty Mode 4 counterpart of the hardware renderer cache.

func (view *MapLayerView) borrowRenderState(c *Context, layer int) {}

func (view *MapLayerView) mapLayerViewChanged(x, y int, id uint16) {
	view.context.mapLayerMutated(view.layer, x, y, id)
}
