package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func (g *generator) saveEnabled() bool {
	for _, name := range []string{"save_get", "save_set", "save_read", "save_write"} {
		if chunkCallsBuiltin(g.chunk, name) {
			return true
		}
	}
	return false
}

func saveKey(call *luaparser.CallExpr) (string, error) {
	if len(call.Args) >= 1 {
		if key, ok := unparenExpr(call.Args[0]).(*luaparser.StringExpr); ok && len(key.Value) >= 1 && len(key.Value) <= 32 {
			return key.Value, nil
		}
	}
	return "", fmt.Errorf("transpiler: %s: [SAVE_KEY] save_read/save_write require a literal string key of 1..32 bytes and an optional slot", call.Pos())
}
