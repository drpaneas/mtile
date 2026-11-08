package main

import (
	"strings"
	"testing"
)

func TestDevelopAssetCompletionAndOpenAsset(t *testing.T) {
	g := newTestGameWithPoses(t)
	t.Cleanup(g.closeNativePerformance)
	g.mode = modeCode
	g.model.Poses.Poses[0].Name = "ήρεμο'1"
	source := "function _draw()\n  pspr('ήold', 30, 40)\nend\n"
	g.code.SetText(source)
	g.code.SetCursor(len([]rune(source[:strings.Index(source, "old")])), false)
	g.openCodeCompletions()
	if len(g.codeCompletions) != 1 || g.codeCompletions[0].name != "ήρεμο'1" {
		t.Fatalf("pose suggestions = %+v", g.codeCompletions)
	}
	layout := newDevelopLayout().Code
	box, _, _ := g.codeCompletionPopup(layout)
	if !g.clickCodeCompletion(layout, box.X+12, box.Y+12) {
		t.Fatal("click did not select the named pose")
	}
	want := strings.Replace(source, "ήold", `ήρεμο\'1`, 1)
	if g.code.Text() != want {
		t.Fatalf("completion = %q, want %q", g.code.Text(), want)
	}
	g.code.SetCursor(len([]rune(want[:strings.Index(want, `\'`)])), false)
	g.openDevelopAssetAtCaret()
	if g.mode != modeSprites || g.selectedPose != 0 {
		t.Fatalf("Open Asset did not navigate to pose: mode %v, pose %d, %s", g.mode, g.selectedPose, g.statusMsg)
	}
}
