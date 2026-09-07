package transpiler

import "strings"

// All source operands have already been evaluated, with ordinary GC protection
// during RHS calls. Test the stable offset before constructing the variadic
// roots slice: the warm overwrite cannot allocate or collect. A guard only
// inside gluTableSetStable is too late because Go prepares its arguments first.
func (g *generator) emitStableWrite(b *sourceBuilder, indent int, numeric bool, base, key, value, cache, roots string) {
	pad := strings.Repeat("\t", indent)
	method := "TableSetStable"
	if numeric {
		method += "Number"
	}
	args := base + ", " + key + ", " + value + ", " + cache
	cold := "_ = glu" + method + "(c, " + args + roots + ")"
	if g.opts.DisableStableWriteSpecialization {
		b.line(pad + cold)
		return
	}
	warm := "TableSetCached"
	if numeric {
		warm += "Number"
	}
	offset := g.next("fieldOffset")
	b.line(pad + offset + " := *(" + cache + ")")
	b.line(pad + "if " + offset + " != 0 && " + base + ".Kind == rt.KindTable {")
	b.line(pad + "\tc." + warm + "(" + base + ".Ref, " + offset + ", " + value + ")")
	b.line(pad + "} else {")
	b.line(pad + "\t" + cold)
	b.line(pad + "}")
}
