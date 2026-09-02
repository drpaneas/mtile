package transpiler

import (
	"math"
	"sort"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

type mode0TextAtlas struct {
	keys  []uint16
	tiles []uint32
}

// mode0TextAtlasKeysForDraw returns the complete immutable tile identity set
// admitted by Mode 0. Renderer selection has already required every print to
// be a direct statement with a literal string and literal color; keeping this
// collector defensive also makes direct unit use deterministic.
func mode0TextAtlasKeysForDraw(draw *luaparser.FunctionDeclStmt) []uint16 {
	if draw == nil {
		return nil
	}
	seen := make(map[uint16]bool)
	for _, stmt := range draw.Body.Body {
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		name, ok := callKey(callStmt.Call)
		if !ok || name != "print" || len(callStmt.Call.Args) != 4 {
			continue
		}
		text, ok := unparenExpr(callStmt.Call.Args[0]).(*luaparser.StringExpr)
		if !ok {
			continue
		}
		colorValue, literal := staticNumberLiteral(callStmt.Call.Args[3])
		if !literal || colorValue != math.Trunc(colorValue) || colorValue < 1 || colorValue > 255 {
			continue
		}
		color := byte(colorValue)
		for index := 0; index < len(text.Value); index++ {
			if text.Value[index] < 32 || text.Value[index] > 126 {
				continue
			}
			key := runtimeapi.Mode0TextGlyphKey(text.Value[index], color)
			if !seen[key] {
				seen[key] = true
			}
		}
	}
	keys := make([]uint16, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func buildMode0TextAtlas(draw *luaparser.FunctionDeclStmt) mode0TextAtlas {
	keys := mode0TextAtlasKeysForDraw(draw)
	atlas := mode0TextAtlas{keys: keys, tiles: make([]uint32, 0, len(keys)*16)}
	for _, key := range keys {
		tile := runtimeapi.BuildMode0GlyphTile(byte(key>>8), byte(key))
		for offset := 0; offset < len(tile); offset += 4 {
			atlas.tiles = append(atlas.tiles,
				uint32(tile[offset])|uint32(tile[offset+1])<<8|uint32(tile[offset+2])<<16|uint32(tile[offset+3])<<24)
		}
	}
	return atlas
}
