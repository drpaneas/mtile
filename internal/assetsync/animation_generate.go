package assetsync

import (
	"fmt"
	"strings"
)

func writeAnimationArrays(b *strings.Builder, assets *Assets) {
	b.WriteString("//go:section .rodata\n")
	fmt.Fprintf(b, "var animationClipROM = [%d]rt.AnimationClip{\n", len(assets.AnimationClips))
	for _, clip := range assets.AnimationClips {
		fmt.Fprintf(b, "{Name:%q,Start:%d,Count:%d,FPS:%d,Loop:%t,FlipOffsetX:%d,FlipOffsetY:%d},\n", clip.Name, clip.Start, clip.Count, clip.FPS, clip.Loop, clip.FlipOffsetX, clip.FlipOffsetY)
	}
	b.WriteString("}\n\n//go:section .rodata\n")
	writeUint16Array(b, "animationFrameROM", assets.AnimationFrames)
}
