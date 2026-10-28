package luaparser

// NodeBase is embedded by every AST node. Synthetic nodes created while
// lowering compound assignment retain the source position of the compound
// operator's statement and are marked with Synthetic on their statement.
type NodeBase struct {
	At   Position
	Last Position
}

func (n NodeBase) Pos() Position { return n.At }

func (n NodeBase) LastPos() Position {
	if n.Last.Line == 0 {
		return n.At
	}
	return n.Last
}

type Node interface {
	Pos() Position
}

type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

// Chunk is a parsed source file.
type Chunk struct {
	NodeBase
	SourceName string
	Body       []Stmt
}

// Block is used by control-flow and function statements.
type Block struct {
	NodeBase
	Body []Stmt
}

type NilExpr struct{ NodeBase }

func (*NilExpr) exprNode() {}

type BoolExpr struct {
	NodeBase
	Value bool
}

func (*BoolExpr) exprNode() {}

// AnalysisChoiceExpr is an unknown, side-effect-free condition constructed
// only by compiler analysis. The parser never produces it and code generation
// must never emit it. Separate instances do not correlate with game inputs.
type AnalysisChoiceExpr struct{ NodeBase }

func (*AnalysisChoiceExpr) exprNode() {}

type NumberExpr struct {
	NodeBase
	Raw     string
	Value   float64
	Integer bool
}

func (*NumberExpr) exprNode() {}

type StringExpr struct {
	NodeBase
	Raw   string
	Value string
}

func (*StringExpr) exprNode() {}

type VarExpr struct {
	NodeBase
	Name string
}

func (*VarExpr) exprNode() {}

// ParenExpr preserves a syntactic single-value boundary. Parentheses are
// transparent for ordinary evaluation, but Lua collapses a parenthesized
// multi-result call to its first result: `(pair())` is one value while a
// trailing bare `pair()` may expand in an expression list.
type ParenExpr struct {
	NodeBase
	Expr Expr
}

func (*ParenExpr) exprNode() {}

type UnaryExpr struct {
	NodeBase
	Op   Operator
	Expr Expr
}

func (*UnaryExpr) exprNode() {}

type BinaryExpr struct {
	NodeBase
	Left  Expr
	Op    Operator
	Right Expr
}

func (*BinaryExpr) exprNode() {}

type IndexExpr struct {
	NodeBase
	Base Expr
	Key  Expr
}

func (*IndexExpr) exprNode() {}

type FieldExpr struct {
	NodeBase
	Base Expr
	Name string
}

func (*FieldExpr) exprNode() {}

type CallExpr struct {
	NodeBase
	Callee Expr
	Args   []Expr
	Method string // non-empty for `receiver:method(args)` calls
}

func (*CallExpr) exprNode() {}

type TableField struct {
	NodeBase
	Key   Expr
	Value Expr
	Array bool // true for the implicit array part: { value }
}

type TableExpr struct {
	NodeBase
	Fields []*TableField
}

func (*TableExpr) exprNode() {}

// Operator is deliberately textual. It keeps Mtile operators visible to
// the later transpiler instead of mapping them to host operators.
type Operator string

const (
	OpOr            Operator = "or"
	OpAnd           Operator = "and"
	OpEqual         Operator = "=="
	OpNotEqual      Operator = "!="
	OpLess          Operator = "<"
	OpLessEq        Operator = "<="
	OpGreater       Operator = ">"
	OpGreaterEq     Operator = ">="
	OpBitOr         Operator = "|"
	OpBitXor        Operator = "^^"
	OpBitAnd        Operator = "&"
	OpShiftL        Operator = "<<"
	OpShiftR        Operator = ">>"
	OpShiftRLogical Operator = ">>>"
	OpRotateL       Operator = "<<>"
	OpRotateR       Operator = ">><"
	OpConcat        Operator = ".."
	OpAdd           Operator = "+"
	OpSubtract      Operator = "-"
	OpMultiply      Operator = "*"
	OpDivide        Operator = "/"
	OpIntegerDivide Operator = "\\"
	OpModulo        Operator = "%"
	OpPower         Operator = "^"
	OpNot           Operator = "not"
	OpNegate        Operator = "-"
	OpBitNot        Operator = "~"
	OpLength        Operator = "#"
)

type AssignStmt struct {
	NodeBase
	Targets        []Expr
	Values         []Expr
	Synthetic      bool
	CompoundTarget Expr // original l-value when compound lowering uses temporaries
}

func (*AssignStmt) stmtNode() {}

type LocalAssignStmt struct {
	NodeBase
	Names     []string
	Values    []Expr
	Synthetic bool
}

func (*LocalAssignStmt) stmtNode() {}

type CallStmt struct {
	NodeBase
	Call *CallExpr
}

func (*CallStmt) stmtNode() {}

type FunctionDeclStmt struct {
	NodeBase
	Name   []string
	Method string
	Params []string
	Vararg bool
	Body   *Block
	Local  bool
}

func (*FunctionDeclStmt) stmtNode() {}

type IfBranch struct {
	NodeBase
	Condition Expr
	Body      *Block
}

type IfStmt struct {
	NodeBase
	Condition Expr
	Then      *Block
	ElseIf    []*IfBranch
	Else      *Block
}

func (*IfStmt) stmtNode() {}

type WhileStmt struct {
	NodeBase
	Condition Expr
	Body      *Block
}

func (*WhileStmt) stmtNode() {}

type RepeatStmt struct {
	NodeBase
	Body      *Block
	Condition Expr
}

func (*RepeatStmt) stmtNode() {}

type DoStmt struct {
	NodeBase
	Body *Block
}

func (*DoStmt) stmtNode() {}

type NumericForStmt struct {
	NodeBase
	Name  string
	Init  Expr
	Limit Expr
	Step  Expr
	Body  *Block
}

func (*NumericForStmt) stmtNode() {}

type GenericForStmt struct {
	NodeBase
	Names  []string
	Values []Expr
	Body   *Block
}

func (*GenericForStmt) stmtNode() {}

type ReturnStmt struct {
	NodeBase
	Values []Expr
}

func (*ReturnStmt) stmtNode() {}

type BreakStmt struct{ NodeBase }

func (*BreakStmt) stmtNode() {}
