//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_no_pspr

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func tileLinkFrameForTest(t *testing.T, c *Context, camX, camY, w, h, mask int) []byte {
	t.Helper()
	requireMode0LayerCount(t, 1)
	if c.cart.PoseOBJStage == nil {
		hw.ResetForTest()
		c.cart.PoseOBJStage = make([]uint32, mode0PsprWindowLimit*16)
		c.InitHardware()
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.Camera(camX, camY)
	c.SetTileLinkSites(0, 0, 2)
	c.Map(0, 0, 0, 0, w, h, mask)
	c.PresentRender()
	return composeMode0Objects(t)
}
