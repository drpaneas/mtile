package main

import (
	"image/color"
	"testing"
)

func sameHighlightColor(left, right color.Color) bool {
	lr, lg, lb, la := left.RGBA()
	rr, rg, rb, ra := right.RGBA()
	return lr == rr && lg == rg && lb == rb && la == ra
}

func TestGeneratedGoHighlighterRecognizesGoInsteadOfLua(t *testing.T) {
	segments := highlightGoLine(`func render(value int) string { return "ok" } // generated`)
	want := map[string]color.Color{
		"func": codeKeywordColor, "int": codeBuiltinColor, "string": codeBuiltinColor,
		"return": codeKeywordColor, `"ok"`: codeStringColor, "// generated": codeCommentColor,
	}
	for text, expected := range want {
		found := false
		for _, segment := range segments {
			if segment.text == text && sameHighlightColor(segment.color, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Go token %q was not highlighted with its expected role: %#v", text, segments)
		}
	}
}
