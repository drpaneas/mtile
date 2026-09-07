package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

// Preserve base/key evaluation before selecting the already-proven warm slot.
// The cold path must retain the original Values: reconstructing KindTable from
// a Ref would turn nil/non-table inputs into potentially live table handles.
// Pending statements keep this branch in the normal expression evaluation
// order without adding an immediately invoked closure or evaluating twice.
func (g *generator) emitStableNumberRead(field *luaparser.FieldExpr) string {
	baseExpr := g.emitExpr(field.Base)
	keyExpr, cache := g.staticExpr(field.Name), g.fieldCacheRef(field)
	if g.opts.DisableStableReadSpecialization {
		return "c.TableGetStableNumber(" + baseExpr + ", " + keyExpr + ", " + cache + ")"
	}
	base, key := g.next("fieldBase"), g.next("fieldKey")
	offset, result := g.next("fieldOffset"), g.next("fieldNumber")
	g.pending = append(g.pending, base+" := "+baseExpr)
	g.pending = append(g.pending, key+" := "+keyExpr)
	g.pending = append(g.pending, offset+" := *("+cache+")")
	g.pending = append(g.pending, "var "+result+" rt.Number")
	g.pending = append(g.pending, "if "+offset+" != 0 && "+base+".Kind == rt.KindTable {")
	g.pending = append(g.pending, "\t"+result+" = c.TableGetCachedNumber("+base+".Ref, "+offset+")")
	g.pending = append(g.pending, "} else {")
	g.pending = append(g.pending, "\t"+result+" = c.TableGetStableNumber("+base+", "+key+", "+cache+")")
	g.pending = append(g.pending, "}")
	return result
}
