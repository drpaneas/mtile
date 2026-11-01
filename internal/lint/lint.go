// Package lint finds reviewable Effective mTile opportunities in Lua source.
// Diagnostics are advice, not proof of a speedup or permission to change behavior.
package lint

import (
	"fmt"
	"math"
	"sort"
	"strings"

	p "github.com/drpaneas/mtile/internal/luaparser"
	"github.com/drpaneas/mtile/internal/sourcebundle"
)

type Rule struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Guide string `json:"guide"`
}

var rules = []Rule{
	{"MTL001", "frame-table-allocation", "reuse-storage-when-the-same-slots-will-do"},
	{"MTL002", "numeric-list-alias", "make-a-fixed-list-easy-to-recognise"},
	{"MTL003", "frame-palette-reset", "change-the-colours-that-actually-change"},
	{"MTL004", "literal-map-edit", "keep-permanent-world-data-in-the-assets"},
	{"MTL005", "split-fixed-label", "keep-a-fixed-label-whole"},
	{"MTL006", "pixel-rectangle", "ask-for-the-picture-you-mean"},
	{"MTL007", "mixed-slot-meaning", "keep-one-number-about-one-thing"},
	{"MTL008", "discarded-table-construction", "reuse-storage-when-the-same-slots-will-do"},
}

// Rules returns the stable diagnostic IDs available to CLI/CI integrations.
func Rules() []Rule { return append([]Rule(nil), rules...) }

type Diagnostic struct {
	Rule          string            `json:"rule"`
	Path          string            `json:"path"`
	Line          int               `json:"line"`
	Column        int               `json:"column"`
	Message       string            `json:"message"`
	Suggestion    string            `json:"suggestion"`
	Guide         string            `json:"guide"`
	Severity      string            `json:"severity"`
	Label         string            `json:"label"`
	Snippet       []SourceLine      `json:"snippet"`
	Notes         []string          `json:"notes,omitempty"`
	CallPath      []string          `json:"call_path,omitempty"`
	ConditionalAt *Location         `json:"conditional_at,omitempty"`
	Documentation string            `json:"documentation,omitempty"`
	Related       []RelatedLocation `json:"related,omitempty"`
}

type RelatedLocation struct {
	Location
	Label   string       `json:"label"`
	Snippet []SourceLine `json:"snippet"`
}

type SourceLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type analyzer struct {
	bundle        sourcebundle.Bundle
	funcs         map[string]*p.FunctionDeclStmt
	blocked       map[string]bool
	writes        map[string]int
	arrays        map[string]bool
	tables        map[string]bool
	slots         map[string]map[int]uint8
	diagnostics   []Diagnostic
	reported      map[string]bool
	lines         []string
	poolFallbacks map[*p.TableExpr]string
}

// Analyze shares the build's require graph and source-position mapping. It does
// not run game code, load assets, invoke TinyGo, or estimate target cycles.
func Analyze(bundle sourcebundle.Bundle) ([]Diagnostic, error) {
	chunk, err := p.ParseString(bundle.Entry, bundle.Source)
	if err != nil {
		return nil, bundle.RemapError(err)
	}
	a := &analyzer{
		bundle:        bundle,
		funcs:         map[string]*p.FunctionDeclStmt{},
		blocked:       map[string]bool{},
		writes:        map[string]int{},
		arrays:        map[string]bool{},
		tables:        map[string]bool{},
		slots:         map[string]map[int]uint8{},
		reported:      map[string]bool{},
		diagnostics:   []Diagnostic{},
		lines:         strings.Split(bundle.Source, "\n"),
		poolFallbacks: map[*p.TableExpr]string{},
	}
	a.collect(chunk)
	hot := a.frameFunctions()
	functions := make([]*p.FunctionDeclStmt, 0, len(a.funcs))
	for _, fn := range a.funcs {
		functions = append(functions, fn)
	}
	sort.Slice(functions, func(i, j int) bool {
		x, y := functions[i].Pos(), functions[j].Pos()
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		return x.Column < y.Column
	})
	// Identity escapes affect the whole-program numeric-list proof, even in
	// startup/dead helpers. Other rules only inspect named frame-reachable code.
	a.checkAliases(chunk.Body)
	for _, fn := range functions {
		a.checkAliases(fn.Body.Body)
	}
	for _, fn := range functions {
		if path, ok := hot[fn.Name[0]]; ok {
			a.findPoolFallbacks(fn.Body.Body)
			a.checkFrame(fn.Body.Body, path)
		}
	}
	a.checkDeadConstructions(chunk, functions, hot)
	sort.Slice(a.diagnostics, func(i, j int) bool {
		x, y := a.diagnostics[i], a.diagnostics[j]
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		if x.Column != y.Column {
			return x.Column < y.Column
		}
		return x.Rule < y.Rule
	})
	return a.diagnostics, nil
}

func (a *analyzer) collect(chunk *p.Chunk) {
	counts := map[string]int{}
	for _, stmt := range chunk.Body {
		if fn, ok := stmt.(*p.FunctionDeclStmt); ok && len(fn.Name) == 1 && fn.Method == "" {
			counts[fn.Name[0]]++
			a.funcs[fn.Name[0]] = fn
		}
	}
	var collect visitor
	collect.stmt = func(stmt p.Stmt, _ context) {
		switch s := stmt.(type) {
		case *p.LocalAssignStmt:
			for _, name := range s.Names {
				a.blocked[name] = true
			}
		case *p.NumericForStmt:
			a.blocked[s.Name] = true
		case *p.GenericForStmt:
			for _, name := range s.Names {
				a.blocked[name] = true
			}
		case *p.AssignStmt:
			for i, target := range s.Targets {
				if name := variable(target); name != "" {
					a.writes[name]++
				}
				if i < len(s.Values) {
					a.recordSlot(target, s.Values[i])
				}
			}
		case *p.FunctionDeclStmt:
			if len(s.Name) == 1 {
				if a.funcs[s.Name[0]] != s {
					a.blocked[s.Name[0]] = true
				}
			}
			for _, name := range s.Params {
				a.blocked[name] = true
			}
			collect.statements(s.Body.Body, context{})
		}
	}
	collect.statements(chunk.Body, context{})
	for name := range a.funcs {
		if counts[name] != 1 || a.blocked[name] || a.writes[name] != 0 {
			a.blocked[name] = true
			delete(a.funcs, name)
		}
	}
	for _, stmt := range chunk.Body {
		s, ok := stmt.(*p.AssignStmt)
		if !ok || len(s.Targets) != 1 || len(s.Values) != 1 {
			continue
		}
		name := variable(s.Targets[0])
		table, ok := unparen(s.Values[0]).(*p.TableExpr)
		if !ok || name == "" || a.blocked[name] || a.writes[name] != 1 {
			continue
		}
		a.tables[name] = true
		numeric := len(table.Fields) > 0 && len(table.Fields) <= 256
		arrayIndex := 1
		for _, field := range table.Fields {
			index := 0
			if field.Array {
				index = arrayIndex
				arrayIndex++
			} else if n, ok := integer(field.Key); ok {
				index = n
			}
			if index > 0 {
				a.addKind(name, index, kind(field.Value))
			}
			_, literal := unparen(field.Value).(*p.NumberExpr)
			numeric = numeric && field.Array && literal
		}
		if numeric {
			a.arrays[name] = true
		}
	}
}

// Deliberately conservative: any same-named local/parameter anywhere prevents
// a global/function/builtin assumption. We skip ambiguous bindings rather than
// claim a whole-program type or reachability proof we do not have.
func (a *analyzer) builtin(call *p.CallExpr, name string) bool {
	return call.Method == "" && variable(call.Callee) == name && !a.blocked[name] && a.writes[name] == 0 && a.funcs[name] == nil
}

func (a *analyzer) emit(id string, node p.Node, message, suggestion string) *Diagnostic {
	pos := node.Pos()
	key := fmt.Sprintf("%s:%d:%d", id, pos.Line, pos.Column)
	if a.reported[key] {
		return nil
	}
	a.reported[key] = true
	path, line, column, ok := a.bundle.Position(pos.Line, pos.Column)
	if !ok {
		path, line, column = pos.Source, pos.Line, pos.Column
	}
	guide := ""
	for _, rule := range rules {
		if rule.ID == id {
			guide = "docs/performance.md#" + rule.Guide
			break
		}
	}
	d := Diagnostic{Rule: id, Path: path, Line: line, Column: column, Message: message, Suggestion: suggestion, Guide: guide, Severity: "advice", Label: diagnosticLabel(id)}
	d.Snippet = a.snippet(pos)
	a.diagnostics = append(a.diagnostics, d)
	return &a.diagnostics[len(a.diagnostics)-1]
}

func (a *analyzer) snippet(pos p.Position) []SourceLine {
	path := a.location(pos).Path
	var lines []SourceLine
	for n := max(1, pos.Line-1); n <= min(len(a.lines), pos.Line+1); n++ {
		if _, _, _, ok := a.bundle.Position(n, 1); !ok && len(a.bundle.Spans) > 0 {
			continue
		}
		location := a.location(p.Position{Source: pos.Source, Line: n, Column: 1})
		if location.Path == path {
			lines = append(lines, SourceLine{location.Line, strings.TrimSuffix(a.lines[n-1], "\r")})
		}
	}
	return lines
}

func (a *analyzer) checkAliases(body []p.Stmt) {
	v := visitor{stmt: func(stmt p.Stmt, _ context) {
		var values []p.Expr
		switch s := stmt.(type) {
		case *p.LocalAssignStmt:
			values = s.Values
		case *p.AssignStmt:
			values = s.Values
		}
		for _, value := range values {
			if name := variable(value); a.arrays[name] {
				a.emit("MTL002", value, fmt.Sprintf("Whole-list alias of fixed numeric candidate %q prevents its direct-use storage proof.", name), "Use direct indexed accesses if the second name is unnecessary. Keep aliases when shared table identity is part of the design; this is only one eligibility condition.")
			}
		}
	}}
	v.statements(body, context{})
}

func (a *analyzer) checkFrame(body []p.Stmt, path framePath) {
	v := visitor{
		stmt: func(stmt p.Stmt, c context) {
			if loop, ok := stmt.(*p.NumericForStmt); ok {
				a.rectangle(loop)
			}
			if s, ok := stmt.(*p.AssignStmt); ok {
				for i, target := range s.Targets {
					if i >= len(s.Values) {
						break
					}
					index, ok := unparen(target).(*p.IndexExpr)
					if !ok {
						continue
					}
					name := variable(index.Base)
					key, ok := integer(index.Key)
					if !ok || !a.tables[name] || a.slots[name][key] != 3 || kind(s.Values[i]) == 0 {
						continue
					}
					tag := fmt.Sprintf("mixed:%s:%d", name, key)
					if a.reported[tag] {
						continue
					}
					a.reported[tag] = true
					a.emit("MTL007", target, fmt.Sprintf("Slot %s[%d] has both literal numeric and nonnumeric values in this program.", name, key), "For hot numeric state, consider a separate active/existence flag and a consistently numeric value. Keep mixed storage if its flexibility is needed; zero may be a real coordinate.")
				}
			}
		},
		expr: func(expr p.Expr, c context) {
			if table, ok := expr.(*p.TableExpr); ok {
				a.tableAllocation(table, c, path)
			}
			call, ok := expr.(*p.CallExpr)
			if !ok {
				return
			}
			paletteReset := len(call.Args) == 0
			if len(call.Args) == 1 {
				layer, literal := integer(call.Args[0])
				paletteReset = literal && (layer == 0 || layer == 1)
			}
			if a.builtin(call, "pal") && paletteReset {
				a.emit("MTL003", call, "A full palette mapping reset is reachable from a frame callback.", "If only a few mappings change, consider updating those entries on the same layer. Keep resets required by drawing/display state or full effects; compare complete palette results.")
			}
			if a.builtin(call, "mset") && (len(call.Args) == 3 || len(call.Args) == 4) {
				literal := true
				for _, arg := range call.Args {
					if _, ok := integer(arg); !ok {
						literal = false
					}
				}
				if literal {
					a.emit("MTL004", call, "A map edit with literal coordinates and tile is reachable from a frame callback.", "If this cell is permanent, author it in the map asset and remove the repeated edit only after checking all writers/readers. Keep event-driven doors, destruction and resets.")
				}
			}
			if a.builtin(call, "print") && len(call.Args) > 0 && len(c.loops) > 0 {
				sub, ok := unparen(call.Args[0]).(*p.CallExpr)
				if !ok || !a.builtin(sub, "sub") || len(sub.Args) != 3 {
					return
				}
				label, ok := unparen(sub.Args[0]).(*p.StringExpr)
				if !ok || len(label.Value) < 2 || strings.IndexFunc(label.Value, func(r rune) bool { return r > 127 }) >= 0 {
					return
				}
				loop := c.loops[len(c.loops)-1]
				// A single-call body avoids confusing a shadowed or reassigned
				// same-spelled local with the loop's induction variable.
				if len(loop.Body.Body) != 1 {
					return
				}
				stmt, singleCall := loop.Body.Body[0].(*p.CallStmt)
				if !singleCall || stmt.Call != call {
					return
				}
				start, sok := integer(loop.Init)
				end, eok := integer(loop.Limit)
				if sok && eok && start == 1 && end == len(label.Value) && unitStep(loop) && variable(sub.Args[1]) == loop.Name && variable(sub.Args[2]) == loop.Name {
					a.emit("MTL005", call, "A fixed label is sliced and printed one character at a time in a loop.", "Use one literal print when font spacing, colours and positions produce the same picture. Keep separate glyphs for typewriter effects or per-letter motion.")
				}
			}
		},
	}
	v.statements(body, context{})
}

func (a *analyzer) rectangle(outer *p.NumericForStmt) {
	if len(outer.Body.Body) != 1 || !literalRange(outer) {
		return
	}
	inner, ok := outer.Body.Body[0].(*p.NumericForStmt)
	if !ok || len(inner.Body.Body) != 1 || inner.Name == outer.Name || !literalRange(inner) {
		return
	}
	stmt, ok := inner.Body.Body[0].(*p.CallStmt)
	if !ok || !a.builtin(stmt.Call, "pset") || len(stmt.Call.Args) != 3 {
		return
	}
	x, y := variable(stmt.Call.Args[0]), variable(stmt.Call.Args[1])
	if !(x == inner.Name && y == outer.Name || x == outer.Name && y == inner.Name) {
		return
	}
	if _, ok := integer(stmt.Call.Args[2]); !ok {
		return
	}
	a.emit("MTL006", outer, "Two literal unit-step loops paint a solid rectangle pixel by pixel.", "Consider rectfill with the same inclusive bounds and colour. Confirm clipping, camera and final pixels with the selected renderer before keeping the rewrite.")
}

func (a *analyzer) recordSlot(target, value p.Expr) {
	index, ok := unparen(target).(*p.IndexExpr)
	if !ok {
		return
	}
	key, ok := integer(index.Key)
	if ok && key > 0 && key <= 256 {
		a.addKind(variable(index.Base), key, kind(value))
	}
}

func (a *analyzer) addKind(name string, key int, k uint8) {
	if name == "" || k == 0 {
		return
	}
	if a.slots[name] == nil {
		a.slots[name] = map[int]uint8{}
	}
	a.slots[name][key] |= k
}

func kind(expr p.Expr) uint8 {
	if _, ok := number(expr); ok {
		return 1
	}
	switch unparen(expr).(type) {
	case *p.StringExpr, *p.BoolExpr, *p.NilExpr:
		return 2
	}
	return 0
}

func unparen(expr p.Expr) p.Expr {
	for {
		x, ok := expr.(*p.ParenExpr)
		if !ok {
			return expr
		}
		expr = x.Expr
	}
}

func variable(expr p.Expr) string {
	if v, ok := unparen(expr).(*p.VarExpr); ok {
		return v.Name
	}
	return ""
}

func number(expr p.Expr) (float64, bool) {
	switch e := unparen(expr).(type) {
	case *p.NumberExpr:
		return e.Value, true
	case *p.UnaryExpr:
		if e.Op == p.OpNegate {
			n, ok := number(e.Expr)
			return -n, ok
		}
	}
	return 0, false
}

func integer(expr p.Expr) (int, bool) {
	n, ok := number(expr)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < -32768 || n > 32767 {
		return 0, false
	}
	return int(n), true
}

func unitStep(loop *p.NumericForStmt) bool {
	if loop.Step == nil {
		return true
	}
	n, ok := integer(loop.Step)
	return ok && n == 1
}

func literalRange(loop *p.NumericForStmt) bool {
	start, sok := integer(loop.Init)
	end, eok := integer(loop.Limit)
	return sok && eok && end >= start && end < 32767 && unitStep(loop)
}
