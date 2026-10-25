package transpiler

import (
	"fmt"
	"math"

	"github.com/drpaneas/mtile/internal/luaparser"
	"github.com/drpaneas/mtile/runtime/audio"
)

// validateStaticAudioCalls catches only IDs that are unambiguously numeric
// literals in a project that links audio assets. Dynamic expressions retain
// the runtime's bounded fail-closed behavior, and audio-less ports may keep
// compatibility calls without pulling in the audio runtime.
func (g *generator) validateStaticAudioCalls() error {
	if !g.nativeAudioEnabled && g.opts.NativeAudio == nil &&
		!chunkCallsBuiltin(g.chunk, "gba_note") && !chunkCallsBuiltin(g.chunk, "gba_stop") {
		return nil
	}
	var firstErr error
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if firstErr != nil {
			return
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if firstErr != nil {
				return
			}
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if !ok {
				return
			}
			argCount := g.exprListWidth(call.Args)
			expanded := argCount != len(call.Args)
			if g.opts.NativeAudio != nil {
				switch name {
				case "sample":
					firstErr = fmt.Errorf("transpiler: %s: sample() is not part of four-channel audio v2", call.Pos())
					return
				case "sfx":
					if argCount < 1 || argCount > 2 {
						firstErr = fmt.Errorf("transpiler: %s: audio v2 sfx() takes id and optional priority", call.Pos())
						return
					}
				case "music":
					if argCount != 1 {
						firstErr = fmt.Errorf("transpiler: %s: audio v2 music() takes exactly one id", call.Pos())
						return
					}
				case "gba_note":
					if argCount < 3 || argCount > 4 {
						firstErr = fmt.Errorf("transpiler: %s: gba_note() takes channel, note, sound id, and optional duration", call.Pos())
						return
					}
					if !expanded {
						g.validateStaticNativeNote(call, &firstErr)
					}
					return
				case "gba_stop":
					if argCount != 1 {
						firstErr = fmt.Errorf("transpiler: %s: gba_stop() takes one channel", call.Pos())
						return
					}
					if !expanded {
						if channel, literal := staticNumberLiteral(call.Args[0]); literal && (channel != math.Trunc(channel) || channel < 1 || channel > 4) {
							firstErr = fmt.Errorf("transpiler: %s: GBA channel must be an integer from 1 through 4", call.Pos())
						}
					}
					return
				}
			} else if name == "gba_note" || name == "gba_stop" {
				firstErr = fmt.Errorf("transpiler: %s: %s() requires four-channel audio v2 assets", call.Pos(), name)
				return
			}
			if (name != "sfx" && name != "music" && name != "sample") || argCount == 0 || expanded {
				return
			}
			value, literal := staticNumberLiteral(call.Args[0])
			if !literal {
				return
			}
			if value != math.Trunc(value) {
				firstErr = fmt.Errorf("transpiler: %s: %s ID must be an integer, got %g", call.Pos(), name, value)
				return
			}
			if value == -1 {
				return
			}
			if value < 0 || value > math.MaxInt32 || !g.hasAudioID(name, int(value)) {
				firstErr = fmt.Errorf("transpiler: %s: %s ID %g is not present in %s", call.Pos(), name, value, g.audioAssetPath(name))
			}
		})
	}
	for _, stmt := range g.chunk.Body {
		visitStmt(stmt)
	}
	return firstErr
}

func (g *generator) validateStaticNativeNote(call *luaparser.CallExpr, firstErr *error) {
	channelValue, channelStatic := staticNumberLiteral(call.Args[0])
	if channelStatic && (channelValue != math.Trunc(channelValue) || channelValue < 1 || channelValue > 4) {
		*firstErr = fmt.Errorf("transpiler: %s: GBA channel must be an integer from 1 through 4", call.Pos())
		return
	}
	noteValue, noteStatic := staticNumberLiteral(call.Args[1])
	if noteStatic && (noteValue != math.Trunc(noteValue) || noteValue < 0 || noteValue > 127) {
		*firstErr = fmt.Errorf("transpiler: %s: GBA note must be an integer from 0 through 127", call.Pos())
		return
	}
	soundValue, soundStatic := staticNumberLiteral(call.Args[2])
	if soundStatic && (soundValue != math.Trunc(soundValue) || soundValue < 0 || soundValue > 255) {
		*firstErr = fmt.Errorf("transpiler: %s: GBA sound ID must be an integer from 0 through 255", call.Pos())
		return
	}
	if soundStatic {
		var sound *audio.Sound
		for index := range g.opts.NativeAudio.Library.Sounds {
			candidate := &g.opts.NativeAudio.Library.Sounds[index]
			if int(candidate.ID) == int(soundValue) {
				sound = candidate
				break
			}
		}
		if sound == nil {
			*firstErr = fmt.Errorf("transpiler: %s: sound ID %g is not present in assets/audio/sounds.json", call.Pos(), soundValue)
			return
		}
		if channelStatic && int(sound.Kind)+1 != int(channelValue) {
			*firstErr = fmt.Errorf("transpiler: %s: sound ID %g belongs to CH%d, not CH%d", call.Pos(), soundValue, int(sound.Kind)+1, int(channelValue))
			return
		}
		if channelStatic && noteStatic && sound.Kind != audio.SoundCH4 {
			if _, ok := audio.NativeSoundPeriod(*sound, uint8(noteValue)); !ok {
				*firstErr = fmt.Errorf("transpiler: %s: note %g is outside CH%d hardware range", call.Pos(), noteValue, int(channelValue))
				return
			}
		}
	}
	if len(call.Args) == 4 {
		if duration, literal := staticNumberLiteral(call.Args[3]); literal && (duration != math.Trunc(duration) || duration < 0 || duration > 65535) {
			*firstErr = fmt.Errorf("transpiler: %s: duration must be an integer from 0 through 65535", call.Pos())
		}
	}
}

func staticNumberLiteral(expr luaparser.Expr) (float64, bool) {
	switch value := expr.(type) {
	case *luaparser.ParenExpr:
		return staticNumberLiteral(value.Expr)
	case *luaparser.NumberExpr:
		return value.Value, true
	case *luaparser.UnaryExpr:
		if value.Op != luaparser.OpNegate {
			return 0, false
		}
		if number, ok := staticNumberLiteral(value.Expr); ok {
			return -number, true
		}
	}
	return 0, false
}

func unparenExpr(expr luaparser.Expr) luaparser.Expr {
	for {
		paren, ok := expr.(*luaparser.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.Expr
	}
}

func (g *generator) hasAudioID(name string, id int) bool {
	if g.opts.NativeAudio != nil {
		switch name {
		case "sfx":
			for _, clip := range g.opts.NativeAudio.Library.Clips {
				if int(clip.ID) == id {
					return true
				}
			}
		case "music":
			for _, song := range g.opts.NativeAudio.Library.Songs {
				if int(song.ID) == id {
					return true
				}
			}
		}
		return false
	}
	return false
}

func (g *generator) audioAssetPath(name string) string {
	if g.opts.NativeAudio != nil {
		if name == "music" || name == "sfx" {
			return "assets/audio/music.json"
		}
		return "four-channel audio v2"
	}
	return "four-channel audio v2"
}

// chunkCallsBuiltin reports whether the source directly calls a builtin.
// Mtile has no first-class functions or closures, so every sset mutation is
// syntactically visible in the AST and immutable-sheet selection is exact.
func chunkCallsBuiltin(chunk *luaparser.Chunk, name string) bool {
	found := false
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(e luaparser.Expr) {
			call, ok := e.(*luaparser.CallExpr)
			if !ok {
				return
			}
			if key, ok := callKey(call); ok && key == name {
				found = true
			}
		})
	}
	for _, stmt := range chunk.Body {
		visitStmt(stmt)
	}
	return found
}

func functionCallsBuiltin(fn *functionInfo, name string) bool {
	if fn == nil || fn.decl == nil {
		return false
	}
	found := false
	for _, stmt := range fn.decl.Body.Body {
		collectStmtExprs(stmt, func(e luaparser.Expr) {
			call, ok := e.(*luaparser.CallExpr)
			if !ok {
				return
			}
			if key, ok := callKey(call); ok && key == name {
				found = true
			}
		})
	}
	return found
}
