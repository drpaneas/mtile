package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

// Simulates mapdemo's HUD each draw: stat(2), flr, two Concats, print.
func simulateMapdemoHUDPrint(c *Context) {
	stat := c.Stat(2)
	pct := NumVal(stat.Mul(FromInt(100)).Flr())
	s1, err := c.Concat(c.InternStatic("cpu "), pct)
	if err != HeapOK {
		return
	}
	s2, err := c.Concat(s1, c.InternStatic("%"))
	if err != HeapOK {
		return
	}
	c.PrintV(s2, IntVal(4), IntVal(4), IntVal(15))
}

func simulateMapdemoHUDPrintStatic(c *Context) {
	c.PrintV(c.InternStatic("cpu 62%"), IntVal(4), IntVal(4), IntVal(15))
}

func countAllocatedHandles(c *Context) int {
	n := 0
	for i := 0; i < maxHandles; i++ {
		if c.heap.handles[i].State() == HandleAllocated {
			n++
		}
	}
	return n
}

func simulateMapdemoTick(c *Context, hud func(*Context)) {
	hw.SetFrameCycleCounterForTest(0)
	c.BeginFramePace()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 30, 20, 0)
	c.Spr(8, 60, 136, 1, 1, false, false)
	hud(c)
	c.PrepareHiddenPage()
	c.CollectIfNeeded()
	c.PaceFrame30()
}

// TestMapdemoHUDConcatTriggersGCSpike verifies concat in the HUD eventually
// forces the Lua collector (~900 frames on a 56 KiB heap).
func TestMapdemoHUDConcatTriggersGCSpike(t *testing.T) {
	hw.ResetForTest()
	c := newBenchSparseMapContext(t)
	c.SetProfiling(true)

	var gcFrame int
	var handlesAtGC int
	for frame := 1; frame <= 950; frame++ {
		before := c.heap.collectCount
		simulateMapdemoTick(c, simulateMapdemoHUDPrint)
		if c.heap.collectCount > before && gcFrame == 0 {
			gcFrame = frame
			handlesAtGC = countAllocatedHandles(c)
			t.Logf("GC at frame %d: collectCycles=%d handlesLive=%d cursor=%d",
				frame, c.heap.lastCollectCycles, handlesAtGC, c.heap.cursor)
		}
	}

	if gcFrame == 0 {
		t.Fatalf("expected GC within 950 frames, handles=%d cursor=%d",
			countAllocatedHandles(c), c.heap.cursor)
	}
	if gcFrame < 650 || gcFrame > 950 {
		t.Fatalf("first GC at frame %d, want ~683 (handle table exhaustion)", gcFrame)
	}
}

// TestMapdemoHUDStaticPrintNoGCSpike is the control: static print never
// allocates, so the collector never runs across 950 ticks.
func TestMapdemoHUDStaticPrintNoGCSpike(t *testing.T) {
	hw.ResetForTest()
	c := newBenchSparseMapContext(t)
	c.SetProfiling(true)
	const tickBudget = 561_792

	for frame := 1; frame <= 950; frame++ {
		simulateMapdemoTick(c, simulateMapdemoHUDPrintStatic)
	}

	if c.heap.collectCount != 0 {
		t.Fatalf("collectCount = %d, want 0 with static HUD", c.heap.collectCount)
	}
	if c.maxFrameCycles > tickBudget {
		t.Fatalf("maxFrameCycles = %d exceeded budget without GC", c.maxFrameCycles)
	}
}

// GBA hardware (mGBA + GDB, Aug 2025) confirms the concat HUD spikes on
// frame 683: maxFrameCycles=600093 (106.8% of the 561792 tick budget).
// A static-string HUD stays at ~61% through frame 900.
