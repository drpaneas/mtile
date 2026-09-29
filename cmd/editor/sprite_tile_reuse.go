package main

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type objTileUsage struct {
	pixels           []byte
	analysis         assetsync.ObjectSheetCellAnalysis
	groups           map[int][]int
	poses            map[int]map[int]bool
	directReferences map[int]int
}

func (g *Game) objectTileUsage() *objTileUsage {
	if g.objUsage != nil {
		return g.objUsage
	}
	u := &objTileUsage{groups: make(map[int][]int), poses: make(map[int]map[int]bool), directReferences: make(map[int]int)}
	if g.model == nil || g.model.ObjSheet == nil {
		return u
	}
	s := g.model.ObjSheet
	u.pixels = make([]byte, s.Width*s.Height)
	for _, sprite := range s.Sprites {
		for y, row := range sprite.Pixels {
			for x, pixel := range row {
				px, py := sprite.X+x, sprite.Y+y
				if px >= 0 && py >= 0 && px < s.Width && py < s.Height {
					u.pixels[py*s.Width+px] = byte(pixel)
				}
			}
		}
	}
	u.analysis = assetsync.AnalyzeObjectSheetCells(u.pixels, s.Width, s.Width/8*(s.Height/8))
	for id, key := range u.analysis.CanonicalIDs {
		if key >= 0 {
			u.groups[key] = append(u.groups[key], id)
		}
	}
	for id, pose := range g.authoredPoses() {
		seen := make(map[int]bool)
		for _, part := range pose.Parts {
			if part.Tile < 0 || part.Tile >= len(u.analysis.CanonicalIDs) {
				continue
			}
			key := u.analysis.CanonicalIDs[part.Tile]
			if key < 0 {
				continue
			}
			if !seen[part.Tile] {
				u.directReferences[part.Tile]++
				seen[part.Tile] = true
			}
			if u.poses[key] == nil {
				u.poses[key] = make(map[int]bool)
			}
			u.poses[key][id] = true
		}
	}
	g.objUsage = u
	return u
}

func (g *Game) selectedTileCanonical() int {
	u := g.objectTileUsage()
	if g.currentSprite < 0 || g.currentSprite >= len(u.analysis.CanonicalIDs) {
		return -1
	}
	return u.analysis.CanonicalIDs[g.currentSprite]
}

func (g *Game) poseSharesSelectedTile(id int) bool {
	if !g.objEditing || !g.objShowReuse {
		return false
	}
	key := g.selectedTileCanonical()
	return key >= 0 && g.objectTileUsage().poses[key][id]
}

func (g *Game) objPageSharesSelectedTile(id int) bool {
	if id < 0 || id >= len(g.model.Animation.Pages) {
		return false
	}
	key := g.selectedTileCanonical()
	if key < 0 {
		return false
	}
	for _, tile := range g.objectTileUsage().groups[key] {
		if editor.ObjPageContainsTile(g.model.Animation.Pages[id], tile, g.spriteCols()) {
			return true
		}
	}
	return false
}

func tileReuseButtonRect() editor.Rect {
	return editor.Rect{X: flagsRect.X, Y: flagsRect.Y, W: flagsRect.W, H: spriteButtonHeight}
}

func (g *Game) drawObjTileReuse(screen *ebiten.Image) {
	u := g.objectTileUsage()
	drawSpritePanelTitle(screen, spriteFlagsPanelRect, "TILE REUSE")
	label := "AUTO AT BUILD"
	drawSmallText(screen, label, spriteFlagsPanelRect.X+spriteFlagsPanelRect.W-smallTextWidth(label)-14, spritePanelMetaY(spriteFlagsPanelRect), colorDim)
	drawTactileButton(screen, tileReuseButtonRect(), "HIGHLIGHT MATCHING TILES", g.objShowReuse, colorHighlight, true)
	summary := fmt.Sprintf("ROM: %d unique · %.1f KiB saved", u.analysis.UniqueTiles, float64(u.analysis.SavedBytes)/1024)
	y := flagsRect.Y + spriteButtonHeight + 8
	drawSmallText(screen, truncateUIText(summary, flagsRect.W), flagsRect.X, y, colorDim)
	summary = "Select a tile, then highlight its matches."
	if g.objShowReuse {
		key := g.selectedTileCanonical()
		if key < 0 {
			summary = "Select a painted tile to find its matches"
		} else {
			summary = fmt.Sprintf("Tile %d: %d copies · used by %d poses", g.currentSprite, len(u.groups[key]), len(u.poses[key]))
			if n := u.directReferences[g.currentSprite]; n > 1 {
				summary = fmt.Sprintf("%d copies · %d poses · %d linked", len(u.groups[key]), len(u.poses[key]), n)
			}
		}
	}
	drawSmallText(screen, truncateUIText(summary, flagsRect.W), flagsRect.X, y+uiSmallTextHeight+6, colorDim)
	if y+2*(uiSmallTextHeight+6)+uiSmallTextHeight+4 <= spriteFlagsPanelRect.Y+spriteFlagsPanelRect.H {
		hint := "Identical copies can still be edited independently."
		if n := u.directReferences[g.currentSprite]; n > 1 {
			hint = fmt.Sprintf("Editing this tile changes %d linked poses.", n)
		}
		drawSmallText(screen, truncateUIText(hint, flagsRect.W), flagsRect.X, y+2*(uiSmallTextHeight+6), colorDim)
	}
}

func (g *Game) updateObjTileReuse(mx, my int) {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && rectContains(tileReuseButtonRect(), mx, my) {
		g.objShowReuse = !g.objShowReuse
		g.statusMsg = "Yellow marks equal tile pixels, their pages and poses. Builds store each pattern once."
		if n := g.objectTileUsage().directReferences[g.currentSprite]; g.objShowReuse && n > 1 {
			g.statusMsg = fmt.Sprintf("This tile is linked to %d poses: painting it changes them all. Equal copies elsewhere stay independent.", n)
		}
		g.requestRender()
	}
}

func (g *Game) drawObjTileMatches(screen *ebiten.Image, canvas editor.Rect) {
	if !g.objEditing || !g.objShowReuse {
		return
	}
	key := g.selectedTileCanonical()
	if key < 0 {
		return
	}
	for _, tile := range g.objectTileUsage().groups[key] {
		col, row := tile%g.spriteCols(), tile/g.spriteCols()
		if !g.objPageContains(col, row) {
			continue
		}
		r := sheetCameraRect(g.sheetCam, canvas, col, row, 1, 1)
		if r, ok := intersectEditorRects(r, canvas); ok {
			fillRect(screen, r, rgba(colorHighlight, 55))
			strokeRect(screen, r, colorHighlight)
		}
	}
}
