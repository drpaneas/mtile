package transpiler

import (
	"fmt"

	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// Allocate only the pose portion of OBJ VRAM. Ordinary sheet tiles preceding
// these windows do not need a pose staging buffer.
func (g *generator) poseOBJStageWindow() (base, tiles int) {
	if g.poseOBJResident != nil {
		return 0, 0
	}
	base, end := mode0PsprWindowTiles, 0
	for _, allocation := range g.psprPlan {
		if allocation.stride == 0 {
			continue
		}
		base = min(base, allocation.window)
		end = max(end, allocation.window+allocation.count*allocation.stride)
	}
	if end <= base {
		return 0, 0
	}
	return base, end - base
}

// The compiler and hardware renderer consume the same rectangle layout. The
// original pose recipe remains intact for the software renderer.
func (g *generator) preparePoseOBJPlans() {
	if len(g.psprPlan) == 0 {
		return
	}
	g.poseOBJIndex = make([]runtimeapi.PoseOBJRange, len(g.opts.PoseIndex))
	for id, pose := range g.opts.PoseIndex {
		start, end := int(pose.Start), int(pose.Start)+int(pose.Count)
		if start < 0 || end < start || end > len(g.opts.PoseParts) {
			continue
		}
		objects, tiles := runtimeapi.BuildPoseOBJPlan(g.opts.PoseParts[start:end])
		g.poseOBJIndex[id] = runtimeapi.PoseOBJRange{
			Start: uint32(len(g.poseOBJParts)), Count: uint32(len(objects)),
			TileStart: uint32(len(g.poseOBJTiles)), TileCount: uint32(len(tiles)),
		}
		g.poseOBJParts = append(g.poseOBJParts, objects...)
		g.poseOBJTiles = append(g.poseOBJTiles, tiles...)
	}
}

func (g *generator) renderPoseOBJPlans(b *sourceBuilder) {
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var poseOBJIndexROM = [%d]rt.PoseOBJRange{", len(g.poseOBJIndex)))
	for _, pose := range g.poseOBJIndex {
		b.line(fmt.Sprintf("{Start:%d, Count:%d, TileStart:%d, TileCount:%d},", pose.Start, pose.Count, pose.TileStart, pose.TileCount))
	}
	b.line("}")
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var poseOBJPartsROM = [%d]rt.PoseOBJ{", len(g.poseOBJParts)))
	for _, object := range g.poseOBJParts {
		b.line(fmt.Sprintf("{X:%d,Y:%d,TileOffset:%d,Width:%d,Height:%d,Flags:%d},", object.X, object.Y, object.TileOffset, object.Width, object.Height, object.Flags))
	}
	b.line("}")
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var poseOBJTilesROM = [%d]uint16{%s}", len(g.poseOBJTiles), u16Literal(g.poseOBJTiles, len(g.poseOBJTiles))))
	if g.poseOBJResident != nil {
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var poseOBJResidentROM = [%d]uint32{%s}", len(g.poseOBJResident.words), u32Literal(g.poseOBJResident.words, len(g.poseOBJResident.words))))
	}
}
