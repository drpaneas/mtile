package transpiler

import (
	"fmt"
	"strconv"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
)

// lowerPoseNames gives every analysis and emitter the exact same numeric
// operand. Restore the caller's AST afterwards: Generate and SelectRenderer
// may be called again with a reordered asset library.
func lowerPoseNames(chunk *luaparser.Chunk, opts Options) (warnings []string, restore func(), err error) {
	type replacement struct {
		call *luaparser.CallExpr
		args []luaparser.Expr
	}
	var replaced []replacement
	restore = func() {
		for _, entry := range replaced {
			entry.call.Args = entry.args
		}
	}
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if err != nil {
			return
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, body := range fn.Body.Body {
				visit(body)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if err != nil {
				return
			}
			call, ok := expr.(*luaparser.CallExpr)
			if !ok || len(call.Args) == 0 {
				return
			}
			if name, _ := callKey(call); name != "pspr" {
				return
			}
			operand := unparenExpr(call.Args[0])
			if name, literal := operand.(*luaparser.StringExpr); literal {
				id := -1
				for index, candidate := range opts.PoseNames {
					if candidate == "" || candidate != name.Value {
						continue
					}
					if id != -1 {
						err = fmt.Errorf("transpiler: %s: [PSPR_AMBIGUOUS_POSE] pose name %q is not unique in assets/poses.json", name.Pos(), name.Value)
						return
					}
					id = index
				}
				if id < 0 {
					suggestion := ""
					if closest := closestPoseName(name.Value, opts.PoseNames); closest != "" {
						suggestion = fmt.Sprintf("; did you mean %q?", closest)
					}
					err = fmt.Errorf("transpiler: %s: [PSPR_UNKNOWN_POSE] no pose %q in assets/poses.json%s", name.Pos(), name.Value, suggestion)
					return
				}
				if id >= canonicalPoseCount(opts) || id >= assetsync.MaxPoseCount {
					err = fmt.Errorf("transpiler: %s: [PSPR_INVALID_POSE] pose %q has no representable canonical index", name.Pos(), name.Value)
					return
				}
				replaced = append(replaced, replacement{call, call.Args})
				call.Args = append([]luaparser.Expr(nil), call.Args...)
				call.Args[0] = &luaparser.NumberExpr{NodeBase: name.NodeBase, Raw: strconv.Itoa(id), Value: float64(id), Integer: true}
				return
			}
			// Use the actual signed 16:16 conversion, including fractional
			// quantization and wrapping. Dynamic IDs keep their existing no-op
			// semantics, and a warning never changes numeric code generation.
			if id, literal := mode0StaticRuntimeInt(operand); literal && (id < 0 || id >= canonicalPoseCount(opts)) {
				warnings = append(warnings, fmt.Sprintf("%s: [PSPR_ID_OUT_OF_RANGE] pspr ID %d has no pose (%d registered); this call draws nothing", operand.Pos(), id, canonicalPoseCount(opts)))
			}
		})
	}
	for _, stmt := range chunk.Body {
		visit(stmt)
	}
	return warnings, restore, err
}

func closestPoseName(query string, names []string) string {
	a := []rune(query)
	limit := min(3, max(1, len(a)/3))
	best, distance := "", limit+1
	for _, name := range names {
		if name == "" {
			continue
		}
		b := []rune(name)
		if len(a)-len(b) > limit || len(b)-len(a) > limit {
			continue
		}
		row := make([]int, len(b)+1)
		for j := range row {
			row[j] = j
		}
		for i, ar := range a {
			previous := row[0]
			row[0] = i + 1
			for j, br := range b {
				cost := 0
				if ar != br {
					cost = 1
				}
				next := min(row[j+1]+1, row[j]+1, previous+cost)
				previous, row[j+1] = row[j+1], next
			}
		}
		d := row[len(b)]
		if d <= limit && (d < distance || d == distance && name < best) {
			best, distance = name, d
		}
	}
	return best
}
