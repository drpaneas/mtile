package build

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	rt "github.com/drpaneas/mtile/runtime"
)

func flattenAssetAnimations(assets *assetsync.Assets) []rt.AnimationClip {
	clips := make([]rt.AnimationClip, len(assets.AnimationClips))
	for i, clip := range assets.AnimationClips {
		clips[i] = rt.AnimationClip{Name: clip.Name, Start: clip.Start, Count: clip.Count, FPS: clip.FPS, Loop: clip.Loop, FlipOffsetX: clip.FlipOffsetX, FlipOffsetY: clip.FlipOffsetY}
	}
	return clips
}

func flattenAssetTileLinks(assets *assetsync.Assets) []rt.TileLink {
	links := make([]rt.TileLink, len(assets.TileLinks))
	for i, link := range assets.TileLinks {
		links[i] = rt.TileLink{Tile: link.Tile, Clip: link.Clip, ID: link.ID}
	}
	return links
}
