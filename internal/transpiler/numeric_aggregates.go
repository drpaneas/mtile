package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
)

const maxNumericArrayLength = 256

// analyzeNumericArrays finds a deliberately narrow compact-storage shape:
// fixed-length numeric literals declared at chunk scope and used only through
// direct integer indexing (or #array). Any escape, rebinding, or shadowing
// keeps the ordinary Lua table representation. Element writes are converted
// to Number once; enabling the experiment opts into the monomorphic contract
// that their type will not change later.
func (g *generator) analyzeNumericArrays() {
	if g.opts.DisableNumericArrays || g.opts.DisableMonomorphicNumbers {
		return
	}
	for _, stmt := range g.chunk.Body {
		declaration, ok := stmt.(*luaparser.LocalAssignStmt)
		if !ok || len(declaration.Names) != 1 || len(declaration.Values) != 1 {
			continue
		}
		literal, ok := declaration.Values[0].(*luaparser.TableExpr)
		if !ok || len(literal.Fields) == 0 || len(literal.Fields) > maxNumericArrayLength {
			continue
		}
		numeric := true
		for _, field := range literal.Fields {
			if !field.Array {
				numeric = false
				break
			}
			if _, ok := field.Value.(*luaparser.NumberExpr); !ok {
				numeric = false
				break
			}
		}
		if numeric {
			name := declaration.Names[0]
			g.numericArrays[name] = &numericArrayInfo{declaration: declaration, literal: literal, length: len(literal.Fields)}
		}
	}
	for name, info := range g.numericArrays {
		if !g.numericArrayUsesAreSafe(name, info.declaration) {
			delete(g.numericArrays, name)
		}
	}
}

func (g *generator) numericArrayUsesAreSafe(name string, declaration *luaparser.LocalAssignStmt) bool {
	var obviouslyNonNumeric func(luaparser.Expr) bool
	obviouslyNonNumeric = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case *luaparser.ParenExpr:
			return obviouslyNonNumeric(x.Expr)
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
			return true
		case *luaparser.UnaryExpr:
			return x.Op == luaparser.OpNot
		case *luaparser.BinaryExpr:
			switch x.Op {
			case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess,
				luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq,
				luaparser.OpConcat:
				return true
			}
		}
		return false
	}
	var safeExpr func(luaparser.Expr) bool
	safeExpr = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr, *luaparser.StringExpr:
			return true
		case *luaparser.ParenExpr:
			return safeExpr(x.Expr)
		case *luaparser.VarExpr:
			return x.Name != name
		case *luaparser.UnaryExpr:
			if x.Op == luaparser.OpLength {
				if base, ok := x.Expr.(*luaparser.VarExpr); ok && base.Name == name {
					return true
				}
			}
			return safeExpr(x.Expr)
		case *luaparser.BinaryExpr:
			return safeExpr(x.Left) && safeExpr(x.Right)
		case *luaparser.IndexExpr:
			if base, ok := x.Base.(*luaparser.VarExpr); ok && base.Name == name {
				return safeExpr(x.Key)
			}
			return safeExpr(x.Base) && safeExpr(x.Key)
		case *luaparser.FieldExpr:
			return safeExpr(x.Base)
		case *luaparser.CallExpr:
			if !safeExpr(x.Callee) {
				return false
			}
			for _, argument := range x.Args {
				if !safeExpr(argument) {
					return false
				}
			}
			return true
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if !safeExpr(field.Key) || !safeExpr(field.Value) {
					return false
				}
			}
			return true
		}
		return true
	}

	var safeBlock func(*luaparser.Block) bool
	var safeStmt func(luaparser.Stmt) bool
	safeBlock = func(block *luaparser.Block) bool {
		for _, stmt := range block.Body {
			if !safeStmt(stmt) {
				return false
			}
		}
		return true
	}
	safeStmt = func(stmt luaparser.Stmt) bool {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for index, target := range s.Targets {
				if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == name {
					return false
				}
				if indexed, ok := target.(*luaparser.IndexExpr); ok {
					if base, ok := indexed.Base.(*luaparser.VarExpr); ok && base.Name == name {
						if index >= len(s.Values) || obviouslyNonNumeric(s.Values[index]) {
							return false
						}
					}
				}
				if !safeExpr(target) {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.LocalAssignStmt:
			if s != declaration {
				for _, local := range s.Names {
					if local == name {
						return false
					}
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.CallStmt:
			return safeExpr(s.Call)
		case *luaparser.IfStmt:
			if !safeExpr(s.Condition) || !safeBlock(s.Then) {
				return false
			}
			for _, branch := range s.ElseIf {
				if !safeExpr(branch.Condition) || !safeBlock(branch.Body) {
					return false
				}
			}
			return s.Else == nil || safeBlock(s.Else)
		case *luaparser.WhileStmt:
			return safeExpr(s.Condition) && safeBlock(s.Body)
		case *luaparser.RepeatStmt:
			return safeBlock(s.Body) && safeExpr(s.Condition)
		case *luaparser.DoStmt:
			return safeBlock(s.Body)
		case *luaparser.NumericForStmt:
			return s.Name != name && safeExpr(s.Init) && safeExpr(s.Limit) && safeExpr(s.Step) && safeBlock(s.Body)
		case *luaparser.GenericForStmt:
			for _, local := range s.Names {
				if local == name {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
			return safeBlock(s.Body)
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.FunctionDeclStmt:
			for _, parameter := range s.Params {
				if parameter == name {
					return false
				}
			}
			return safeBlock(s.Body)
		}
		return true
	}

	for _, stmt := range g.chunk.Body {
		if !safeStmt(stmt) {
			return false
		}
	}
	return true
}

const maxNumericRecordFields = 32

// analyzeNumericRecords performs scalar replacement for one deliberately
// narrow Lua record shape: a chunk-scope local table with unique named fields,
// numeric initializers, and no identity-observing use. Any alias, dynamic key,
// rebinding, shadowing, shape change, or obviously nonnumeric write keeps the
// ordinary heap-table representation.
func (g *generator) analyzeNumericRecords() {
	if g.opts.DisableMonomorphicNumbers || g.opts.DisableStableTableFields {
		return
	}
	for _, stmt := range g.chunk.Body {
		name, literal, ok := numericRecordDeclaration(stmt)
		if !ok || len(literal.Fields) == 0 || len(literal.Fields) > maxNumericRecordFields {
			continue
		}
		info := &numericRecordInfo{
			declaration: stmt,
			literal:     literal,
			fields:      make(map[string]*numericRecordFieldInfo, len(literal.Fields)),
		}
		valid := true
		for index, field := range literal.Fields {
			key, named := field.Key.(*luaparser.StringExpr)
			if field.Array || !named || key.Value == "" || info.fields[key.Value] != nil ||
				!g.numericRecordInitializerSafe(field.Value, stmt) {
				valid = false
				break
			}
			recordField := &numericRecordFieldInfo{name: key.Value, value: field.Value, index: index}
			info.fields[key.Value] = recordField
			info.fieldOrder = append(info.fieldOrder, recordField)
		}
		if valid {
			g.numericRecords[name] = info
			g.numericRecordOrder = append(g.numericRecordOrder, info)
		}
	}

	kept := g.numericRecordOrder[:0]
	for name, info := range g.numericRecords {
		if !g.numericRecordUsesAreSafe(name, info) {
			delete(g.numericRecords, name)
		}
	}
	// Restore source order after the map-based validation pass and assign
	// deterministic generated member indices.
	for _, stmt := range g.chunk.Body {
		name, _, ok := numericRecordDeclaration(stmt)
		info := g.numericRecords[name]
		if !ok || info == nil || info.declaration != stmt {
			continue
		}
		info.index = len(kept)
		kept = append(kept, info)
		for _, field := range info.literal.Fields {
			g.numericRecordKeys[field.Key.(*luaparser.StringExpr)] = true
		}
	}
	g.numericRecordOrder = kept
}

func (g *generator) numericRecordInitializerSafe(expr luaparser.Expr, declaration luaparser.Stmt) bool {
	switch x := expr.(type) {
	case *luaparser.NumberExpr:
		return true
	case *luaparser.ParenExpr:
		return g.numericRecordInitializerSafe(x.Expr, declaration)
	case *luaparser.VarExpr:
		if g.globalKinds[x.Name] != monoNumber {
			return false
		}
		for _, stmt := range g.chunk.Body {
			if stmt == declaration {
				return false
			}
			switch s := stmt.(type) {
			case *luaparser.LocalAssignStmt:
				for _, name := range s.Names {
					if name == x.Name {
						return true
					}
				}
			case *luaparser.AssignStmt:
				for _, target := range s.Targets {
					if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == x.Name {
						return true
					}
				}
			}
		}
		return false
	case *luaparser.UnaryExpr:
		return (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) && g.numericRecordInitializerSafe(x.Expr, declaration)
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
			luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
			luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
			luaparser.OpRotateL, luaparser.OpRotateR:
			return g.numericRecordInitializerSafe(x.Left, declaration) && g.numericRecordInitializerSafe(x.Right, declaration)
		}
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		switch name {
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat", "mget":
			for _, argument := range x.Args {
				if !g.numericRecordInitializerSafe(argument, declaration) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func numericRecordDeclaration(stmt luaparser.Stmt) (string, *luaparser.TableExpr, bool) {
	switch declaration := stmt.(type) {
	case *luaparser.LocalAssignStmt:
		if len(declaration.Names) != 1 || len(declaration.Values) != 1 {
			return "", nil, false
		}
		literal, ok := declaration.Values[0].(*luaparser.TableExpr)
		return declaration.Names[0], literal, ok
	case *luaparser.AssignStmt:
		if len(declaration.Targets) != 1 || len(declaration.Values) != 1 {
			return "", nil, false
		}
		variable, ok := declaration.Targets[0].(*luaparser.VarExpr)
		if !ok {
			return "", nil, false
		}
		literal, ok := declaration.Values[0].(*luaparser.TableExpr)
		return variable.Name, literal, ok
	default:
		return "", nil, false
	}
}

func (g *generator) numericRecordUsesAreSafe(name string, info *numericRecordInfo) bool {
	for _, stmt := range g.chunk.Body {
		if stmt == info.declaration {
			break
		}
		referenced := false
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if variable, ok := expr.(*luaparser.VarExpr); ok && variable.Name == name {
				referenced = true
			}
		})
		if referenced {
			return false
		}
	}
	obviouslyNonNumeric := func(expr luaparser.Expr) bool {
		for {
			paren, ok := expr.(*luaparser.ParenExpr)
			if !ok {
				break
			}
			expr = paren.Expr
		}
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
			return true
		case *luaparser.UnaryExpr:
			return x.Op == luaparser.OpNot
		case *luaparser.BinaryExpr:
			switch x.Op {
			case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess,
				luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq,
				luaparser.OpConcat, luaparser.OpAnd, luaparser.OpOr:
				return true
			}
		}
		return false
	}
	directField := func(expr luaparser.Expr) (*luaparser.FieldExpr, bool) {
		field, ok := expr.(*luaparser.FieldExpr)
		if !ok {
			return nil, false
		}
		base, ok := field.Base.(*luaparser.VarExpr)
		return field, ok && base.Name == name
	}
	var safeExpr func(luaparser.Expr) bool
	safeExpr = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr, *luaparser.StringExpr:
			return true
		case *luaparser.ParenExpr:
			return safeExpr(x.Expr)
		case *luaparser.VarExpr:
			return x.Name != name
		case *luaparser.UnaryExpr:
			return safeExpr(x.Expr)
		case *luaparser.BinaryExpr:
			return safeExpr(x.Left) && safeExpr(x.Right)
		case *luaparser.IndexExpr:
			if base, ok := x.Base.(*luaparser.VarExpr); ok && base.Name == name {
				return false
			}
			return safeExpr(x.Base) && safeExpr(x.Key)
		case *luaparser.FieldExpr:
			if field, ok := directField(x); ok {
				return info.fields[field.Name] != nil
			}
			return safeExpr(x.Base)
		case *luaparser.CallExpr:
			if !safeExpr(x.Callee) {
				return false
			}
			for _, argument := range x.Args {
				if !safeExpr(argument) {
					return false
				}
			}
			return true
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if !safeExpr(field.Key) || !safeExpr(field.Value) {
					return false
				}
			}
			return true
		}
		return true
	}

	var safeBlock func(*luaparser.Block) bool
	var safeStmt func(luaparser.Stmt) bool
	safeBlock = func(block *luaparser.Block) bool {
		for _, stmt := range block.Body {
			if !safeStmt(stmt) {
				return false
			}
		}
		return true
	}
	safeStmt = func(stmt luaparser.Stmt) bool {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for index, target := range s.Targets {
				if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == name {
					if s != info.declaration {
						return false
					}
					continue
				}
				if field, ok := directField(target); ok {
					if info.fields[field.Name] == nil || len(s.Targets) != len(s.Values) || index >= len(s.Values) || obviouslyNonNumeric(s.Values[index]) {
						return false
					}
				}
				if !safeExpr(target) {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.LocalAssignStmt:
			if luaparser.Stmt(s) != info.declaration {
				for _, local := range s.Names {
					if local == name {
						return false
					}
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.CallStmt:
			return safeExpr(s.Call)
		case *luaparser.IfStmt:
			if !safeExpr(s.Condition) || !safeBlock(s.Then) {
				return false
			}
			for _, branch := range s.ElseIf {
				if !safeExpr(branch.Condition) || !safeBlock(branch.Body) {
					return false
				}
			}
			return s.Else == nil || safeBlock(s.Else)
		case *luaparser.WhileStmt:
			return safeExpr(s.Condition) && safeBlock(s.Body)
		case *luaparser.RepeatStmt:
			return safeBlock(s.Body) && safeExpr(s.Condition)
		case *luaparser.DoStmt:
			return safeBlock(s.Body)
		case *luaparser.NumericForStmt:
			return s.Name != name && safeExpr(s.Init) && safeExpr(s.Limit) && safeExpr(s.Step) && safeBlock(s.Body)
		case *luaparser.GenericForStmt:
			for _, local := range s.Names {
				if local == name {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
			return safeBlock(s.Body)
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.FunctionDeclStmt:
			if len(s.Name) != 0 && s.Name[0] == name {
				return false
			}
			for _, parameter := range s.Params {
				if parameter == name {
					return false
				}
			}
			return safeBlock(s.Body)
		}
		return true
	}
	for _, stmt := range g.chunk.Body {
		if !safeStmt(stmt) {
			return false
		}
	}
	return true
}

func (g *generator) numericRecordField(expr *luaparser.FieldExpr) (*numericRecordInfo, *numericRecordFieldInfo, bool) {
	if expr == nil {
		return nil, nil, false
	}
	base, ok := expr.Base.(*luaparser.VarExpr)
	if !ok {
		return nil, nil, false
	}
	if _, local := g.lookupLocal(base.Name); local {
		return nil, nil, false
	}
	info := g.numericRecords[base.Name]
	if info == nil || info.fields[expr.Name] == nil {
		return nil, nil, false
	}
	return info, info.fields[expr.Name], true
}

func numericRecordMember(info *numericRecordInfo, field *numericRecordFieldInfo) string {
	return fmt.Sprintf("_gluRecord%dField%d", info.index, field.index)
}

func (g *generator) numericArrayIndex(expr *luaparser.IndexExpr) (string, *numericArrayInfo, bool) {
	if expr == nil {
		return "", nil, false
	}
	base, ok := expr.Base.(*luaparser.VarExpr)
	if !ok {
		return "", nil, false
	}
	if _, local := g.lookupLocal(base.Name); local {
		return "", nil, false
	}
	info := g.numericArrays[base.Name]
	return base.Name, info, info != nil
}
