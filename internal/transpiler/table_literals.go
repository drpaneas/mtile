package transpiler

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// After the ordinary fresh allocation and checked borrow, every planned slot
// is in-bounds and empty. No intervening value can collect or expose the table,
// and every value below is non-nil, so no preceding insertion can be skipped.
// This stronger proof permits constant-offset stores without repeating the
// capacity, occupancy and hash-count guards for each field.
func (g *generator) trustedTableLiteralValues(table *luaparser.TableExpr) bool {
	if g.opts.DisableTrustedTableLiteralWrites {
		return false
	}
	for _, field := range table.Fields {
		if !g.tableLiteralValueNonNil(field.Value) {
			return false
		}
	}
	return true
}

func (g *generator) tableLiteralValueNonNil(expr luaparser.Expr) bool {
	if g.exprNumber(expr) {
		return true
	}
	switch x := unparenExpr(expr).(type) {
	case *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
		return true
	case *luaparser.UnaryExpr:
		return x.Op == luaparser.OpNot
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess, luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq:
			return true
		}
	}
	return false
}

// A fresh constructor cannot be observed by its values before assignment.
// Borrow only for complete unique-static-key layouts whose value evaluation
// cannot allocate/collect; otherwise retain the checked per-field path.
// Keeping the existing root push before borrowing preserves root-limit/OOM
// behavior even though this particular constructor cannot collect afterward.
func (g *generator) borrowableTableLiteral(table *luaparser.TableExpr, plans map[int]uint32) bool {
	if g.opts.DisableTableLiteralViews || len(plans) < 2 || len(plans) != len(table.Fields) {
		return false
	}
	for _, field := range table.Fields {
		if !g.exprNoAlloc(field.Value) {
			return false
		}
	}
	return true
}

// staticTableLiteralPlans preserves the ordinary insertion layout. Runtime
// checks guard possibly-nil values: after an absent field, the hash count is
// below every subsequent planned count, so the remaining stores use TableSet.
// Dynamic/duplicate keys stop planning because they could close that count
// gap or change probe placement. Positional fields cannot change occupancy.
func (g *generator) staticTableLiteralPlans(table *luaparser.TableExpr, arrayCap, hashCap int) map[int]uint32 {
	if g.opts.DisableStaticTableLiterals {
		return nil
	}
	plans := make(map[int]uint32)
	occupied := make([]bool, hashCap)
	seen := make(map[string]bool)
	count := 0
	for index, field := range table.Fields {
		if field.Array {
			continue
		}
		key, ok := unparenExpr(field.Key).(*luaparser.StringExpr)
		if !ok || seen[key.Value] {
			break
		}
		seen[key.Value] = true
		// Match runtime.hashBytes and hashBucket exactly, including non-power-
		// of-two capacities. Runtime parity tests protect this shared contract.
		hash := uint32(2166136261)
		for i := 0; i < len(key.Value); i++ {
			hash ^= uint32(key.Value[i])
			hash *= 16777619
		}
		slot := int(hash % uint32(hashCap))
		for occupied[slot] {
			slot = (slot + 1) % hashCap
		}
		occupied[slot] = true
		plans[index] = uint32(arrayCap) | uint32(hashCap)<<9 | uint32(slot)<<17 | uint32(count)<<24
		count++
	}
	return plans
}

type generatedTableImage struct {
	name string
	data []byte
}

type tableLiteralImagePlan struct {
	name     string
	dynamic  map[int]uint32
	arrayCap int
	hashCap  int
}

func (g *generator) staticID(s string) (uint16, bool) {
	for i, value := range g.statics {
		if value == s {
			return uint16(i), true
		}
	}
	return 0, false
}

func (g *generator) tableLiteralConstantValue(expr luaparser.Expr) (runtimeapi.Value, bool) {
	if number, ok := staticNumberLiteral(expr); ok {
		return runtimeapi.NumVal(runtimeapi.Number(fixedNumberLiteral(number))), true
	}
	switch x := unparenExpr(expr).(type) {
	case *luaparser.BoolExpr:
		return runtimeapi.BoolVal(x.Value), true
	case *luaparser.StringExpr:
		id, ok := g.staticID(x.Value)
		if !ok {
			return runtimeapi.Value{}, false
		}
		return runtimeapi.Value{Kind: runtimeapi.KindStaticString, SRef: id}, true
	}
	return runtimeapi.Value{}, false
}

// ROM images keep the ordinary hash layout. Constant fields are already stored;
// dynamic non-nil fields keep their keys occupied so overlay stores do not
// re-hash or bump the count. Reject the image if any key is not interned.
func (g *generator) imageableTableLiteral(table *luaparser.TableExpr, plans map[int]uint32) bool {
	if g.opts.DisableTableLiteralViews || len(plans) < 2 || len(plans) != len(table.Fields) {
		return false
	}
	for _, field := range table.Fields {
		if _, nested := unparenExpr(field.Value).(*luaparser.TableExpr); nested {
			continue
		}
		if !g.exprNoAlloc(field.Value) {
			return false
		}
	}
	return true
}

func (g *generator) tableLiteralNeedsLiveRoot(table *luaparser.TableExpr) bool {
	for _, field := range table.Fields {
		if !g.exprNoAlloc(field.Value) {
			return true
		}
	}
	return false
}

func (g *generator) tableLiteralImage(table *luaparser.TableExpr, plans map[int]uint32, arrayCap, hashCap int) tableLiteralImagePlan {
	if g.opts.DisableTableLiteralImages || !g.trustedTableLiteralValues(table) ||
		!g.imageableTableLiteral(table, plans) {
		return tableLiteralImagePlan{}
	}
	fields := make([]runtimeapi.TableLiteralImageField, 0, len(table.Fields))
	dynamic := make(map[int]uint32)
	for index, field := range table.Fields {
		plan, ok := plans[index]
		if !ok {
			return tableLiteralImagePlan{}
		}
		key, ok := unparenExpr(field.Key).(*luaparser.StringExpr)
		if !ok {
			return tableLiteralImagePlan{}
		}
		keyID, ok := g.staticID(key.Value)
		if !ok {
			return tableLiteralImagePlan{}
		}
		slot := int((plan >> 17) & 127)
		entry := runtimeapi.TableLiteralImageField{Slot: slot, KeyID: keyID}
		if value, constant := g.tableLiteralConstantValue(field.Value); constant {
			entry.Value, entry.HasValue = value, true
		} else {
			dynamic[index] = plan
		}
		fields = append(fields, entry)
	}
	image := runtimeapi.BuildTableLiteralImage(arrayCap, hashCap, fields)
	if len(image) == 0 {
		return tableLiteralImagePlan{}
	}
	return tableLiteralImagePlan{name: g.internTableImage(image), dynamic: dynamic, arrayCap: arrayCap, hashCap: hashCap}
}

func (g *generator) internTableImage(data []byte) string {
	for _, image := range g.tableImages {
		if bytes.Equal(image.data, data) {
			return image.name
		}
	}
	name := fmt.Sprintf("gluTableImg%d", len(g.tableImages))
	copied := append([]byte(nil), data...)
	g.tableImages = append(g.tableImages, generatedTableImage{name: name, data: copied})
	return name
}

func (g *generator) renderTableImages(b *sourceBuilder) {
	for _, image := range g.tableImages {
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var %s = [%d]byte{%s}", image.name, len(image.data), byteLiteral(image.data, len(image.data))))
	}
	if len(g.tableImages) != 0 {
		b.line("")
	}
}

func (g *generator) emitEmptyTableConstructor(arrayCap, hashCap int) string {
	tbl := g.next("tbl")
	errName := g.next("tblErr")
	g.pending = append(g.pending, "var "+tbl+" rt.Value")
	g.pending = append(g.pending, "var "+errName+" rt.HeapError")
	g.pending = append(g.pending, tbl+", "+errName+" = c.NewTableWithCaps("+strconv.Itoa(arrayCap)+", "+strconv.Itoa(hashCap)+")")
	g.pending = append(g.pending, "if "+errName+" != rt.HeapOK { "+tbl+" = rt.NilVal() }")
	return tbl
}

func (g *generator) emitTrustedImageConstructor(table *luaparser.TableExpr, image tableLiteralImagePlan) string {
	tbl := g.next("tbl")
	errName := g.next("tblErr")
	g.pending = append(g.pending, "var "+tbl+" rt.Value")
	g.pending = append(g.pending, "var "+errName+" rt.HeapError")
	g.pending = append(g.pending, tbl+", "+errName+" = c.NewTableFromImage("+image.name+"[:])")
	if len(image.dynamic) == 0 {
		g.pending = append(g.pending, "if "+errName+" != rt.HeapOK { "+tbl+" = rt.NilVal() }")
		return tbl
	}
	view := g.next("tblView")
	g.pending = append(g.pending, "if "+errName+" != rt.HeapOK { "+tbl+" = rt.NilVal() } else {")
	g.pending = append(g.pending, "\t"+view+" := c.BorrowTableLiteral("+tbl+")")
	oldRoots := g.extraRoots
	rootSlots := g.activeRootSlots
	g.activeRootSlots++
	for index, field := range table.Fields {
		plan, overlay := image.dynamic[index]
		if !overlay {
			continue
		}
		inner, _ := g.capturePending(func() string {
			key, _ := unparenExpr(field.Key).(*luaparser.StringExpr)
			keyID, _ := g.staticID(key.Value)
			if g.exprNumber(field.Value) {
				value := g.emitNumberExpr(field.Value)
				g.pending = append(g.pending, "_ = "+view+".StorePlannedNumber("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+")")
			} else {
				value := g.emitExpr(field.Value)
				g.pending = append(g.pending, "_ = "+view+".StorePlanned("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+")")
			}
			return ""
		})
		g.indentPending(inner)
	}
	g.extraRoots = oldRoots
	g.activeRootSlots = rootSlots
	g.pending = append(g.pending, "}")
	return tbl
}
