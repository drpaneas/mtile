package transpiler

import (
	"strings"
	"testing"
)

func TestOBJMemorySnapshotIsOnlyEmittedForDebugCartridges(t *testing.T) {
	for _, heap := range []bool{false, true} {
		for _, debug := range []bool{false, true} {
			opts := animationOptions()
			opts.Debug, opts.DisableHeapElision = debug, heap
			code, _, err := TranspileWithReport("debug-animation.lua", `function _draw() cls() anim("idle",20,30) end`, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(code, "ctx.DebugSnapshot()"); got != debug {
				t.Fatalf("heap=%t debug=%t: debug-only memory scan emitted=%t", heap, debug, got)
			}
			if !debug && strings.Contains(code, "debugMailbox.Begin(") {
				t.Fatal("release cartridge retained a debug mailbox snapshot")
			}
		}
	}
}
