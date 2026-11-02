package transpiler

import (
	"encoding/binary"
	"fmt"
	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	"math"
)

func (g *generator) directAudioEnabled() bool {
	return g.opts.DirectAudio != nil && (chunkCallsBuiltin(g.chunk, "pcm_music") || chunkCallsBuiltin(g.chunk, "sample"))
}

func (g *generator) validateDirectAudioCalls() error {
	if g.opts.DirectAudio != nil {
		if err := assetsync.ValidateDirectAudio(g.opts.DirectAudio.Manifest, g.opts.DirectAudio.Bank); err != nil {
			return fmt.Errorf("transpiler: imported audio: %w", err)
		}
	}
	var first error
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, s := range fn.Body.Body {
				visit(s)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if first != nil {
				return
			}
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if !ok || (name != "sample" && name != "pcm_music") {
				return
			}
			if g.opts.DirectAudio == nil {
				if name == "pcm_music" {
					first = fmt.Errorf("transpiler: %s: pcm_music() requires imported audio; run mtile import-audio", call.Pos())
				}
				return
			}
			if len(call.Args) == 0 {
				return
			}
			items := g.opts.DirectAudio.Manifest.Music
			if name == "sample" {
				items = g.opts.DirectAudio.Manifest.Samples
			}
			if id, literal := staticNumberLiteral(call.Args[0]); literal && (id != math.Trunc(id) || id < -1 || id >= float64(len(items)) || id >= 0 && items[int(id)].Deleted) {
				first = fmt.Errorf("transpiler: %s: %s ID %g is not present in the imported audio manifest", call.Pos(), name, id)
				return
			}
			if name == "sample" && len(call.Args) > 1 {
				if volume, literal := staticNumberLiteral(call.Args[1]); literal && (volume < 0 || volume > 1) {
					first = fmt.Errorf("transpiler: %s: sample volume must be from 0 through 1", call.Pos())
				}
			}
			if name == "sample" && len(call.Args) > 2 {
				if priority, literal := staticNumberLiteral(call.Args[2]); literal && (priority < 0 || priority > 255 || priority != math.Trunc(priority)) {
					first = fmt.Errorf("transpiler: %s: sample priority must be an integer from 0 through 255", call.Pos())
				}
			}
		})
	}
	for _, stmt := range g.chunk.Body {
		visit(stmt)
	}
	return first
}

func (g *generator) renderDirectAudioStorage(b *sourceBuilder) {
	bank := g.opts.DirectAudio.Bank
	b.line("//go:section .rodata\nvar pcmBankROM = [...]uint32{")
	for offset := 0; offset < len(bank); offset += 4 {
		var word [4]byte
		copy(word[:], bank[offset:min(offset+4, len(bank))])
		b.line(fmt.Sprintf("0x%08x,", binary.LittleEndian.Uint32(word[:])))
	}
	b.line("}")
	for _, group := range []struct {
		name  string
		items []assetsync.DirectAudioItem
	}{{"Music", g.opts.DirectAudio.Manifest.Music}, {"Sample", g.opts.DirectAudio.Manifest.Samples}} {
		b.line("//go:section .rodata\nvar pcm" + group.name + "IDs = [...]uint16{")
		for _, item := range group.items {
			b.line(fmt.Sprintf("%d,", item.BankID))
		}
		b.line("}")
	}
	b.line("//go:section .ewram\nvar pcmAudioController audio.PCMController")
}
