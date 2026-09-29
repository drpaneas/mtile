package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const animationDockHeight = 240
const objPageBarHeight = 40

func (g *Game) currentObjPageRect() editor.Rect {
	if g.model == nil || g.model.ObjSheet == nil {
		return editor.Rect{}
	}
	g.model.EnsureObjPages()
	pages := g.model.Animation.Pages
	if len(pages) == 0 {
		return editor.Rect{}
	}
	g.objPage = min(max(0, g.objPage), len(pages)-1)
	p := pages[g.objPage]
	return editor.Rect{X: p.X, Y: p.Y, W: p.Width, H: p.Height}
}

func (g *Game) visiblePagePoseIDs() []int {
	if g.model == nil || g.model.ObjSheet == nil {
		return nil
	}
	g.currentObjPageRect()
	return editor.PoseIDsOnObjPage(g.authoredPoses(), g.model.Animation.Pages[g.objPage], g.spriteCols())
}

func (g *Game) animationDockRect() editor.Rect {
	return editor.Rect{X: sheetPanelRect.X + 10, Y: sheetPanelRect.Y + sheetPanelRect.H - animationDockHeight - 10, W: sheetPanelRect.W - 20, H: animationDockHeight}
}

func (g *Game) animationWorkspaceRect() editor.Rect {
	return editor.Rect{X: spriteRailRect.X, Y: spriteRailRect.Y, W: sheetPanelRect.X + sheetPanelRect.W - spriteRailRect.X, H: sheetPanelRect.H}
}

func (g *Game) objPageContains(col, row int) bool {
	p := g.currentObjPageRect()
	return col >= p.X/8 && row >= p.Y/8 && col < (p.X+p.W)/8 && row < (p.Y+p.H)/8
}

func (g *Game) selectObjPage(index int) {
	if g.model == nil || index < 0 || index >= len(g.model.Animation.Pages) {
		return
	}
	g.cancelPoseNameEdit()
	g.objPage, g.objPageNameEditing = index, false
	g.spritePaste = nil
	g.sheetSelectionDragging = false
	g.invalidateAnimationPreview()
	p := g.currentObjPageRect()
	g.currentSprite = p.Y/8*g.spriteCols() + p.X/8
	g.gridCols, g.gridRows = 1, 1
	g.selectedPose = -1
	if ids := g.visiblePagePoseIDs(); len(ids) > 0 {
		g.selectedPose = ids[0]
	}
	g.poseStripScroll = 0
	g.fitSheetCamera()
	g.clampSpriteCanvasPan()
	g.requestRender()
}

func (g *Game) addObjPage() error {
	if g.model == nil || !g.objEditing {
		return fmt.Errorf("open Poses before adding a page")
	}
	index, err := g.model.AddObjPage()
	if err != nil {
		return err
	}
	g.objUsage = nil
	g.selectObjPage(index)
	g.dirty = true
	g.undo.forcePush(g.snapshot())
	p := g.model.Animation.Pages[index]
	g.statusMsg = fmt.Sprintf("%s added · %d × %d pixels · draw or paste artwork, then Register Pose", p.Name, p.Width, p.Height)
	return nil
}

// Cameras keep global sheet coordinates so existing selection, paste and pose
// overlays continue to address the same tiles. The visible bounds are one page.
func (g *Game) fitObjPageCamera() {
	p := g.currentObjPageRect()
	canvas := g.sheetGridCanvas()
	if p.W <= 0 || p.H <= 0 {
		return
	}
	// Match the Background editing surface: fit each axis to the available
	// canvas. Pose previews still display the artwork at its true aspect ratio.
	g.sheetCam.Zoom = min(float64(canvas.W)/float64(p.W), spriteSheetMaxFitZoom)
	g.sheetCam.ZoomY = min(float64(canvas.H)/float64(p.H), spriteSheetMaxFitZoom)
	g.sheetCam.X, g.sheetCam.Y = float64(p.X/8), float64(p.Y/8)
	g.clampObjPageCamera(canvas)
}

func (g *Game) clampObjPageCamera(canvas editor.Rect) {
	p := g.currentObjPageRect()
	spanX, spanY := g.sheetCam.VisibleSpan(canvas)
	clamp := func(value, start, length, span float64) float64 {
		if span >= length {
			return start
		}
		return min(max(value, start), start+length-span)
	}
	g.sheetCam.X = clamp(g.sheetCam.X, float64(p.X/8), float64(p.W/8), spanX)
	g.sheetCam.Y = clamp(g.sheetCam.Y, float64(p.Y/8), float64(p.H/8), spanY)
}

type objPageBarLayout struct {
	Prev, Next, Paste, Add editor.Rect
	Tabs                   []editor.Rect
	First                  int
}

func (g *Game) objPageBar() objPageBarLayout {
	x, y, width := sheetPanelRect.X+10, spritePanelContentY(sheetPanelRect), sheetPanelRect.W-20
	count := max(1, (width-284)/92)
	pages := len(g.model.Animation.Pages)
	if g.objPage < g.objPageFirstTab {
		g.objPageFirstTab = g.objPage
	}
	if g.objPage >= g.objPageFirstTab+count {
		g.objPageFirstTab = g.objPage - count + 1
	}
	g.objPageFirstTab = min(max(0, g.objPageFirstTab), max(0, pages-count))
	l := objPageBarLayout{
		Prev:  editor.Rect{X: x, Y: y, W: 32, H: 30},
		Next:  editor.Rect{X: x + width - 242, Y: y, W: 32, H: 30},
		Paste: editor.Rect{X: x + width - 202, Y: y, W: 96, H: 30},
		Add:   editor.Rect{X: x + width - 98, Y: y, W: 98, H: 30}, First: g.objPageFirstTab,
	}
	for i := 0; i < count && i+l.First < pages; i++ {
		l.Tabs = append(l.Tabs, editor.Rect{X: x + 40 + i*92, Y: y, W: 84, H: 30})
	}
	return l
}

func (g *Game) drawObjPageHeader(screen *ebiten.Image) {
	p := g.currentObjPageRect()
	x, y := spritePanelTitlePosition(sheetPanelRect)
	drawText(screen, "OBJ PAGES", x, y, colorText)
	remaining := assetsync.MaxObjSheetCells - g.model.ObjSheet.Width/8*(g.model.ObjSheet.Height/8)
	capacity := fmt.Sprintf("%d SLOTS LEFT", remaining)
	if remaining == 0 {
		capacity = "BANK FULL"
	}
	metaX := x + uiTextWidth("OBJ PAGES") + 12
	metaWidth := sheetPanelRect.X + sheetPanelRect.W - 14 - metaX
	meta := fmt.Sprintf("%d × %d PX · %d TILES · %s", p.W, p.H, p.W/8*(p.H/8), capacity)
	if smallTextWidth(meta) > metaWidth {
		meta = fmt.Sprintf("%d × %d PX · %s", p.W, p.H, capacity)
	}
	drawSmallText(screen, truncateTextToWidth(meta, metaWidth, uiSmallFace), metaX, spritePanelMetaY(sheetPanelRect), colorDim)
	l := g.objPageBar()
	drawObjPageAction(screen, l.Prev, "<", g.objPage > 0)
	drawObjPageAction(screen, l.Next, ">", g.objPage+1 < len(g.model.Animation.Pages))
	drawTactileButton(screen, l.Paste, "PASTE", false, colorSprite, true)
	nextWidth, nextHeight := g.model.NextObjPageDimensions()
	drawObjPageAction(screen, l.Add, "PAGE+", nextWidth > 0 && nextHeight > 0)
	for i, r := range l.Tabs {
		id := i + l.First
		name := g.model.Animation.Pages[id].Name
		if g.objPageNameEditing && id == g.objPage {
			name = g.objPageNameDraft + "_"
		}
		drawTactileButton(screen, r, truncateUIText(name, r.W-12), id == g.objPage, colorSprite, true)
		if g.objShowReuse && g.objPageSharesSelectedTile(id) {
			strokeRect(screen, r, colorHighlight)
		}
	}
}

func drawObjPageAction(screen *ebiten.Image, rect editor.Rect, label string, enabled bool) {
	if enabled {
		drawTactileButton(screen, rect, label, false, colorSprite, true)
		return
	}
	fillRect(screen, rect, colorBackground)
	strokeRect(screen, rect, rgba(colorBorder, 100))
	drawSmallTextCentered(screen, label, rect, colorDim)
}

func (g *Game) updateObjPageHeader(mx, my int) bool {
	if !g.objEditing || !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return false
	}
	l := g.objPageBar()
	switch {
	case rectContains(l.Paste, mx, my):
		g.pasteSpriteClipboard()
		return true
	case rectContains(l.Add, mx, my):
		if err := g.addObjPage(); err != nil {
			g.statusMsg = err.Error()
			g.requestRender()
		}
		return true
	case rectContains(l.Prev, mx, my):
		g.selectObjPage(g.objPage - 1)
		return true
	case rectContains(l.Next, mx, my):
		g.selectObjPage(g.objPage + 1)
		return true
	}
	for i, r := range l.Tabs {
		if rectContains(r, mx, my) {
			id := i + l.First
			if id == g.objPage {
				g.cancelPoseNameEdit()
				g.objPageNameEditing = true
				g.objPageNameSelectAll = true
				g.objPageNameDraft = g.model.Animation.Pages[id].Name
				g.statusMsg = "Rename page: type a name, Enter to save, Esc to cancel"
			} else {
				g.selectObjPage(id)
			}
			g.requestRender()
			return true
		}
	}
	return false
}

func (g *Game) updateObjPageName() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.objPageNameEditing = false
		g.requestRender()
		return
	}
	modifier := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	if modifier && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		g.objPageNameSelectAll = true
	}
	if !modifier {
		for _, r := range ebiten.AppendInputChars(nil) {
			draft := g.objPageNameDraft
			if g.objPageNameSelectAll {
				draft = ""
			}
			if !unicode.IsControl(r) && len(draft)+utf8.RuneLen(r) <= assetsync.MaxPoseNameBytes {
				g.objPageNameDraft, g.objPageNameSelectAll = draft+string(r), false
			}
		}
	}
	if codeKeyRepeat(ebiten.KeyBackspace) && g.objPageNameDraft != "" {
		if g.objPageNameSelectAll {
			g.objPageNameDraft = ""
		} else {
			_, n := utf8.DecodeLastRuneInString(g.objPageNameDraft)
			g.objPageNameDraft = g.objPageNameDraft[:len(g.objPageNameDraft)-n]
		}
		g.objPageNameSelectAll = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		name := strings.TrimSpace(g.objPageNameDraft)
		if name == "" {
			g.statusMsg = "Page name cannot be empty"
			return
		}
		for i, p := range g.model.Animation.Pages {
			if i != g.objPage && p.Name == name {
				g.statusMsg = "A page already uses that name"
				return
			}
		}
		g.model.Animation.Pages[g.objPage].Name = name
		g.objPageNameEditing = false
		g.dirty = true
		g.undo.forcePush(g.snapshot())
		g.statusMsg = "Page renamed"
	}
	g.requestRender()
}

func (g *Game) revealPosePage(id int) {
	if !g.objEditing || id < 0 || id >= len(g.authoredPoses()) {
		return
	}
	pose := g.authoredPoses()[id]
	if len(pose.Parts) == 0 {
		return
	}
	g.currentObjPageRect()
	for i, p := range g.model.Animation.Pages {
		if editor.ObjPageContainsTile(p, pose.Parts[0].Tile, g.spriteCols()) {
			if i != g.objPage {
				g.objPage = i
				g.invalidateAnimationPreview()
				g.fitSheetCamera()
			}
			return
		}
	}
}

func (g *Game) pageSafeSelection(col, row, width, height int) (int, int, int, int) {
	if !g.objEditing {
		return col, row, width, height
	}
	p := g.currentObjPageRect()
	col = min(max(col, p.X/8), (p.X+p.W)/8-1)
	row = min(max(row, p.Y/8), (p.Y+p.H)/8-1)
	return col, row, min(width, (p.X+p.W)/8-col), min(height, (p.Y+p.H)/8-row)
}

func (g *Game) sheetContentClip(canvas editor.Rect) editor.Rect {
	if !g.objEditing {
		return canvas
	}
	p := g.currentObjPageRect()
	board := sheetCameraRect(g.sheetCam, canvas, p.X/8, p.Y/8, p.W/8, p.H/8)
	board, _ = intersectEditorRects(board, canvas)
	return board
}
