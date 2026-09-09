package transpiler

import (
	"math"
	"math/bits"
	"strconv"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func maybeFlr(expr string, floor bool) string {
	if !floor {
		return expr
	}
	return "(" + expr + ").Flr()"
}

func (g *generator) constantDivisorRaw(expr luaparser.Expr) (int32, bool) {
	if number, ok := staticNumberLiteral(expr); ok {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return 0, false
		}
		raw := fixedNumberLiteral(number)
		return raw, raw != 0
	}
	variable, ok := unparenExpr(expr).(*luaparser.VarExpr)
	if !ok {
		return 0, false
	}
	info := g.numericConstants[variable.Name]
	if info == nil || info.value == 0 {
		return 0, false
	}
	return info.value, true
}

func wideningNumberDiv(left string, raw int32) string {
	return "rt.Number((int64(" + left + ") << 16) / " + strconv.FormatInt(int64(raw), 10) + ")"
}

func (g *generator) fractionalNumberDiv(left string, raw int32, boxed bool) (string, bool) {
	divisor := int64(raw)
	if divisor < 0 {
		divisor = -divisor
	}
	if g.opts.DisableConstantDivision && divisor == 98304 {
		return "", false
	}
	helper := "gluNumberDivFraction"
	if boxed {
		helper = "gluValueDivFraction"
	}
	args := left + ", " + strconv.FormatInt(divisor, 10)
	if divisor > 65536 {
		multiplier := ((uint64(1) << 48) + uint64(divisor) - 1) / uint64(divisor)
		helper = "gluNumberDivReciprocal"
		if boxed {
			helper = "gluValueDivReciprocal"
		}
		args = left + ", " + strconv.FormatUint(multiplier, 10) + ", " + strconv.FormatInt(divisor, 10)
	}
	call := helper + "(" + args + ")"
	if raw < 0 {
		if boxed {
			call = "gluValueNegNumber(" + call + ")"
		} else {
			call = "-(" + call + ")"
		}
	}
	return call, true
}

// Direct arithmetic must operate on a variable so Go applies Number's
// wrapping semantics instead of rejecting overflowing constant expressions.
// Keep the divisor literal so LLVM can still specialize the operation.
func (g *generator) materializeNumberDividend(left string) string {
	tmp := g.next("num")
	g.pending = append(g.pending, tmp+" := "+left)
	return tmp
}

// emitConstantNumberDivision runs only on the proven-number expression path.
// Boxed operands retain Value.Div's type checks. A literal right operand has
// no side effects, and emitting the left expression here evaluates it once.
func (g *generator) emitConstantNumberDivision(x *luaparser.BinaryExpr) (string, bool) {
	if x.Op != luaparser.OpDivide && x.Op != luaparser.OpIntegerDivide {
		return "", false
	}
	divisor, ok := g.constantDivisorRaw(x.Right)
	if !ok {
		return "", false
	}
	floor := x.Op == luaparser.OpIntegerDivide
	if !g.opts.DisableConstantDivision && (divisor == 98304 || divisor == -98304) {
		call := "gluNumberDivThreeHalves(" + g.emitNumberExpr(x.Left) + ")"
		if divisor < 0 {
			call = "-(" + call + ")"
		}
		return maybeFlr(call, floor), true
	}
	if call, ok := g.emitPow2NumberDiv(x.Left, divisor, floor); ok {
		return call, true
	}
	if k, ok := integerDivisor(divisor); ok {
		left := g.emitNumberExpr(x.Left)
		var call string
		switch k {
		case 1:
			call = g.materializeNumberDividend(left)
		case -1:
			call = "-(" + g.materializeNumberDividend(left) + ")"
		default:
			call = "rt.Number(int32(" + left + ") / " + strconv.FormatInt(int64(k), 10) + ")"
		}
		return maybeFlr(call, floor), true
	}
	left := g.emitNumberExpr(x.Left)
	if call, ok := g.fractionalNumberDiv(left, divisor, false); ok {
		return maybeFlr(call, floor), true
	}
	left = g.materializeNumberDividend(left)
	return maybeFlr(wideningNumberDiv(left, divisor), floor), true
}

func (g *generator) emitPow2NumberDiv(left luaparser.Expr, divisor int32, floor bool) (string, bool) {
	if shift, neg, ok := integerPow2DivShift(divisor); ok {
		call := "gluNumberDivPow2(" + g.emitNumberExpr(left) + ", " + strconv.Itoa(shift) + ")"
		if neg {
			call = "-(" + call + ")"
		}
		return maybeFlr(call, floor), true
	}
	shift, neg, ok := fractionalPow2MulShift(divisor)
	if !ok {
		return "", false
	}
	dividend := g.materializeNumberDividend(g.emitNumberExpr(left))
	call := "(" + dividend + " << " + strconv.Itoa(shift) + ")"
	if neg {
		call = "-(" + call + ")"
	}
	return maybeFlr(call, floor), true
}

func (g *generator) emitConstantNumberModulo(x *luaparser.BinaryExpr) (string, bool) {
	if x.Op != luaparser.OpModulo {
		return "", false
	}
	raw, ok := g.constantDivisorRaw(x.Right)
	if !ok {
		return "", false
	}
	shift, ok := integerPow2ModShift(raw)
	if ok {
		return "gluNumberModPow2(" + g.emitNumberExpr(x.Left) + ", " + strconv.Itoa(shift) + ")", true
	}
	return "gluNumberModLit(" + g.emitNumberExpr(x.Left) + ", " + strconv.FormatInt(int64(raw), 10) + ")", true
}

func (g *generator) emitConstantValueArithmetic(x *luaparser.BinaryExpr) (string, bool) {
	if x.Op != luaparser.OpDivide && x.Op != luaparser.OpIntegerDivide && x.Op != luaparser.OpModulo {
		return "", false
	}
	divisor, ok := g.constantDivisorRaw(x.Right)
	if !ok {
		return "", false
	}
	if x.Op == luaparser.OpModulo {
		shift, ok := integerPow2ModShift(divisor)
		if ok {
			return "gluValueModPow2(" + g.emitExpr(x.Left) + ", " + strconv.Itoa(shift) + ")", true
		}
		return "gluValueModLit(" + g.emitExpr(x.Left) + ", " + strconv.FormatInt(int64(divisor), 10) + ")", true
	}
	floor := x.Op == luaparser.OpIntegerDivide
	if !g.opts.DisableConstantDivision && (divisor == 98304 || divisor == -98304) {
		call := "gluValueDivThreeHalves(" + g.emitExpr(x.Left) + ")"
		if divisor < 0 {
			call = "gluValueNegNumber(" + call + ")"
		}
		return maybeFlr(call, floor), true
	}
	if shift, neg, ok := integerPow2DivShift(divisor); ok {
		call := "gluValueDivPow2(" + g.emitExpr(x.Left) + ", " + strconv.Itoa(shift) + ")"
		if neg {
			call = "gluValueNegNumber(" + call + ")"
		}
		return maybeFlr(call, floor), true
	}
	if shift, neg, ok := fractionalPow2MulShift(divisor); ok {
		call := "gluValueMulPow2(" + g.emitExpr(x.Left) + ", " + strconv.Itoa(shift) + ")"
		if neg {
			call = "gluValueNegNumber(" + call + ")"
		}
		return maybeFlr(call, floor), true
	}
	left := g.emitExpr(x.Left)
	if k, ok := integerDivisor(divisor); ok {
		if floor {
			return "gluValueIDivInt(" + left + ", " + strconv.FormatInt(int64(k), 10) + ")", true
		}
		return "gluValueDivInt(" + left + ", " + strconv.FormatInt(int64(k), 10) + ")", true
	}
	if call, ok := g.fractionalNumberDiv(left, divisor, true); ok {
		return maybeFlr(call, floor), true
	}
	call := "gluValueDivLit(" + left + ", " + strconv.FormatInt(int64(divisor), 10) + ")"
	return maybeFlr(call, floor), true
}

// integerPow2DivShift reports the toward-zero shift for n / k when k is a
// positive or negative integer power of two. Number.Div is
// (int64(n)<<16)/int64(k<<16) = toward-zero n/k, so the shift is log2(|k|).
func integerPow2DivShift(raw int32) (shift int, neg bool, ok bool) {
	mag, neg, ok := integerPow2Magnitude(raw)
	if !ok {
		return 0, false, false
	}
	shift = bits.TrailingZeros32(mag) - 16
	if shift < 1 || shift > 30 {
		return 0, false, false
	}
	return shift, neg, true
}

// fractionalPow2MulShift reports the left shift for n / k when k is a
// positive or negative 16.16 power of two smaller than 1 (0.5, 0.25, ...).
func fractionalPow2MulShift(raw int32) (shift int, neg bool, ok bool) {
	if raw == 0 || raw == -raw {
		return 0, false, false
	}
	neg = raw < 0
	var mag uint32
	if neg {
		mag = uint32(-raw)
	} else {
		mag = uint32(raw)
	}
	if mag == 0 || mag&(mag-1) != 0 || mag >= 65536 {
		return 0, false, false
	}
	shift = 16 - bits.TrailingZeros32(mag)
	if shift < 1 || shift > 16 {
		return 0, false, false
	}
	return shift, neg, true
}

func integerDivisor(raw int32) (k int32, ok bool) {
	if raw == 0 || raw&0xFFFF != 0 {
		return 0, false
	}
	return raw >> 16, true
}

// integerPow2ModShift reports the arithmetic-shift count for Lua n % k when
// k is a positive integer power of two. Remainder follows the divisor sign,
// which for positive k is n - floor(n/k)*k, i.e. n - ((n>>s)<<s) on the
// 16.16 encoding whose magnitude is k<<16 = 2^s.
func integerPow2ModShift(raw int32) (shift int, ok bool) {
	if raw <= 0 {
		return 0, false
	}
	mag, _, ok := integerPow2Magnitude(raw)
	if !ok {
		return 0, false
	}
	return bits.TrailingZeros32(mag), true
}

func integerPow2Magnitude(raw int32) (mag uint32, neg bool, ok bool) {
	if raw == 0 {
		return 0, false, false
	}
	if raw == -raw {
		return 1 << 31, true, true
	}
	neg = raw < 0
	if neg {
		mag = uint32(-raw)
	} else {
		mag = uint32(raw)
	}
	if mag < 65536 || mag&0xFFFF != 0 || mag&(mag-1) != 0 {
		return 0, false, false
	}
	return mag, neg, true
}

func renderConstantDivisionHelpers(b *sourceBuilder) {
	// For 0 < d < 2^16, split m=abs(n) into m=q*d+r. Then
	// floor(m*2^16/d)=q*2^16+floor(r*2^16/d). Since r<d<2^16,
	// each divide fits uint32 and LLVM specializes both literal divisors.
	// The first shift deliberately wraps; only the low 32 result bits matter.
	b.line("func gluNumberDivFraction(n rt.Number, divisor uint32) rt.Number {")
	b.line("\tmagnitude := uint32(n)")
	b.line("\tif n < 0 { magnitude = 0 - magnitude }")
	b.line("\tquotient := magnitude / divisor")
	b.line("\tremainder := magnitude - quotient * divisor")
	b.line("\tresult := (quotient << 16) + ((remainder << 16) / divisor)")
	b.line("\tif n < 0 { result = 0 - result }")
	b.line("\treturn rt.Number(result)")
	b.line("}")
	b.line("func gluValueDivFraction(a rt.Value, divisor uint32) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(gluNumberDivFraction(a.Num, divisor))")
	b.line("}")
	// For d > 2^16, M=ceil(2^48/d) fits uint32. For m=abs(n)<=2^31,
	// m*M/2^32 differs from m*2^16/d by less than 1/2. Its floor is
	// therefore the exact unsigned quotient or one too large. Comparing the
	// reconstructed product and subtracting one makes every residue exact.
	b.line("func gluNumberDivReciprocal(n rt.Number, multiplier, divisor uint32) rt.Number {")
	b.line("\tmagnitude := uint32(n)")
	b.line("\tif n < 0 { magnitude = 0 - magnitude }")
	b.line("\tquotient := uint32((uint64(magnitude) * uint64(multiplier)) >> 32)")
	b.line("\tif uint64(quotient) * uint64(divisor) > uint64(magnitude) << 16 { quotient-- }")
	b.line("\tif n < 0 { quotient = 0 - quotient }")
	b.line("\treturn rt.Number(quotient)")
	b.line("}")
	b.line("func gluValueDivReciprocal(a rt.Value, multiplier, divisor uint32) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(gluNumberDivReciprocal(a.Num, multiplier, divisor))")
	b.line("}")
	// For raw n, n / 1.5 is trunc(2*n/3). Let m=abs(n), 0<=m<=2^31.
	// M=0xaaaaaaab=(2*2^32+1)/3, so m*M/2^32 = 2*m/3 + m/(3*2^32).
	// The added term is <=1/6; the fractional part of 2*m/3 is 0, 1/3 or
	// 2/3. Thus it cannot cross the next integer: the high product word is
	// exactly floor(2*m/3). Restoring n's sign gives truncation toward zero.
	// Unsigned negation handles INT_MIN without overflowing a signed temporary.
	// ARM7TDMI lowers the 32x32 product to UMULL rather than a 64-bit division.
	b.line("func gluNumberDivThreeHalves(n rt.Number) rt.Number {")
	b.line("\tmagnitude := uint32(n)")
	b.line("\tif n < 0 { magnitude = 0 - magnitude }")
	b.line("\tquotient := rt.Number(uint32((uint64(magnitude) * 0xaaaaaaab) >> 32))")
	b.line("\tif n < 0 { return -quotient }")
	b.line("\treturn quotient")
	b.line("}")
	// Toward-zero signed division by 2^shift. Negative values add 2^shift-1
	// before ASR so the result matches Go's n/k and Number.Div, not floor.
	b.line("func gluNumberDivPow2(n rt.Number, shift int) rt.Number {")
	b.line("\tif n >= 0 { return n >> shift }")
	b.line("\treturn (n + (1 << shift) - 1) >> shift")
	b.line("}")
	// Floor remainder for a positive power-of-two 16.16 modulus. Matches
	// gluNumberMod without a software divide: Lua % takes the divisor sign.
	b.line("func gluNumberModPow2(n rt.Number, shift int) rt.Number {")
	b.line("\treturn n - ((n >> shift) << shift)")
	b.line("}")
	b.line("func gluNumberModLit(n rt.Number, z int32) rt.Number {")
	b.line("\tif z == 0 { return 0 }")
	b.line("\tr := n % rt.Number(z)")
	b.line("\tif r != 0 && ((r < 0) != (z < 0)) { r += rt.Number(z) }")
	b.line("\treturn r")
	b.line("}")
	b.line("func gluValueDivThreeHalves(a rt.Value) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(gluNumberDivThreeHalves(a.Num))")
	b.line("}")
	b.line("func gluValueDivPow2(a rt.Value, shift int) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(gluNumberDivPow2(a.Num, shift))")
	b.line("}")
	b.line("func gluValueMulPow2(a rt.Value, shift int) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(a.Num << shift)")
	b.line("}")
	b.line("func gluValueDivInt(a rt.Value, k int32) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(rt.Number(int32(a.Num) / k))")
	b.line("}")
	b.line("func gluValueIDivInt(a rt.Value, k int32) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(rt.Number(int32(a.Num) / k).Flr())")
	b.line("}")
	b.line("func gluValueDivLit(a rt.Value, d int32) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(rt.Number((int64(a.Num) << 16) / int64(d)))")
	b.line("}")
	b.line("func gluValueModLit(a rt.Value, z int32) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(gluNumberModLit(a.Num, z))")
	b.line("}")
	b.line("func gluValueModPow2(a rt.Value, shift int) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(gluNumberModPow2(a.Num, shift))")
	b.line("}")
	b.line("func gluValueNegNumber(a rt.Value) rt.Value {")
	b.line("\tif a.Kind != rt.KindNumber { return rt.NilVal() }")
	b.line("\treturn rt.NumVal(-a.Num)")
	b.line("}")
}
