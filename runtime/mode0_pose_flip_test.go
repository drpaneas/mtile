//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_no_pspr

package runtime

import (
	"bytes"
	"slices"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0AutomaticFlipKeepsResidentPoseAndUsesHardwareReflection(t *testing.T) {
	calls := animationFlipCallsForTest()
	calls = append(calls,
		animationFlipCallForTest{"pspr", func(c *Context, _ int, _ Value, x, y bool, _ Value) { c.PSpr(0, 80, 64, x, y) }},
		animationFlipCallForTest{"pspr-site", func(c *Context, _ int, _ Value, x, y bool, _ Value) { c.PSprSite(0, 0, 0, 80, 64, x, y) }},
		animationFlipCallForTest{"pspr-base", func(c *Context, _ int, _ Value, x, y bool, _ Value) { c.PSprSiteFromBase(0, 0, 4, 0, 80, 64, x, y) }},
	)
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			c := plannedMode0PoseFixture(t, rectangularPose(2, 2))
			c.Camera(3, 5)
			c.cart.AnimationClips = []AnimationClip{{Name: "idle", Count: 1, FPS: 8, Loop: true, FlipOffsetX: 16, FlipOffsetY: 16}}
			c.cart.AnimationFrames = []uint16{0}
			name := c.InternStatic("idle")
			c.BeginRenderFrame()
			c.Cls(0)
			call.draw(c, 0, name, false, false, NilVal())
			c.PrepareRender()
			c.PresentRender()
			original := composeMode0Objects(t)
			stage := slices.Clone(c.cart.PoseOBJStage)
			var vram [64]uint32
			for i := range vram {
				vram[i] = hw.ReadVRAM32(hw.OBJVRAM + uintptr(i*4))
			}
			for flip := 1; flip <= 3; flip++ {
				c.BeginRenderFrame()
				c.Cls(0)
				call.draw(c, 0, name, flip&1 != 0, flip&2 != 0, NilVal())
				if c.mode0PsprPending[0].dirty || c.hasMode0PsprUploads() {
					t.Fatal("changing only facing dirtied the cached pose")
				}
				c.PrepareRender()
				c.PresentRender()
				if c.RendererUploadStats().LastOBJTileBytes != 0 || !slices.Equal(stage, c.cart.PoseOBJStage) {
					t.Fatal("changing only facing restaged or uploaded pose pixels")
				}
				for i, word := range vram {
					if hw.ReadVRAM32(hw.OBJVRAM+uintptr(i*4)) != word {
						t.Fatal("hardware flip changed the resident tile data")
					}
				}
				want := reflectedFrameForTest(original, 80-3, 64-5, 16, 16, flip&1 != 0, flip&2 != 0)
				if !bytes.Equal(composeMode0Objects(t), want) {
					t.Fatal("hardware flip moved the pose out of its original rectangle")
				}
			}
		})
	}
}
