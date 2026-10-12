package transpiler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// Literal clip names are immutable asset keys, so resolve them once at build
// time instead of interning and searching the same string on every draw.
func staticAnimationID(args []luaparser.Expr, opts Options) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	name, literal := unparenExpr(args[0]).(*luaparser.StringExpr)
	if !literal {
		return 0, false
	}
	for id, clip := range opts.AnimationClips {
		if clip.Name == name.Value {
			return id, true
		}
	}
	return 0, false
}

func (g *generator) animationMethod(args []luaparser.Expr) (method, id string) {
	if index, literal := staticAnimationID(args, g.opts); literal {
		return "AnimID", strconv.Itoa(index)
	}
	// A whole-program no-allocation proof means a dynamic name can only be a
	// static string (or a non-string no-op). Avoid retaining the Lua heap just
	// because the generic decoder also knows how to read allocated strings.
	if !g.heapEnabled {
		return "AnimStatic", ""
	}
	return "Anim", ""
}

func validateAnimationCall(call *luaparser.CallExpr, opts Options) error {
	if len(call.Args) == 0 {
		return nil
	}
	name, literal := unparenExpr(call.Args[0]).(*luaparser.StringExpr)
	if !literal {
		return nil
	}
	for _, clip := range opts.AnimationClips {
		if clip.Name == name.Value {
			return nil
		}
	}
	return fmt.Errorf("transpiler: %s: [ANIM_UNKNOWN_CLIP] anim(%q) has no registered clip in assets/animation.json", call.Pos(), name.Value)
}

func validateMode0AnimCall(call *luaparser.CallExpr, opts Options) (ids []int, unique, parts int, err error) {
	if len(call.Args) < 3 || len(call.Args) > 6 {
		return nil, 0, 0, mode0CallError(call, reasonMode0PsprShape, "Mode 0 anim() requires 3 to 6 explicit arguments")
	}
	if err := validateAnimationCall(call, opts); err != nil {
		return nil, 0, 0, err
	}
	name, literal := unparenExpr(call.Args[0]).(*luaparser.StringExpr)
	seen := make(map[int]bool)
	for _, clip := range opts.AnimationClips {
		if literal && clip.Name != name.Value {
			continue
		}
		end := uint64(clip.Start) + uint64(clip.Count)
		if end > uint64(len(opts.AnimationFrames)) {
			return nil, 0, 0, fmt.Errorf("transpiler: invalid animation frame table for %q", clip.Name)
		}
		for _, id := range opts.AnimationFrames[uint64(clip.Start):end] {
			if seen[int(id)] {
				continue
			}
			seen[int(id)] = true
			u, p, ok := mode0PoseUniqueAndParts(opts, int(id))
			if !ok {
				continue
			}
			ids = append(ids, int(id))
			unique = max(unique, u)
			parts = max(parts, p)
		}
	}
	ids = expandedPoseIDs(opts, ids)
	for _, id := range ids {
		u, p, ok := mode0PoseUniqueAndParts(opts, id)
		if ok {
			unique = max(unique, u)
			parts = max(parts, p)
		}
	}
	return ids, unique, parts, nil
}

func (g *generator) renderAnimationTables(b *sourceBuilder) {
	var clips strings.Builder
	for _, clip := range g.opts.AnimationClips {
		fmt.Fprintf(&clips, "{Name:%q,Start:%d,Count:%d,FPS:%d,Loop:%t,FlipOffsetX:%d,FlipOffsetY:%d},", clip.Name, clip.Start, clip.Count, clip.FPS, clip.Loop, clip.FlipOffsetX, clip.FlipOffsetY)
	}
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var animationClipROM = [%d]rt.AnimationClip{%s}", len(g.opts.AnimationClips), clips.String()))
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var animationFrameROM = [%d]uint16{%s}", len(g.opts.AnimationFrames), u16Literal(g.opts.AnimationFrames, len(g.opts.AnimationFrames))))
}
