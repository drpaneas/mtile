package transpiler

import (
	"strconv"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// Reserve only the capacity ordinary doubling would ultimately reach. This
// changes allocation/collection/OOM timing, like other constructor capacity
// hints, but leaves successful-run Lua values, RNG/evaluation order and the
// final ordinary table layout unchanged. It introduces no source API.
func (g *generator) analyzeLoopTableReservations() {
	if g.opts.DisableLoopTableReservations {
		return
	}
	g.literalArrayCaps = make(map[*luaparser.TableExpr]int)
	g.reservedAppendLoops = make(map[*luaparser.NumericForStmt]*luaparser.CallExpr)
	var visit func([]luaparser.Stmt)
	visit = func(body []luaparser.Stmt) {
		for i, stmt := range body {
			if name, table := emptyTableBinding(stmt); table != nil {
				for _, later := range body[i+1:] {
					if loop, ok := later.(*luaparser.NumericForStmt); ok {
						if count := fixedAppendLoop(loop, name); count > 1 {
							capacity := 1
							for capacity < count {
								capacity *= 2
							}
							g.literalArrayCaps[table] = capacity
							g.reservedAppendLoops[loop] = loop.Body.Body[0].(*luaparser.CallStmt).Call
							break
						}
					}
					if !reservationIndependentStmt(later, name) {
						break
					}
				}
			}
			switch s := stmt.(type) {
			case *luaparser.FunctionDeclStmt:
				visit(s.Body.Body)
			case *luaparser.DoStmt:
				visit(s.Body.Body)
			case *luaparser.NumericForStmt:
				visit(s.Body.Body)
			case *luaparser.GenericForStmt:
				visit(s.Body.Body)
			case *luaparser.WhileStmt:
				visit(s.Body.Body)
			case *luaparser.RepeatStmt:
				visit(s.Body.Body)
			case *luaparser.IfStmt:
				visit(s.Then.Body)
				for _, branch := range s.ElseIf {
					visit(branch.Body.Body)
				}
				if s.Else != nil {
					visit(s.Else.Body)
				}
			}
		}
	}
	visit(g.chunk.Body)
}

func emptyTableBinding(stmt luaparser.Stmt) (string, *luaparser.TableExpr) {
	var name string
	var value luaparser.Expr
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		if len(s.Targets) != 1 || len(s.Values) != 1 {
			return "", nil
		}
		variable, ok := s.Targets[0].(*luaparser.VarExpr)
		if !ok {
			return "", nil
		}
		name, value = variable.Name, s.Values[0]
	case *luaparser.LocalAssignStmt:
		if len(s.Names) != 1 || len(s.Values) != 1 {
			return "", nil
		}
		name, value = s.Names[0], s.Values[0]
	default:
		return "", nil
	}
	table, ok := unparenExpr(value).(*luaparser.TableExpr)
	if !ok || len(table.Fields) != 0 {
		return "", nil
	}
	return name, table
}

func fixedAppendLoop(loop *luaparser.NumericForStmt, name string) int {
	if loop.Name == name || len(loop.Body.Body) != 1 {
		return 0
	}
	statement, ok := loop.Body.Body[0].(*luaparser.CallStmt)
	if !ok {
		return 0
	}
	call := statement.Call
	key, ok := callKey(call)
	if !ok || key != "add" || len(call.Args) != 2 {
		return 0
	}
	base, ok := unparenExpr(call.Args[0]).(*luaparser.VarExpr)
	if !ok || base.Name != name || !reservationIndependentExpr(call.Args[1], name) {
		return 0
	}
	switch unparenExpr(call.Args[1]).(type) {
	case *luaparser.TableExpr, *luaparser.NumberExpr, *luaparser.StringExpr, *luaparser.BoolExpr:
	default:
		return 0 // A possibly-nil append need not extend the dense prefix.
	}
	return fixedIntegerLoopCount(loop)
}

func integerLoopExpr(expr luaparser.Expr) (int64, bool) {
	sign := int64(1)
	expr = unparenExpr(expr)
	if negative, ok := expr.(*luaparser.UnaryExpr); ok && negative.Op == luaparser.OpNegate {
		sign, expr = -1, unparenExpr(negative.Expr)
	}
	number, ok := expr.(*luaparser.NumberExpr)
	if !ok || !number.Integer || number.Value < -32768 || number.Value > 32768 {
		return 0, false
	}
	value := sign * int64(number.Value)
	return value, value >= -32768 && value <= 32767
}

func integerNumericForBounds(loop *luaparser.NumericForStmt) (init, limit, step int64, ok bool) {
	init, initOK := integerLoopExpr(loop.Init)
	limit, limitOK := integerLoopExpr(loop.Limit)
	step, stepOK := integerLoopExpr(loop.Step)
	ok = initOK && limitOK && stepOK && step != 0 && !((step > 0 && init > limit) || (step < 0 && init < limit))
	return init, limit, step, ok
}

func integerNumericForHeader(name string, init, limit, step int64) string {
	cond := "<="
	if step < 0 {
		cond = ">="
	}
	inc := name + "++"
	switch {
	case step == 1:
	case step == -1:
		inc = name + "--"
	case step > 1:
		inc = name + " += " + strconv.FormatInt(step, 10)
	default:
		inc = name + " -= " + strconv.FormatInt(-step, 10)
	}
	return "for " + name + " := " + strconv.FormatInt(init, 10) + "; " + name + " " + cond + " " + strconv.FormatInt(limit, 10) + "; " + inc + " {"
}

func fixedIntegerLoopCount(loop *luaparser.NumericForStmt) int {
	init, limit, step, ok := integerNumericForBounds(loop)
	if !ok {
		return 0
	}
	count := (limit-init)/step + 1
	next := init + count*step
	if count < 1 || count > maxNumericArrayLength || next < -32768 || next > 32767 {
		return 0 // The fixed-point induction must terminate without wrapping.
	}
	return int(count)
}

// No reference to the fresh binding and no user/unknown call may intervene:
// otherwise the table could escape through a global or change before filling.
// Unrelated initialization and independent fixed loops may allocate normally.
func reservationIndependentStmt(stmt luaparser.Stmt, name string) bool {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, target := range s.Targets {
			variable, ok := target.(*luaparser.VarExpr)
			if !ok || variable.Name == name {
				return false
			}
		}
		for _, value := range s.Values {
			if !reservationIndependentExpr(value, name) {
				return false
			}
		}
		return true
	case *luaparser.LocalAssignStmt:
		for _, local := range s.Names {
			if local == name {
				return false
			}
		}
		for _, value := range s.Values {
			if !reservationIndependentExpr(value, name) {
				return false
			}
		}
		return true
	case *luaparser.CallStmt:
		return reservationIndependentExpr(s.Call, name)
	case *luaparser.NumericForStmt:
		if s.Name == name || fixedIntegerLoopCount(s) == 0 {
			return false
		}
		for _, stmt := range s.Body.Body {
			if !reservationIndependentStmt(stmt, name) {
				return false
			}
		}
		return true
	}
	return false
}

func reservationIndependentExpr(expr luaparser.Expr, name string) bool {
	switch x := expr.(type) {
	case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr, *luaparser.StringExpr:
		return true
	case *luaparser.VarExpr:
		return x.Name != name
	case *luaparser.ParenExpr:
		return reservationIndependentExpr(x.Expr, name)
	case *luaparser.UnaryExpr:
		return reservationIndependentExpr(x.Expr, name)
	case *luaparser.BinaryExpr:
		return reservationIndependentExpr(x.Left, name) && reservationIndependentExpr(x.Right, name)
	case *luaparser.IndexExpr:
		return reservationIndependentExpr(x.Base, name) && reservationIndependentExpr(x.Key, name)
	case *luaparser.FieldExpr:
		return reservationIndependentExpr(x.Base, name)
	case *luaparser.TableExpr:
		for _, field := range x.Fields {
			if !reservationIndependentExpr(field.Key, name) || !reservationIndependentExpr(field.Value, name) {
				return false
			}
		}
		return true
	case *luaparser.CallExpr:
		key, ok := callKey(x)
		if !ok || (key != "add" && !numberArrayViewBuiltin(key)) {
			return false
		}
		for _, arg := range x.Args {
			if !reservationIndependentExpr(arg, name) {
				return false
			}
		}
		return true
	}
	return false
}
