package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

// emitConstantNumberDivision runs only on the proven-number expression path.
// Boxed operands retain Value.Div's type checks. A literal right operand has
// no side effects, and emitting the left expression here evaluates it once.
func (g *generator) emitConstantNumberDivision(x *luaparser.BinaryExpr) (string, bool) {
	if g.opts.DisableConstantDivision || x.Op != luaparser.OpDivide {
		return "", false
	}
	literal, ok := staticNumberLiteral(x.Right)
	if !ok {
		return "", false
	}
	divisor := fixedNumberLiteral(literal)
	if divisor != 98304 && divisor != -98304 {
		return "", false
	}
	call := "gluNumberDivThreeHalves(" + g.emitNumberExpr(x.Left) + ")"
	if divisor < 0 {
		call = "-(" + call + ")"
	}
	return call, true
}

// For raw n, n / 1.5 is trunc(2*n/3). Let m=abs(n), 0<=m<=2^31.
// M=0xaaaaaaab=(2*2^32+1)/3, so m*M/2^32 = 2*m/3 + m/(3*2^32).
// The added term is <=1/6; the fractional part of 2*m/3 is 0, 1/3 or
// 2/3. Thus it cannot cross the next integer: the high product word is
// exactly floor(2*m/3). Restoring n's sign gives truncation toward zero.
// Unsigned negation handles INT_MIN without overflowing a signed temporary.
// ARM7TDMI lowers the 32x32 product to UMULL rather than a 64-bit division.
func renderConstantDivisionHelpers(b *sourceBuilder) {
	b.line("func gluNumberDivThreeHalves(n rt.Number) rt.Number {")
	b.line("\tmagnitude := uint32(n)")
	b.line("\tif n < 0 { magnitude = 0 - magnitude }")
	b.line("\tquotient := rt.Number(uint32((uint64(magnitude) * 0xaaaaaaab) >> 32))")
	b.line("\tif n < 0 { return -quotient }")
	b.line("\treturn quotient")
	b.line("}")
}
