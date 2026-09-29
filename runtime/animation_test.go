//go:build !mtile_mode0

package runtime

import "testing"

func animationFixture(t *testing.T) *Context {
	t.Helper()
	image, memory := poseFixture()
	image.PoseIndex = []PoseRange{{Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}, {Start: 1, Count: 1, FlipOffsetX: 24, FlipOffsetY: 8}, {Count: 2, FlipOffsetX: 16, FlipOffsetY: 8}}
	image.AnimationFrames = []uint16{2, 0, 1}
	image.AnimationClips = []AnimationClip{{Name: "idle", Count: 3, FPS: 12, Loop: true, FlipOffsetX: 16, FlipOffsetY: 8}, {Name: "attack", Count: 3, FPS: 12, FlipOffsetX: 16, FlipOffsetY: 8}, {Name: "empty", FPS: 8, Loop: true}}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("init animation fixture")
	}
	return c
}

func TestAnimationClockUsesFPSAndUpdateRateWithoutFixedPointWrap(t *testing.T) {
	c := animationFixture(t)
	name := c.InternStatic("idle")
	for _, test := range []struct {
		rate int
		tick uint32
		want int
	}{{60, 0, 2}, {60, 4, 2}, {60, 5, 0}, {60, 10, 1}, {60, 15, 2}, {30, 2, 2}, {30, 3, 0}, {30, 5, 1}, {30, 32768, 2}, {60, 60000, 2}} {
		c.SetTickRate(test.rate)
		c.updateTick = test.tick
		if got := c.animationPose(name, NilVal()); got != test.want {
			t.Errorf("rate %d tick %d = %d, want %d", test.rate, test.tick, got, test.want)
		}
	}
}

func TestAnimationExplicitElapsedClampsAndLoops(t *testing.T) {
	c := animationFixture(t)
	for _, test := range []struct {
		name    string
		elapsed Number
		want    int
	}{{"idle", FromInt(-5), 2}, {"idle", 0, 2}, {"idle", 5461, 2}, {"idle", 5462, 0}, {"idle", FromInt(1), 2}, {"idle", FromInt(32767), 2}, {"attack", FromInt(100), 1}, {"attack", FromInt(-1), 2}, {"empty", FromInt(1), -1}, {"missing", 0, -1}} {
		if got := c.animationPose(c.InternStatic(test.name), NumVal(test.elapsed)); got != test.want {
			t.Errorf("%s elapsed %d = %d, want %d", test.name, test.elapsed, got, test.want)
		}
	}
	dynamic, err := c.NewString([]byte("attack"))
	if err != HeapOK {
		t.Fatal(err)
	}
	if got := c.animationPose(dynamic, NumVal(FromInt(100))); got != 1 {
		t.Errorf("heap-string clip = %d", got)
	}
}

func TestAnimationElapsedClockCrossesTheOldTickWrap(t *testing.T) {
	c := animationFixture(t)
	c.SetTickRate(30)
	c.updateTick = 32767
	started := c.Time()
	c.updateTick += 30
	elapsed := (c.Time() - started)
	if elapsed != FromInt(1) {
		t.Fatalf("elapsed across tick32768 = %v, want one second", elapsed)
	}
	if id := c.animationPose(c.InternStatic("attack"), NumVal(elapsed)); id != 1 {
		t.Fatalf("elapsed action restarted at premature clock wrap: pose %d", id)
	}
	// Signed 16.16 seconds wrap remains normal fixed-point arithmetic:
	// subtraction still preserves a short duration across that boundary.
	c.updateTick = 32767 * 30
	started = c.Time()
	c.updateTick += 60
	if elapsed := (c.Time() - started); elapsed != FromInt(2) {
		t.Fatalf("elapsed across seconds wrap = %v", elapsed)
	}
}

func TestAnimationDrawMatchesPosePixelsAndFlipSemantics(t *testing.T) {
	for _, elapsed := range []Number{-1, 0, 5462, 10923, FromInt(2)} {
		for _, flip := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			got, want := animationFixture(t), animationFixture(t)
			got.Cls(4)
			want.Cls(4)
			name := got.InternStatic("idle")
			id := got.animationPose(name, NumVal(elapsed))
			got.AnimV(name, IntVal(40), IntVal(40), BoolVal(flip[0]), BoolVal(flip[1]), NumVal(elapsed))
			x, y := 40, 40
			if flip[0] {
				x += 16
			}
			if flip[1] {
				y += 8
			}
			want.drawPose(id, x, y, flip[0], flip[1])
			assertFramesMatch(t, got, want, "anim uses the clip bounds for the selected pose")
		}
	}
}

func TestAnimationInitRejectsInvalidMetadata(t *testing.T) {
	for _, clip := range []AnimationClip{{Name: "bad", Count: 2, FPS: 8}, {Name: "bad", Count: 1, FPS: 0}, {Name: "bad", Count: 1, FPS: 61}, {Count: 1, FPS: 8}} {
		image, ram := poseFixture()
		image.AnimationClips = []AnimationClip{clip}
		image.AnimationFrames = []uint16{0}
		if new(Context).Init(image, ram) {
			t.Fatalf("accepted invalid clip %+v", clip)
		}
	}
	image, ram := poseFixture()
	image.AnimationFrames = []uint16{99}
	if new(Context).Init(image, ram) {
		t.Fatal("accepted missing animation pose")
	}
}

func TestAnimationIDAndStaticNameMatchGenericSelection(t *testing.T) {
	c := animationFixture(t)
	for id, clip := range c.cart.AnimationClips {
		name := c.InternStatic(clip.Name)
		for _, elapsed := range []Value{NilVal(), NumVal(-1), IntVal(0), NumVal(5462), IntVal(32767), BoolVal(true)} {
			for _, tick := range []uint32{0, 1, 5, 32768, 1 << 31, ^uint32(0)} {
				c.updateTick = tick
				want := c.animationPose(name, elapsed)
				if got := c.animationPoseID(id, elapsed); got != want {
					t.Fatalf("ID %d tick %d: %d != %d", id, tick, got, want)
				}
				if got := c.animationPoseStatic(name, elapsed); got != want {
					t.Fatalf("static %q tick %d: %d != %d", clip.Name, tick, got, want)
				}
			}
		}
	}
	for _, id := range []int{-1, len(c.cart.AnimationClips)} {
		if got := c.animationPoseID(id, NilVal()); got != -1 {
			t.Fatalf("invalid ID %d = %d", id, got)
		}
	}
	if got := c.animationPoseStatic(IntVal(0), NilVal()); got != -1 {
		t.Fatalf("non-string name = %d", got)
	}
}

func TestAnimationIDDrawMatchesGenericPixels(t *testing.T) {
	for _, elapsed := range []Value{NilVal(), NumVal(-1), NumVal(5462), IntVal(1)} {
		for _, flip := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			got, want := animationFixture(t), animationFixture(t)
			got.Cls(4)
			want.Cls(4)
			got.AnimIDV(0, IntVal(40), IntVal(40), BoolVal(flip[0]), BoolVal(flip[1]), elapsed)
			want.AnimV(want.InternStatic("idle"), IntVal(40), IntVal(40), BoolVal(flip[0]), BoolVal(flip[1]), elapsed)
			assertFramesMatch(t, got, want, "direct clip ID preserves pixels")
		}
	}
}
