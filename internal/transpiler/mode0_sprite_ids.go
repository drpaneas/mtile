package transpiler

import (
	"fmt"
	"math"
	"sort"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// mode0SpriteIDAnalysis proves a scalar sprite id visible from _draw. The
// enumerated path accepts unconditional top-level or _init initialization plus
// assignments that are integer literals or immutable integer constants.
// Arithmetic writes are admitted only when bounded-counter analysis can close
// the values observed between complete _init/_update/_draw callbacks. An
// unguarded `frame = frame + 1` remains outside the proof.
type mode0SpriteIDAnalysis struct {
	chunk              *luaparser.Chunk
	writes             map[string][]mode0SpriteIDWrite
	topLocals          map[string]int
	shadowed           map[string]bool
	functionNames      map[string]bool
	constants          map[string]int
	constantWrite      map[string]mode0SpriteIDWrite
	firstFunctionOrder int
	bounded            map[string]boundedCounterResult
	boundedBusy        map[string]bool
}

type mode0SpriteIDWrite struct {
	value      luaparser.Expr
	position   luaparser.Position
	order      int
	topLevel   bool
	directInit bool
	valid      bool
}

func newMode0SpriteIDAnalysis(chunk *luaparser.Chunk) *mode0SpriteIDAnalysis {
	a := &mode0SpriteIDAnalysis{
		chunk:              chunk,
		writes:             map[string][]mode0SpriteIDWrite{},
		topLocals:          map[string]int{},
		shadowed:           map[string]bool{},
		functionNames:      map[string]bool{},
		constants:          map[string]int{},
		constantWrite:      map[string]mode0SpriteIDWrite{},
		firstFunctionOrder: -1,
		bounded:            map[string]boundedCounterResult{},
		boundedBusy:        map[string]bool{},
	}
	if chunk == nil {
		return a
	}
	for order, stmt := range chunk.Body {
		a.collectStmt(stmt, mode0SpriteIDContext{order: order, topLevel: true, direct: true})
	}
	a.discoverConstants()
	return a
}

type mode0SpriteIDContext struct {
	order    int
	topLevel bool
	direct   bool
	function string
}

func (a *mode0SpriteIDAnalysis) collectStmt(stmt luaparser.Stmt, context mode0SpriteIDContext) {
	if stmt == nil {
		return
	}
	collectBlock := func(block *luaparser.Block, child mode0SpriteIDContext) {
		if block == nil {
			return
		}
		for _, nested := range block.Body {
			a.collectStmt(nested, child)
		}
	}
	invalidWrite := func(name string, position luaparser.Position) {
		a.writes[name] = append(a.writes[name], mode0SpriteIDWrite{
			position: position,
			order:    context.order,
			topLevel: context.topLevel && context.direct,
			valid:    false,
		})
	}

	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for index, target := range s.Targets {
			variable, ok := target.(*luaparser.VarExpr)
			if !ok {
				continue
			}
			if len(s.Targets) != 1 || len(s.Values) != 1 || index >= len(s.Values) {
				invalidWrite(variable.Name, s.Pos())
				continue
			}
			a.writes[variable.Name] = append(a.writes[variable.Name], mode0SpriteIDWrite{
				value:      s.Values[index],
				position:   s.Pos(),
				order:      context.order,
				topLevel:   context.topLevel && context.direct,
				directInit: context.function == "_init" && context.direct,
				valid:      true,
			})
		}

	case *luaparser.LocalAssignStmt:
		if context.topLevel && context.direct {
			for index, name := range s.Names {
				if _, exists := a.topLocals[name]; exists {
					a.shadowed[name] = true
				} else {
					a.topLocals[name] = context.order
				}
				if len(s.Names) != 1 || len(s.Values) != 1 || index >= len(s.Values) {
					invalidWrite(name, s.Pos())
					continue
				}
				a.writes[name] = append(a.writes[name], mode0SpriteIDWrite{
					value:    s.Values[index],
					position: s.Pos(),
					order:    context.order,
					topLevel: true,
					valid:    true,
				})
			}
		} else {
			for _, name := range s.Names {
				a.shadowed[name] = true
			}
		}

	case *luaparser.FunctionDeclStmt:
		if context.topLevel && a.firstFunctionOrder < 0 {
			a.firstFunctionOrder = context.order
		}
		if len(s.Name) == 1 && s.Method == "" {
			if s.Local {
				a.shadowed[s.Name[0]] = true
			} else {
				a.functionNames[s.Name[0]] = true
			}
		}
		for _, parameter := range s.Params {
			a.shadowed[parameter] = true
		}
		collectBlock(s.Body, mode0SpriteIDContext{
			order: context.order, direct: true, function: functionKey(s),
		})

	case *luaparser.IfStmt:
		child := context
		child.topLevel = false
		child.direct = false
		collectBlock(s.Then, child)
		for _, branch := range s.ElseIf {
			collectBlock(branch.Body, child)
		}
		collectBlock(s.Else, child)

	case *luaparser.WhileStmt:
		child := context
		child.topLevel = false
		child.direct = false
		collectBlock(s.Body, child)

	case *luaparser.RepeatStmt:
		child := context
		child.topLevel = false
		child.direct = false
		collectBlock(s.Body, child)

	case *luaparser.DoStmt:
		child := context
		child.topLevel = false
		child.direct = false
		collectBlock(s.Body, child)

	case *luaparser.NumericForStmt:
		a.shadowed[s.Name] = true
		child := context
		child.topLevel = false
		child.direct = false
		collectBlock(s.Body, child)

	case *luaparser.GenericForStmt:
		for _, name := range s.Names {
			a.shadowed[name] = true
		}
		child := context
		child.topLevel = false
		child.direct = false
		collectBlock(s.Body, child)
	}
}

func (a *mode0SpriteIDAnalysis) discoverConstants() {
	names := make([]string, 0, len(a.writes))
	for name := range a.writes {
		names = append(names, name)
	}
	sort.Strings(names)
	for {
		changed := false
		for _, name := range names {
			if _, resolved := a.constants[name]; resolved {
				continue
			}
			writes := a.writes[name]
			if len(writes) != 1 || a.shadowed[name] || a.functionNames[name] {
				continue
			}
			write := writes[0]
			if !write.valid || !write.topLevel {
				continue
			}
			if localOrder, local := a.topLocals[name]; local {
				if localOrder != write.order || a.firstFunctionOrder >= 0 && localOrder >= a.firstFunctionOrder {
					continue
				}
			}
			value, resolved := a.finiteWriteValue(write)
			if !resolved {
				continue
			}
			a.constants[name] = value
			a.constantWrite[name] = write
			changed = true
		}
		if !changed {
			return
		}
	}
}

// finiteIDs returns every value that the expression can supply to spr(). The
// bool is false for any expression outside the intentionally explicit proof;
// detail is suitable for the source-positioned Mode 0 diagnostic.
func (a *mode0SpriteIDAnalysis) finiteIDs(expr luaparser.Expr) ([]int, bool, string) {
	return a.lookupIDs(expr, true)
}

func (a *mode0SpriteIDAnalysis) enumeratedIDs(expr luaparser.Expr) ([]int, bool, string) {
	return a.lookupIDs(expr, false)
}

func (a *mode0SpriteIDAnalysis) lookupIDs(expr luaparser.Expr, allowBounded bool) ([]int, bool, string) {
	if value, literal := staticNumberLiteral(expr); literal && value == math.Trunc(value) {
		return []int{int(value)}, true, ""
	}
	variable, ok := unparenExpr(expr).(*luaparser.VarExpr)
	if !ok {
		return nil, false, "the id expression is not a scalar variable"
	}
	name := variable.Name
	if a == nil {
		return nil, false, "no finite-set analysis is available"
	}
	if a.shadowed[name] {
		return nil, false, fmt.Sprintf("%s is shadowed by a local, parameter, or loop variable", name)
	}
	if a.functionNames[name] {
		return nil, false, fmt.Sprintf("%s is also a function name", name)
	}
	if value, constant := a.constants[name]; constant {
		return []int{value}, true, ""
	}
	writes := a.writes[name]
	if len(writes) == 0 {
		return nil, false, fmt.Sprintf("%s has no assignments", name)
	}
	if localOrder, local := a.topLocals[name]; local {
		if a.firstFunctionOrder >= 0 && localOrder >= a.firstFunctionOrder {
			return nil, false, fmt.Sprintf("top-level local %s is declared after a function and has ambiguous capture", name)
		}
		for _, write := range writes {
			if write.topLevel && write.order < localOrder {
				return nil, false, fmt.Sprintf("%s is assigned before its top-level local declaration", name)
			}
		}
	}

	initialized := false
	values := map[int]bool{}
	var open *mode0SpriteIDWrite
	for i, write := range writes {
		if !write.valid {
			return nil, false, fmt.Sprintf("%s participates in a multiple or value-less assignment", name)
		}
		if write.topLevel || write.directInit {
			initialized = true
		}
		value, ok := a.finiteWriteValue(write)
		if !ok {
			open = &writes[i]
			continue
		}
		values[value] = true
	}
	if !initialized {
		return nil, false, fmt.Sprintf("%s needs an unconditional top-level or _init assignment", name)
	}
	if open != nil {
		if !allowBounded {
			return nil, false, fmt.Sprintf("%s has a non-enumerated assignment at %s", name, open.position)
		}
		ids, ok, detail := a.boundedCounterIDs(name)
		if ok {
			return ids, true, ""
		}
		if detail == "" {
			detail = fmt.Sprintf("%s has a non-enumerated assignment at %s", name, open.position)
		}
		return nil, false, detail
	}
	ids := make([]int, 0, len(values))
	for value := range values {
		ids = append(ids, value)
	}
	sort.Ints(ids)
	return ids, true, ""
}

func (a *mode0SpriteIDAnalysis) finiteWriteValue(write mode0SpriteIDWrite) (int, bool) {
	if value, literal := staticNumberLiteral(write.value); literal && value == math.Trunc(value) {
		return int(value), true
	}
	variable, ok := unparenExpr(write.value).(*luaparser.VarExpr)
	if !ok {
		return 0, false
	}
	value, constant := a.constants[variable.Name]
	if !constant {
		return 0, false
	}
	// A top-level assignment executes immediately, so its constant must already
	// exist. Function bodies run after all top-level initialization.
	if write.topLevel && a.constantWrite[variable.Name].order >= write.order {
		return 0, false
	}
	return value, true
}
