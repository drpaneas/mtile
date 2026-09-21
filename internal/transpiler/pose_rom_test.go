package transpiler

import (
	"strings"
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
)

// TestTranspileEmitsPoseROMAndCartWiring locks the ROM-embedding contract for
// pspr's pose tables: non-empty Options.PoseIndex/PoseParts must surface as
// literal .rodata arrays and wire straight into both cartImage and cartData,
// with no EWRAM copy (poses are read-only cart metadata).
func TestTranspileEmitsPoseROMAndCartWiring(t *testing.T) {
	got, err := Transpile("pose.lua", `
function _draw()
end
`, Options{
		PoseIndex: []rt.PoseRange{{Start: 0, Count: 1}},
		PoseParts: []rt.PosePart{{Tile: 1, X: 2, Y: 3, Flags: 1}},
	})
	if err != nil {
		t.Fatalf("transpile: %v", err)
	}
	if !strings.Contains(got, "PosePart{{Tile: 1, X: 2, Y: 3, Flags: 1}}") {
		t.Fatalf("generated source missing pose part ROM literal:\n%s", got)
	}
	if !strings.Contains(got, "PoseRange{{Start: 0, Count: 1}}") {
		t.Fatalf("generated source missing pose index ROM literal:\n%s", got)
	}
	if !strings.Contains(got, "cartData.PoseIndex") {
		t.Fatalf("generated source missing cartData.PoseIndex wiring:\n%s", got)
	}
	if !strings.Contains(got, "PoseIndex: poseIndexROM[:], PoseParts: posePartROM[:],") {
		t.Fatalf("generated source missing cartImage pose ROM wiring:\n%s", got)
	}
}

// TestTranspileEmptyPosesEmitZeroLengthROMArrays guards new/empty projects:
// no poses.json must still emit legal [0]rt.PoseRange{}/[0]rt.PosePart{}
// arrays so poseIndexROM[:] / posePartROM[:] stay valid slice expressions.
func TestTranspileEmptyPosesEmitZeroLengthROMArrays(t *testing.T) {
	got, err := Transpile("pose_empty.lua", `
function _draw()
end
`, Options{})
	if err != nil {
		t.Fatalf("transpile: %v", err)
	}
	if !strings.Contains(got, "var poseIndexROM = [ 0 ]rt.PoseRange{}") {
		t.Fatalf("generated source missing empty pose index ROM array:\n%s", got)
	}
	if !strings.Contains(got, "var posePartROM = [ 0 ]rt.PosePart{}") {
		t.Fatalf("generated source missing empty pose part ROM array:\n%s", got)
	}
}
