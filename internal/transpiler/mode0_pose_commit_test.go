package transpiler

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
)

func TestMode0PoseCommitTransferBoundary(t *testing.T) {
	const paletteAndMutations = `function _update()
 pal(1,2)
 sset(0,0,1)
 sset(8,0,1)
 sset(16,0,1)
 sset(24,0,1)
end
`
	cases := []struct {
		name                 string
		head                 string
		tiles, layers, edits int
		audio                bool
		wantError            string
	}{
		{name: "full-window", tiles: 512},
		{name: "full-window-four-backgrounds", tiles: 512, layers: 4, wantError: "117536-cycle commit"},
		// 323 tiles plus one BG, four direct mutations and a palette commit
		// need 82,536 cycles: eight cycles below the complete VBlank window.
		{name: "combined-last-fitting-tile", head: paletteAndMutations, tiles: 323, layers: 1, edits: 4},
		{name: "combined-one-more-tile", head: paletteAndMutations, tiles: 324, layers: 1, edits: 4, wantError: "82664-cycle commit"},
		{name: "initial-palette-is-already-flushed", head: "function _init() pal(1,2) end\n", tiles: 512},
		{name: "unused-audio-assets", tiles: 512, audio: true},
		{name: "full-window-with-audio", head: "function _init() music(0) end\n", tiles: 512, audio: true, wantError: "114304-cycle commit"},
		// Audio reserves a second service interrupt above 49,152 warm cycles.
		{name: "audio-one-interrupt-boundary", head: "function _init() music(0) end\n", tiles: 259, audio: true},
		{name: "audio-last-fitting-tile", head: "function _init() music(0) end\n", tiles: 263, audio: true},
		{name: "audio-one-more-tile", head: "function _init() music(0) end\n", tiles: 264, audio: true, wantError: "82560-cycle commit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := luaparser.ParseString("pose-commit.lua", tc.head+"function _draw()\n pspr(0,0,0)\nend\n")
			if err != nil {
				t.Fatal(err)
			}
			call := newMode0CallGraph(chunk).firstBuiltinFromFunction("_draw", "pspr")
			if call == nil {
				t.Fatal("fixture has no pose draw")
			}
			opts := Options{SheetW: 32, SheetH: 8}
			if tc.audio {
				opts.NativeAudio = &assetsync.NativeAudioAssets{}
			}
			ops := []mode0DrawOp{{name: "pspr", call: call, copies: 1, psprUnique: tc.tiles}}
			err = validateMode0PoseCommit(chunk, ops, opts, tc.layers, tc.edits)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("fitting commit rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "[M0_RESOURCE_LIMIT]") ||
				!strings.Contains(err.Error(), "pose-commit.lua:") || !strings.Contains(err.Error(), tc.wantError) ||
				!strings.Contains(err.Error(), "82544-cycle VBlank window") {
				t.Fatalf("error = %v; want source-positioned resource diagnostic containing %q", err, tc.wantError)
			}
		})
	}
}
