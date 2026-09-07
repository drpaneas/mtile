package transpiler

import (
	"strconv"
	"strings"
)

// typedDrawIntegers consumes already sequenced operands. Evaluation and Lua's
// trailing multiple-result rules therefore remain identical to the Value ABI.
// Only proven numeric operands bypass Int/IntOr's tag checks; an explicit nil,
// string, or other unknown value retains the complete existing wrapper path.
func (g *generator) typedDrawIntegers(values []adjustedValue, defaults []int) ([]string, bool) {
	if g.opts.DisableTypedDrawCalls {
		return nil, false
	}
	args := make([]string, len(defaults))
	for i, fallback := range defaults {
		if i >= len(values) {
			args[i] = strconv.Itoa(fallback)
			continue
		}
		if values[i].kind != monoNumber {
			return nil, false
		}
		args[i] = "(" + values[i].code + ").Int()"
	}
	return args, true
}

func (g *generator) typedDrawCall(name string, values []adjustedValue) (string, bool) {
	if name == "camera" {
		args, ok := g.typedDrawIntegers(values, []int{0, 0})
		return "c.Camera(" + strings.Join(args, ",") + ")", ok
	}
	args, ok := g.typedDrawIntegers(values, []int{0, 0, 0, 1, 1})
	if !ok {
		return "", false
	}
	for i := 5; i < 7; i++ {
		if i >= len(values) {
			args = append(args, "false")
		} else {
			// Lua considers 0, strings and tables truthy too. The local Value
			// conversion optimizes away for numeric/boolean expressions without
			// changing this rule or discarding an argument's side effects.
			args = append(args, "("+adjustedLuaValue(values[i])+").IsTrue()")
		}
	}
	return "c.Spr(" + strings.Join(args, ",") + ")", true
}
