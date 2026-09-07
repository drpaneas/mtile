package transpiler

import (
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// The reservation proof supplies a fresh, unaliased list and a loop whose
// entire body is one non-nil append. Count attempts independently of the Lua
// induction variable, including descending and non-unit steps. A failed
// append leaves a hole at that attempt's position; the runtime then retains
// ordinary dense-prefix behavior through its guarded fallback.
func (g *generator) beginKnownAppendLoop(b *sourceBuilder, loop *luaparser.NumericForStmt, indent int) (string, func()) {
	call := g.reservedAppendLoops[loop]
	if g.opts.DisableKnownTableAppends || call == nil {
		return "", func() {}
	}
	if g.knownAppendPositions == nil {
		g.knownAppendPositions = make(map[*luaparser.CallExpr]string)
	}
	at := g.next("appendAt")
	g.knownAppendPositions[call] = at
	b.line(strings.Repeat("\t", indent) + at + " := 1")
	return at, func() { delete(g.knownAppendPositions, call) }
}

func (g *generator) emitKnownTableAppend(call *luaparser.CallExpr, at string) string {
	// A globally registered list is already a heap root. Putting a copy on the
	// local root stack during the RHS constructor only repeats PushLocal work.
	// Locals and any other destination still use the ordinary sequencer so a
	// later allocating operand cannot collect the destination list.
	if variable, ok := unparenExpr(call.Args[0]).(*luaparser.VarExpr); ok {
		if _, local := g.lookupLocal(variable.Name); !local {
			list := g.emitExpr(call.Args[0])
			value := g.emitExpr(call.Args[1])
			// The destination is a registered global. Root only the new element
			// so a collection during the append cannot drop it; the list itself
			// is already in the global root set.
			return "_ = gluTableAppendKnownHeld(c," + list + "," + value + "," + at + g.rootArgsFor(5, call.Args[0].Pos()) + ")"
		}
	}
	args := g.sequenceAdjustedExprs(call.Args)
	return "_ = gluTableAppendKnown(c," + adjustedLuaValue(args[0]) + "," + adjustedLuaValue(args[1]) + "," + at + g.rootArgsFor(5, call.Args[0].Pos()) + ")"
}
