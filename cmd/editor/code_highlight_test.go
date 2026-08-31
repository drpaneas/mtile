package main

import (
	"image/color"
	"strings"
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

func TestLuaHighlightSurvivesHorizontalScrolling(t *testing.T) {
	source := `local value = "wide string" -- retained comment`
	segments := highlightLuaLine(source)
	tests := []struct {
		name  string
		start int
		limit int
		text  string
		color color.Color
	}{
		{name: "inside keyword", start: 2, limit: 3, text: "cal", color: codeKeywordColor},
		{name: "inside string", start: 17, limit: 4, text: "de s", color: codeStringColor},
		{name: "inside comment", start: 33, limit: 8, text: "tained c", color: codeCommentColor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window := clipCodeSegments(segments, test.start, test.limit)
			var visible strings.Builder
			foundColor := false
			for _, segment := range window {
				visible.WriteString(segment.text)
				if strings.Contains(segment.text, test.text) && sameHighlightColor(segment.color, test.color) {
					foundColor = true
				}
			}
			if got := visible.String(); got != test.text {
				t.Fatalf("visible text = %q, want %q", got, test.text)
			}
			if !foundColor {
				t.Fatalf("visible segments lost expected color: %#v", window)
			}
		})
	}
}

func TestCodeSegmentClippingIsRuneSafe(t *testing.T) {
	segments := []codeSegment{{text: "αβγδε", color: codeStringColor}}
	window := clipCodeSegments(segments, 1, 3)
	if len(window) != 1 || window[0].text != "βγδ" || !sameHighlightColor(window[0].color, codeStringColor) {
		t.Fatalf("Unicode window = %#v, want one colored βγδ segment", window)
	}
	if got := clipCodeSegments(segments, 99, 3); len(got) != 0 {
		t.Fatalf("past-end window = %#v, want empty", got)
	}
}
