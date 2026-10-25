//go:build !mtile_mode0

package runtime

import "testing"

func TestPoseVariantSelectionAndAnimation(t *testing.T) {
	image, memory := poseFixture()
	image.Sheet[9] = 9 // second cell's first row in the row-major sheet
	image.PoseIndex = []PoseRange{{Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}, {Start: 1, Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}}
	image.PoseParts = []PosePart{{Tile: 0}, {Tile: 1}}
	image.PoseVariantBaseCount = 1
	image.PoseVariantNames = []string{"fire"}
	image.AnimationFrames = []uint16{0}
	image.AnimationClips = []AnimationClip{{Name: "idle", Count: 1, FPS: 8, Loop: true, FlipOffsetX: 8, FlipOffsetY: 8}}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("init")
	}
	fire := c.InternStatic("fire")
	dynamic, err := c.NewString([]byte("fire"))
	if err != HeapOK {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		selectVariant func()
		want          byte
	}{
		{"default", func() {}, 7},
		{"id", func() { c.PVariantID(1) }, 9},
		{"persistent", func() {}, 9},
		{"nil reset", func() { c.PVariantV(NilVal()) }, 7},
		{"static", func() { c.PVariantStaticV(fire) }, 9},
		{"unknown", func() { c.PVariantV(c.InternStatic("missing")) }, 7},
		{"dynamic", func() { c.PVariantV(dynamic) }, 9},
		{"out of range", func() { c.PVariantID(2) }, 7},
		{"number", func() { c.PVariantV(NumVal(FromInt(1))) }, 9},
		{"zero", func() { c.PVariantStaticV(NumVal(0)) }, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.selectVariant()
			for _, animation := range []bool{false, true} {
				c.Cls(0)
				if animation {
					c.AnimV(c.InternStatic("idle"), NumVal(FromInt(10)), NumVal(FromInt(20)), BoolVal(false), BoolVal(false), NilVal())
				} else {
					c.PSpr(0, 10, 20, false, false)
				}
				if got := c.fbForTest()[20*ScreenWidth+11]; got != tc.want {
					t.Fatalf("animation=%v pixel=%d want %d", animation, got, tc.want)
				}
			}
		})
	}
	c.Cls(0)
	c.PSpr(1, 10, 20, false, false)
	if got := c.fbForTest()[20*ScreenWidth+11]; got != 0 {
		t.Fatalf("internal pose ID publicly accessible: %d", got)
	}
}
