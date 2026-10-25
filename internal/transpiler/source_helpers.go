package transpiler

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func (g *generator) staticExpr(s string) string {
	if _, ok := g.staticGo[s]; !ok {
		g.staticValue(s)
	}
	if g.current == nil {
		return "c.InternStatic(" + strconv.Quote(s) + ")"
	}
	return "g." + g.staticGo[s]
}
func (g *generator) fieldCacheRef(e *luaparser.FieldExpr) string {
	if g.opts.DisableStableTableFields {
		return ""
	}
	if _, _, ok := g.numericRecordField(e); ok {
		return ""
	}
	id, ok := g.fieldCaches[e]
	if !ok {
		g.setErr(fmt.Errorf("transpiler: missing stable field cache at %s", e.Pos()))
		return ""
	}
	return fmt.Sprintf("&g._gluField%d", id)
}
func (g *generator) lookup(name string) string {
	if goVar, ok := g.lookupLocal(name); ok {
		return goVar
	}
	if constant := g.numericConstants[name]; constant != nil {
		return fmt.Sprintf("rt.Number(%d)", constant.value)
	}
	if v, ok := g.globalGo[name]; ok {
		return "g." + v
	}
	return "rt.NilVal()"
}
func (g *generator) next(prefix string) string {
	g.seq++
	return fmt.Sprintf("glu_%s_%d", prefix, g.seq)
}
func (g *generator) lastName(prefix string) string { return fmt.Sprintf("glu_%s_%d", prefix, g.seq) }
func (g *generator) setErr(err error) {
	if g.err == nil {
		g.err = err
	}
}

type sourceBuilder struct {
	buf bytes.Buffer
}

func (b *sourceBuilder) line(s string)  { b.buf.WriteString(s); b.buf.WriteByte('\n') }
func (b *sourceBuilder) String() string { return b.buf.String() }

func goIdent(s string) string {
	if s == "" {
		return "glu_"
	}
	var b strings.Builder
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "glu_"
	}
	switch out {
	case "break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var":
		out = "glu_" + out
	}
	return out
}
func isGoIdent(s string) bool { return goIdent(s) == s }

// capitalize upper-cases the first byte of an ASCII builtin name (e.g. "sin"
// -> "Sin") to match the corresponding exported rt.Number method name.
// strings.Title is deprecated (and Unicode-aware in ways this call site does
// not need) since every caller passes a fixed, lowercase ASCII identifier.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

func byteLiteral(data []byte, n int) string {
	if len(data) == 0 {
		data = make([]byte, n)
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		v := byte(0)
		if i < len(data) {
			v = data[i]
		}
		parts[i] = fmt.Sprintf("0x%02x", v)
	}
	return strings.Join(parts, ",")
}

func u16Literal(data []uint16, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		v := uint16(0)
		if i < len(data) {
			v = data[i]
		}
		parts[i] = fmt.Sprintf("0x%04x", v)
	}
	return strings.Join(parts, ",")
}

func poseRangeLiteral(data []runtimeapi.PoseRange) string {
	parts := make([]string, len(data))
	for i, r := range data {
		parts[i] = fmt.Sprintf("{Start: %d, Count: %d, FlipOffsetX: %d, FlipOffsetY: %d}", r.Start, r.Count, r.FlipOffsetX, r.FlipOffsetY)
	}
	return strings.Join(parts, ",")
}

func posePartLiteral(data []runtimeapi.PosePart) string {
	parts := make([]string, len(data))
	for i, p := range data {
		parts[i] = fmt.Sprintf("{Tile: %d, X: %d, Y: %d, Flags: %d}", p.Tile, p.X, p.Y, p.Flags)
	}
	return strings.Join(parts, ",")
}

func u32Literal(data []uint32, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		v := uint32(0)
		if i < len(data) {
			v = data[i]
		}
		parts[i] = fmt.Sprintf("0x%08x", v)
	}
	return strings.Join(parts, ",")
}
