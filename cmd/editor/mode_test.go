package main

import "testing"

func TestEditorModeCycleHasFourPrimaryWorkspaces(t *testing.T) {
	cycle := []editorMode{modeSprites, modeMap, modeMusic, modeCode, modeSprites}
	for index := 0; index < len(cycle)-1; index++ {
		if got := nextEditorMode(cycle[index]); got != cycle[index+1] {
			t.Fatalf("next mode after %v = %v, want %v", cycle[index], got, cycle[index+1])
		}
	}
	if len(primaryEditorModes) != 4 {
		t.Fatalf("primary editor tabs = %d, want 4", len(primaryEditorModes))
	}
}

func TestEditorTabsDoNotOverlap(t *testing.T) {
	for leftIndex, leftMode := range primaryEditorModes {
		left := editorTabRect(leftMode)
		for _, rightMode := range primaryEditorModes[leftIndex+1:] {
			right := editorTabRect(rightMode)
			if left.X < right.X+right.W && right.X < left.X+left.W {
				t.Fatalf("tabs %v and %v overlap: %+v %+v", leftMode, rightMode, left, right)
			}
		}
	}
}
