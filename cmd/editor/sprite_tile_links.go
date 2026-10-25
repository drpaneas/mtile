package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

// colorTileLink marks linked shared tiles in Sprite Lab and Map Lab.
var colorTileLink = color.RGBA{R: 255, G: 176, B: 64, A: 255}

// tileLinkTarget is one pose or animation clip a shared tile can link to.
type tileLinkTarget struct {
	pose int // -1 for a clip
	clip string
}

func (t tileLinkTarget) label(g *Game) string {
	if t.pose >= 0 {
		name := ""
		if t.pose < len(g.authoredPoses()) {
			name = g.authoredPoses()[t.pose].Name
		}
		if name == "" {
			return fmt.Sprintf("POSE %d", t.pose)
		}
		return fmt.Sprintf("POSE %d %s", t.pose, name)
	}
	return "CLIP " + t.clip
}

func tileLinkLabel(link assetsync.TileLink) string {
	if link.Pose != nil {
		return fmt.Sprintf("pose %d", *link.Pose)
	}
	return "clip " + link.Animation
}

// tileLinkTargets lists every registered pose, then every animation clip.
func (g *Game) tileLinkTargets() []tileLinkTarget {
	var targets []tileLinkTarget
	for id := range g.authoredPoses() {
		targets = append(targets, tileLinkTarget{pose: id})
	}
	if g.model != nil {
		for _, clip := range g.model.Animation.Clips {
			targets = append(targets, tileLinkTarget{pose: -1, clip: clip.Name})
		}
	}
	return targets
}

func tileLinkRowY() int { return flagsRect.Y + spriteButtonHeight + 8 }

func tileLinkPrevRect() editor.Rect {
	return editor.Rect{X: flagsRect.X, Y: tileLinkRowY(), W: 28, H: spriteButtonHeight}
}

func tileLinkTargetRect() editor.Rect {
	return editor.Rect{X: flagsRect.X + 32, Y: tileLinkRowY(), W: max(40, flagsRect.W-32-32-62-80), H: spriteButtonHeight}
}

func tileLinkNextRect() editor.Rect {
	r := tileLinkTargetRect()
	return editor.Rect{X: r.X + r.W + 4, Y: tileLinkRowY(), W: 28, H: spriteButtonHeight}
}

func tileLinkLinkRect() editor.Rect {
	r := tileLinkNextRect()
	return editor.Rect{X: r.X + r.W + 6, Y: tileLinkRowY(), W: 58, H: spriteButtonHeight}
}

func tileLinkUnlinkRect() editor.Rect {
	r := tileLinkLinkRect()
	return editor.Rect{X: r.X + r.W + 4, Y: tileLinkRowY(), W: flagsRect.X + flagsRect.W - (r.X + r.W + 4), H: spriteButtonHeight}
}

// selectedSharedTiles returns the shared tiles in the current selection.
func (g *Game) selectedSharedTiles() []int {
	baseRow, baseCol := g.baseSpriteRowCol()
	var tiles []int
	for dr := 0; dr < g.spriteGridRows(); dr++ {
		for dc := 0; dc < g.spriteGridCols(); dc++ {
			row, col := baseRow+dr, baseCol+dc
			if row < g.spriteRows() && col < g.spriteCols() {
				tiles = append(tiles, row*g.spriteCols()+col)
			}
		}
	}
	return tiles
}

// syncTileLinkChoice points the selector at the selected tile's link
// whenever the selection changes, so LINK re-applies the current target.
func (g *Game) syncTileLinkChoice() {
	tiles := g.selectedSharedTiles()
	if len(tiles) == 0 || g.model == nil || g.tileLinkSyncedTile == tiles[0] {
		return
	}
	g.tileLinkSyncedTile = tiles[0]
	link, ok := g.model.TileLinkFor(tiles[0])
	if !ok {
		return
	}
	for i, target := range g.tileLinkTargets() {
		if link.Pose != nil && target.pose == *link.Pose || link.Pose == nil && target.pose < 0 && target.clip == link.Animation {
			g.tileLinkChoice = i
			return
		}
	}
}

// tileLinkActions reports which link-row buttons apply to the selection:
// LINK when a selected tile can be linked and is not yet (and a target
// exists), UNLINK when a selected tile is linked. The selector follows LINK.
func (g *Game) tileLinkActions() (canLink, canUnlink bool) {
	if g.model == nil {
		return false, false
	}
	for _, tile := range g.selectedSharedTiles() {
		if _, linked := g.model.TileLinkFor(tile); linked {
			canUnlink = true
		} else if !editor.SpriteIsReserved(tile) {
			canLink = true
		}
	}
	return canLink && len(g.tileLinkTargets()) > 0, canUnlink
}

func (g *Game) updateTileLinkControls(mx, my int) {
	g.syncTileLinkChoice()
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	targets := g.tileLinkTargets()
	canLink, canUnlink := g.tileLinkActions()
	switch {
	case rectContains(tileLinkPrevRect(), mx, my) && canLink:
		g.tileLinkChoice = (g.tileLinkChoice - 1 + len(targets)) % len(targets)
	case rectContains(tileLinkNextRect(), mx, my) && canLink:
		g.tileLinkChoice = (g.tileLinkChoice + 1) % len(targets)
	case rectContains(tileLinkLinkRect(), mx, my) && canLink:
		g.linkSelectedTiles(targets)
	case rectContains(tileLinkUnlinkRect(), mx, my) && canUnlink:
		g.unlinkSelectedTiles()
	default:
		return
	}
	g.requestRender()
}

func (g *Game) linkSelectedTiles(targets []tileLinkTarget) {
	if len(targets) == 0 {
		g.statusMsg = "Register a pose or an animation clip in Poses before linking a tile"
		return
	}
	target := targets[min(g.tileLinkChoice, len(targets)-1)]
	changed := 0
	for _, tile := range g.selectedSharedTiles() {
		var err error
		if target.pose >= 0 {
			err = g.model.LinkTileToPose(tile, target.pose)
		} else {
			err = g.model.LinkTileToAnimation(tile, target.clip)
		}
		if err != nil {
			g.statusMsg = err.Error()
			continue
		}
		changed++
	}
	if changed == 0 {
		return
	}
	g.undo.forcePush(g.snapshot())
	g.dirty = true
	g.statusMsg = fmt.Sprintf("Linked %d tile(s) to %s: every map cell holding them draws it instead", changed, target.label(g))
}

func (g *Game) unlinkSelectedTiles() {
	changed := 0
	for _, tile := range g.selectedSharedTiles() {
		if g.model.UnlinkTile(tile) {
			changed++
		}
	}
	if changed == 0 {
		g.statusMsg = "The selected tile is not linked"
		return
	}
	g.undo.forcePush(g.snapshot())
	g.dirty = true
	g.statusMsg = fmt.Sprintf("Unlinked %d tile(s): the map draws them as tiles again", changed)
}

func (g *Game) drawTileLinkControls(screen *ebiten.Image) {
	g.syncTileLinkChoice()
	targets := g.tileLinkTargets()
	label := "NO POSES OR CLIPS"
	if len(targets) > 0 {
		label = targets[min(g.tileLinkChoice, len(targets)-1)].label(g)
	}
	canLink, canUnlink := g.tileLinkActions()
	button := func(r editor.Rect, label string, enabled bool) {
		if enabled {
			drawTactileButton(screen, r, label, false, colorTileLink, true)
		} else {
			drawDisabledTactileButton(screen, r, label, true)
		}
	}
	button(tileLinkPrevRect(), "<", canLink)
	button(tileLinkTargetRect(), truncateUIText(label, tileLinkTargetRect().W-12), canLink)
	button(tileLinkNextRect(), ">", canLink)
	button(tileLinkLinkRect(), "LINK", canLink)
	button(tileLinkUnlinkRect(), "UNLINK", canUnlink)

	// The panel header names the selected tile's link, like TILE REUSE does.
	status := "NOT LINKED"
	clr := color.Color(colorDim)
	if tiles := g.selectedSharedTiles(); len(tiles) > 0 && g.model != nil {
		if link, ok := g.model.TileLinkFor(tiles[0]); ok {
			target := "CLIP " + link.Animation // clip names are case-sensitive
			if link.Pose != nil {
				target = fmt.Sprintf("POSE %d", *link.Pose)
			}
			status = fmt.Sprintf("TILE %d -> %s", tiles[0], target)
			clr = colorTileLink
		}
	}
	status = truncateUIText(status, spriteFlagsPanelRect.W/2)
	drawSmallText(screen, status, spriteFlagsPanelRect.X+spriteFlagsPanelRect.W-smallTextWidth(status)-14, spritePanelMetaY(spriteFlagsPanelRect), clr)
}

// tileLinked reports whether a shared tile draws as a linked object.
func (g *Game) tileLinked(id int) bool {
	if g.model == nil || id <= 0 {
		return false
	}
	_, ok := g.model.TileLinkFor(id)
	return ok
}

// drawTileLinkMarker outlines a linked cell and flags its top-left corner.
func drawTileLinkMarker(screen *ebiten.Image, r editor.Rect) {
	strokeRect(screen, r, colorTileLink)
	corner := max(3, r.W/4)
	fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: corner, H: corner}, colorTileLink)
}
