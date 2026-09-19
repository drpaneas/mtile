package transpiler

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func mode0AudioCommitOptions(t *testing.T) Options {
	t.Helper()
	opts := nativeTranspilerOptions(t)
	opts.SheetW, opts.SheetH = 16, 8
	opts.MapW, opts.MapH, opts.MapLayers = 1, 1, 4
	opts.Map = []uint16{1, 1, 1, 1}
	return opts
}

func TestMode0AudioCommitRejectsPerFrameWorkWithFourBGs(t *testing.T) {
	cases := []struct{ name, head, mutation, position string }{
		{"sheet", "", "function _update()\n sset(8,0,2)\nend\n", "audio-budget.lua:4:2"},
		{"palette", "", "function _update()\n pal(1,2)\nend\n", "audio-budget.lua:4:2"},
		{"palette-reset", "", "function _update()\n pal()\nend\n", "audio-budget.lua:4:2"},
		{"helper-palette", "function change_palette()\n pal(1,2)\nend\n", "function _update() change_palette() end\n", "audio-budget.lua:2:2"},
		{"helper-sheet", "function change_sheet()\n sset(8,0,2)\nend\n", "function _update() change_sheet() end\n", "audio-budget.lua:2:2"},
		{"conditional", "", "function _update()\n if btn(0) then pal(1,2) end\nend\n", "audio-budget.lua:4:17"},
		{"draw-helper", "function change_palette()\n pal(1,2)\nend\n", "function _draw() cls() map() change_palette() end\n", "audio-budget.lua:2:2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.head + "function _init() music(0) end\n\n" + tc.mutation
			if tc.name != "draw-helper" {
				source += "function _draw()\n cls()\n map()\nend\n"
			}
			chunk, err := luaparser.ParseString("audio-budget.lua", source)
			if err != nil {
				t.Fatal(err)
			}
			var diagnostic string
			for _, renderer := range []Renderer{RendererMode0, RendererAuto, RendererMode4} {
				opts := mode0AudioCommitOptions(t)
				opts.Renderer = renderer
				report, err := SelectRenderer(chunk, opts)
				if renderer == RendererMode4 {
					if err != nil || report.Selected != RendererMode4 || report.Diagnostic != "" {
						t.Fatalf("explicit Mode4: report=%+v err=%v", report, err)
					}
					continue
				}
				if renderer == RendererMode0 {
					if err == nil {
						t.Fatal("forced Mode0 accepted unproven commit")
					}
					diagnostic = err.Error()
				} else {
					if err != nil || report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible {
						t.Fatalf("AUTO: report=%+v err=%v", report, err)
					}
					if report.Diagnostic != diagnostic {
						t.Fatalf("forced/AUTO diagnostics differ: %q / %q", diagnostic, report.Diagnostic)
					}
				}
				for _, want := range []string{tc.position, "[M0_RESOURCE_LIMIT]", "four materialized BGs", "sset()/pal()", "VBlank"} {
					if !strings.Contains(diagnostic, want) {
						t.Fatalf("diagnostic=%q missing %q", diagnostic, want)
					}
				}
			}
		})
	}
}

func TestMode0AudioCommitKeepsSupportedControls(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		native       bool
		layers       int
	}{
		{"four-dirty-maps-audio", "function _init() music(0) end\nfunction _update() mset(0,0,0,1) mset(1,0,0,1) mset(2,0,0,1) mset(3,0,0,1) end\nfunction _draw() cls() map() end", true, 4},
		{"init-mutations", "function _init() music(0) sset(8,0,2) pal(1,2) end\nfunction _draw() cls() map() end", true, 4},
		{"init-helper-palette", "function palette_init() pal(1,2) end\nfunction _init() music(0) palette_init() end\nfunction _draw() cls() map() end", true, 4},
		{"one-materialized-of-four", "function _init() music(0) end\nfunction _update() pal(1,2) sset(8,0,2) end\nfunction _draw() cls() map(0) end", true, 1},
		{"three-materialized-of-four", "function _init() music(0) end\nfunction _update() pal(1,2) end\nfunction _draw() cls() map(0) map(1) map(2) end", true, 3},
		{"unused-authored-audio", "function _update() pal(1,2) sset(8,0,2) end\nfunction _draw() cls() map() end", true, 4},
		{"no-native-assets", "function _init() music(0) end\nfunction _update() pal(1,2) sset(8,0,2) end\nfunction _draw() cls() map() end", false, 4},
		{"unreachable-palette-helper", "function unused_palette() pal(1,2) end\nfunction _init() music(0) end\nfunction _draw() cls() map() end", true, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := luaparser.ParseString("audio-control.lua", tc.source)
			if err != nil {
				t.Fatal(err)
			}
			for _, renderer := range []Renderer{RendererMode0, RendererAuto} {
				opts := mode0AudioCommitOptions(t)
				opts.Renderer = renderer
				if !tc.native {
					opts.NativeAudio = nil
				}
				report, err := SelectRenderer(chunk, opts)
				if err != nil || report.Selected != RendererMode0 || report.Mode0Layers != tc.layers || report.Diagnostic != "" {
					t.Fatalf("%s report=%+v err=%v", renderer, report, err)
				}
			}
		})
	}
}
