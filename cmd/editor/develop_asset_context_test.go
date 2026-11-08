package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDevelopAssetContextFindsDirectLiteralAndReplacementBounds(t *testing.T) {
	cases := []struct {
		marked, kind, raw, prefix, name string
		quote                           rune
		closed                          bool
	}{
		{`anim("ru|n", x, y)`, "anim", "run", "ru", "run", '"', true},
		{"-- ελληνικά\npspr(-- first argument\n'άλ|μα', x, y)", "pspr", "άλμα", "άλ", "άλμα", '\'', true},
		{`anim(("|walk"), x, y)`, "anim", "walk", "", "walk", '"', true},
		{`anim("|", x, y)`, "anim", "", "", "", '"', true},
		{`anim('go\'|left\\fast', 0, 0)`, "anim", `go\'left\\fast`, "go'", `go'left\fast`, '\'', true},
		{`anim("\114u|n", x, y)`, "anim", `\114un`, "ru", "run", '"', true},
		{`pspr('run|`, "pspr", "run", "run", "run", '\'', false},
		{"anim(\"mi|ssing\nprint('next')", "anim", "missing", "mi", "missing", '"', false},
		{`anim("r\|`, "anim", `r\`, "r", "r", '"', false},
		{`pspr("ru|n", `, "pspr", "run", "ru", "run", '"', true},
		{"anim(\"ru|n\", x, y)\nlocal other = \"unfinished", "anim", "run", "ru", "run", '"', true},
	}
	for _, test := range cases {
		t.Run(test.marked, func(t *testing.T) {
			before, after, _ := strings.Cut(test.marked, "|")
			source := before + after
			got, ok := developAssetContextAt(source, utf8.RuneCountInString(before))
			if !ok || got.Kind != test.kind || got.Prefix != test.prefix || got.Name != test.name || got.Quote != test.quote || got.Closed != test.closed {
				t.Fatalf("wrong asset context: %+v, found %t", got, ok)
			}
			runes := []rune(source)
			if got.Start <= 0 || got.End < got.Start || got.End > len(runes) || runes[got.Start-1] != got.Quote || string(runes[got.Start:got.End]) != test.raw {
				t.Fatalf("replacement [%d:%d] does not cover only %q in %q", got.Start, got.End, test.raw, source)
			}
		})
	}
}

func TestDevelopAssetContextIgnoresOtherLuaStrings(t *testing.T) {
	for _, marked := range []string{
		`-- anim("ru|n", x, y)`,
		`--[=[ pspr("ru|n", x, y) ]=]`,
		`print([=[ anim("ru|n", x, y) ]=])`,
		`print('anim("ru|n", x, y)')`,
		`obj.anim("ru|n", x, y)`,
		`obj:pspr("ru|n", x, y)`,
		"obj. -- member\nanim(\"ru|n\", x, y)",
		`obj["anim"]("ru|n", x, y)`,
		`function anim("ru|n", x, y)`,
		`other_anim("ru|n", x, y)`,
		`anim(1, "ru|n", y)`,
		`anim("ru|n" .. suffix, x, y)`,
		`anim(("ru|n" .. suffix), x, y)`,
		`anim(choose("ru|n"), x, y)`,
		`anim("run"|, x, y)`,
	} {
		t.Run(marked, func(t *testing.T) {
			before, after, _ := strings.Cut(marked, "|")
			if got, ok := developAssetContextAt(before+after, utf8.RuneCountInString(before)); ok {
				t.Fatalf("unexpected asset context %+v", got)
			}
		})
	}
}
