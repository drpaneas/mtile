package main

import (
	"slices"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

type developAsset struct {
	Name string
	ID   int // Zero-based index in the authored pose or clip list.
}

func (g *Game) developAssets(kind string) []developAsset {
	if g == nil || g.model == nil {
		return nil
	}
	var assets []developAsset
	switch kind {
	case "pspr":
		for id, pose := range g.authoredPoses() {
			if pose.Name != "" {
				assets = append(assets, developAsset{Name: pose.Name, ID: id})
			}
		}
	case "anim":
		for id, clip := range g.model.Animation.Clips {
			assets = append(assets, developAsset{Name: clip.Name, ID: id})
		}
	}
	return assets
}

func (g *Game) developAssetThumbnail(kind string, id int) editor.PosePreview {
	if g == nil || g.model == nil || id < 0 {
		return editor.PosePreview{}
	}
	switch kind {
	case "pspr":
		if id < len(g.authoredPoses()) {
			return g.animationPoseThumbnail(id)
		}
	case "anim":
		if clips := g.model.Animation.Clips; id < len(clips) && len(clips[id].Frames) != 0 {
			return g.animationPoseThumbnail(clips[id].Frames[0])
		}
	}
	return editor.PosePreview{}
}

func (g *Game) drawDevelopAssetThumbnail(screen *ebiten.Image, kind string, id int, rect editor.Rect) {
	if rect.W <= 8 || rect.H <= 8 {
		return
	}
	g.drawAnimationFrame(screen, g.developAssetThumbnail(kind, id), rect)
}

func (g *Game) openDevelopAsset(kind string, id int) bool {
	if g == nil || g.model == nil || g.mode != modeCode || id < 0 {
		return false
	}
	switch kind {
	case "pspr":
		if id >= len(g.authoredPoses()) {
			return false
		}
	case "anim":
		if id >= len(g.model.Animation.Clips) {
			return false
		}
	default:
		return false
	}
	objBank := g.model.ObjSheet != nil
	if objBank != g.objEditing {
		if err := g.setObjEditing(objBank); err != nil {
			g.statusMsg = err.Error()
			g.requestRender()
			return false
		}
	}

	// An asset jump suspends Develop like a workspace switch without saving
	// the source buffer or creating an OBJ sheet for legacy artwork.
	g.developGameFocus = false
	g.suspendPlaySession()
	g.mode = modeSprites
	g.setAnimationWorkspaceActive(kind == "anim")
	if kind == "anim" {
		g.selectAnimationClip(id + 1)
		return true
	}

	g.selectPose(id)
	if g.objEditing {
		g.animation.scroll = slices.Index(g.pageAnimationPoses(), id)
		g.clampAnimationScroll(newAnimationDockLayout(g.animationDockRect()))
	} else {
		g.poseStripScroll = g.poseStripContentX(id)
		g.clampPoseStripScroll()
	}
	return true
}
