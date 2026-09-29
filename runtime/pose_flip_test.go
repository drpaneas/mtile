//go:build !mtile_mode0

package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func newPoseFlipContextForTest(t *testing.T, parts []PosePart, poses []PoseRange) *Context {
	t.Helper()
	image, ram := poseFixture()
	image.PoseParts, image.PoseIndex = parts, poses
	image.ObjSheetW, image.ObjSheetH = 64, 8
	image.ObjSpriteCols, image.ObjSpriteRows, image.ObjSpriteN = 8, 1, 8
	image.ObjSheet, image.ObjSheetIndex = make([]byte, 8*64), make([]uint32, 8)
	for id := 0; id < 8; id++ {
		image.ObjSheetIndex[id] = uint32(id + 1)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if (x+y)%7 != 0 {
					image.ObjSheet[id*64+y*8+x] = byte(10 + id*20 + x + y)
				}
			}
		}
	}
	c := new(Context)
	if !c.Init(image, ram) {
		t.Fatal("pose flip fixture init")
	}
	c.Camera(3, 5)
	c.Pal(11, 201, 0)
	c.PaltColor(12, true)
	return c
}

func TestPSprFlipsInsideAuthoredBoundsWithCameraPaletteAndPartFlips(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		parts                    []PosePart
		left, top, width, height int
		xOffset, yOffset         int32
	}{
		{"positive", []PosePart{{Tile: 0, X: 8, Y: 16, Flags: PoseFlipX}, {Tile: 1, X: 24, Y: 32, Flags: PoseFlipY}}, 8, 16, 24, 24, 40, 56},
		{"negative", []PosePart{{Tile: 0, X: -24, Y: -16, Flags: PoseFlipY}, {Tile: 1, X: 8, Y: 16, Flags: PoseFlipX}, {Tile: 2, X: -8, Flags: PoseFlipX | PoseFlipY}}, -24, -16, 40, 40, -8, 8},
	} {
		for wrapper := 0; wrapper < 3; wrapper++ {
			for flip := 0; flip < 4; flip++ {
				t.Run(fmt.Sprintf("%s/wrapper%d/flip%d", tc.name, wrapper, flip), func(t *testing.T) {
					c := newPoseFlipContextForTest(t, tc.parts, []PoseRange{{Count: uint16(len(tc.parts)), FlipOffsetX: tc.xOffset, FlipOffsetY: tc.yOffset}})
					c.Cls(3)
					c.PSpr(0, 80, 64, false, false)
					want := reflectedFrameForTest(c.framebuffer[:], 80-3+tc.left, 64-5+tc.top, tc.width, tc.height, flip&1 != 0, flip&2 != 0)
					c.Cls(3)
					switch wrapper {
					case 0:
						c.PSprV(IntVal(0), IntVal(80), IntVal(64), BoolVal(flip&1 != 0), BoolVal(flip&2 != 0))
					case 1:
						c.PSprSiteV(IntVal(0), IntVal(0), IntVal(0), IntVal(80), IntVal(64), BoolVal(flip&1 != 0), BoolVal(flip&2 != 0))
					case 2:
						c.PSprSiteFromBaseV(IntVal(0), IntVal(0), IntVal(4), IntVal(0), IntVal(80), IntVal(64), BoolVal(flip&1 != 0), BoolVal(flip&2 != 0))
					}
					if !bytes.Equal(c.framebuffer[:], want) {
						t.Fatal("pose flip moved outside its original bounds or changed authored pixel/palette semantics")
					}
				})
			}
		}
	}
}

func TestAnimationAllDispatchPathsUseOneStableClipReflectionArea(t *testing.T) {
	for _, negative := range []bool{false, true} {
		var parts []PosePart
		for frame := 0; frame < 2; frame++ {
			for x := 0; x < 6+frame; x++ {
				px, py := x*8, frame*8
				if negative {
					px, py = px-8*(1-frame), py-8
				}
				parts = append(parts, PosePart{Tile: uint16(x), X: int16(px), Y: int16(py), Flags: uint8(x % 4)})
			}
		}
		poses := []PoseRange{poseRangeForTest(0, parts[:6]), poseRangeForTest(6, parts[6:])}
		left, top, width, height, offsetX, offsetY := 0, 0, 56, 16, int32(56), int32(16)
		if negative {
			left, top, width, height, offsetX, offsetY = -8, -8, 64, 16, 48, 0
		}
		for _, call := range animationFlipCallsForTest() {
			for frame := 0; frame < 2; frame++ {
				for flip := 0; flip < 4; flip++ {
					t.Run(fmt.Sprintf("negative%t/%s/frame%d/flip%d", negative, call.name, frame, flip), func(t *testing.T) {
						c := newPoseFlipContextForTest(t, parts, poses)
						c.cart.AnimationClips = []AnimationClip{{Name: "varied", Count: 2, FPS: 1, Loop: true, FlipOffsetX: offsetX, FlipOffsetY: offsetY}}
						c.cart.AnimationFrames = []uint16{0, 1}
						name := c.InternStatic("varied")
						if call.name == "name" || call.name == "name-site" || call.name == "name-base" {
							var err HeapError
							name, err = c.NewString([]byte("varied"))
							if err != HeapOK {
								t.Fatal(err)
							}
						}
						c.Cls(3)
						c.PSpr(frame, 80, 64, false, false)
						want := reflectedFrameForTest(c.framebuffer[:], 80-3+left, 64-5+top, width, height, flip&1 != 0, flip&2 != 0)
						c.Cls(3)
						call.draw(c, 0, name, flip&1 != 0, flip&2 != 0, IntVal(frame))
						if !bytes.Equal(c.framebuffer[:], want) {
							t.Fatal("animation used per-pose bounds, double adjustment, or changed the common clip anchor")
						}
					})
				}
			}
		}
	}
}

func TestAutomaticFlipInvalidAndEmptyDrawsAreNoOps(t *testing.T) {
	c := newPoseFlipContextForTest(t, nil, []PoseRange{{}})
	c.cart.AnimationClips = []AnimationClip{{Name: "empty", FPS: 1}}
	c.Cls(3)
	want := append([]byte(nil), c.framebuffer[:]...)
	for _, id := range []int{-1, 0, 1} {
		c.PSpr(id, 80, 64, true, true)
		c.PSprSite(0, 0, id, 80, 64, true, true)
		c.PSprSiteFromBase(0, 0, 4, id, 80, 64, true, true)
	}
	for _, call := range animationFlipCallsForTest() {
		call.draw(c, 0, c.InternStatic("empty"), true, true, NilVal())
		call.draw(c, -1, c.InternStatic("missing"), true, true, NilVal())
		call.draw(nil, 0, NilVal(), true, true, NilVal())
	}
	if !bytes.Equal(c.framebuffer[:], want) {
		t.Fatal("invalid or empty flipped draw changed pixels")
	}
}

func TestAutomaticFlipDrawsDoNotAllocate(t *testing.T) {
	c := animationFixture(t)
	name := c.InternStatic("idle")
	if allocs := testing.AllocsPerRun(100, func() {
		c.PSpr(0, 40, 40, true, true)
	}); allocs != 0 {
		t.Fatalf("automatic flip drawing allocated %g times", allocs)
	}
	for _, call := range animationFlipCallsForTest() {
		t.Run(call.name, func(t *testing.T) {
			if allocs := testing.AllocsPerRun(100, func() {
				call.draw(c, 0, name, true, true, NilVal())
			}); allocs != 0 {
				t.Fatalf("animation automatic flip drawing allocated %g times", allocs)
			}
		})
	}
}
