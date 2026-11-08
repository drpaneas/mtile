package main

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	poseStripGap          = 8
	poseStripPreviewH     = 72
	poseStripSlotPad      = 8
	poseStripSlotMinW     = 56
	poseStripWheelStep    = 24
	poseStripControlW     = 76
	poseStripControlGap   = 6
	poseStripControlCount = 4
)

var writePoseUsageClipboard = hostclipboard.Write

func (g *Game) poseUsage() string {
	poses := g.authoredPoses()
	if g.selectedPose < 0 || g.selectedPose >= len(poses) {
		return ""
	}
	name := poses[g.selectedPose].Name
	if name == "" {
		return fmt.Sprintf("pspr(%d, x, y)", g.selectedPose)
	}
	return `pspr("` + escapeLuaAssetName(name, '"') + `", x, y)`
}

func escapeLuaAssetName(name string, quote byte) string {
	// Asset JSON may contain names outside the editor's identifier alphabet.
	// Use Lua decimal escapes for control bytes, rather than Go's \x escapes.
	var literal strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '\\' || c == quote:
			literal.WriteByte('\\')
			literal.WriteByte(c)
		case c < 32 || c == 127:
			fmt.Fprintf(&literal, "\\%03d", c)
		default:
			literal.WriteByte(c)
		}
	}
	return literal.String()
}

func (g *Game) copyPoseUsage() {
	usage := g.poseUsage()
	if usage == "" {
		g.statusMsg = "Select a registered pose to copy its Lua call"
	} else if err := writePoseUsageClipboard(usage); err != nil {
		g.statusMsg = "Could not copy pose call: " + err.Error()
	} else {
		g.statusMsg = "Copied " + usage
	}
	g.requestRender()
}

func poseStripHeight() int {
	preview := poseStripSlotPad + poseStripPreviewH + 4 + uiSmallTextHeight + poseStripSlotPad
	controls := poseStripSlotPad*2 + poseStripControlCount*18 + (poseStripControlCount-1)*poseStripControlGap
	return max(preview, controls)
}

func poseStripControlWidth() int {
	return max(poseStripControlW, smallTextWidth("COPY LUA")+16)
}

func poseStripControlsWidth() int {
	return poseStripControlWidth() + poseStripControlGap
}

func (g *Game) authoredPoses() []assetsync.PoseRecord {
	if g == nil || g.model == nil {
		return nil
	}
	return g.model.Poses.Poses
}

func (g *Game) poseBankIsActive() bool {
	if g == nil || g.model == nil {
		return false
	}
	return (g.model.ObjSheet != nil) == g.objEditing
}

func (g *Game) showsPoseStrip() bool {
	if !g.poseBankIsActive() {
		return false
	}
	if g.objEditing {
		return true
	}
	return len(g.authoredPoses()) > 0
}

func (g *Game) ensurePoseSelection() {
	if !g.poseBankIsActive() {
		return
	}
	n := len(g.authoredPoses())
	if n == 0 {
		g.selectedPose = -1
		return
	}
	if g.selectedPose < 0 || g.selectedPose >= n {
		g.selectedPose = 0
	}
}

func (g *Game) selectPose(id int) {
	poses := g.authoredPoses()
	if id < 0 || id >= len(poses) {
		return
	}
	g.selectedPose = id
	g.revealPosePage(id)
	g.focusPoseOnSheet(id)
	g.requestRender()
}

func (g *Game) focusPoseOnSheet(id int) {
	poses := g.authoredPoses()
	if id < 0 || id >= len(poses) {
		return
	}
	col, row, width, height, ok := editor.PoseSheetBounds(poses[id], g.spriteCols(), g.spriteRows())
	if !ok {
		return
	}
	col, row, width, height = g.pageSafeSelection(col, row, width, height)
	if width > g.spriteSelectionLimitCols() {
		width = g.spriteSelectionLimitCols()
	}
	if height > g.spriteSelectionLimitRows() {
		height = g.spriteSelectionLimitRows()
	}
	g.currentSprite = row*g.spriteCols() + col
	g.gridCols = max(gridCellsMin, width)
	g.gridRows = max(gridCellsMin, height)
	g.clampSpriteCanvasPan()
}

func (g *Game) poseHighlightsTile(tile int) bool {
	poses := g.authoredPoses()
	if g.selectedPose < 0 || g.selectedPose >= len(poses) {
		return false
	}
	return editor.PoseContainsTile(poses[g.selectedPose], tile)
}

func (g *Game) syncPoseSelectionFromSprite(tile int) {
	if !g.showsPoseStrip() {
		return
	}
	poses := g.authoredPoses()
	if g.selectedPose >= 0 && g.selectedPose < len(poses) && editor.PoseContainsTile(poses[g.selectedPose], tile) {
		return
	}
	g.selectedPose = editor.FirstPoseContainingTile(poses, tile)
}

func (g *Game) poseStripRect() editor.Rect {
	if !g.showsPoseStrip() {
		return editor.Rect{}
	}
	canvas := g.sheetGridCanvas()
	return editor.Rect{
		X: canvas.X,
		Y: canvas.Y + canvas.H + poseStripGap,
		W: canvas.W,
		H: poseStripHeight(),
	}
}

func (g *Game) poseStripTrackRect() editor.Rect {
	strip := g.poseStripRect()
	if strip.W <= 0 {
		return editor.Rect{}
	}
	return editor.Rect{
		X: strip.X,
		Y: strip.Y,
		W: max(1, strip.W-poseStripControlsWidth()),
		H: strip.H,
	}
}

func (g *Game) poseStripControlRect(index int) editor.Rect {
	strip := g.poseStripRect()
	if strip.W <= 0 || index < 0 || index >= poseStripControlCount {
		return editor.Rect{}
	}
	buttonH := (strip.H - poseStripSlotPad*2 - poseStripControlGap*(poseStripControlCount-1)) / poseStripControlCount
	if buttonH < 18 {
		buttonH = 18
	}
	return editor.Rect{
		X: strip.X + strip.W - poseStripControlWidth(),
		Y: strip.Y + poseStripSlotPad + index*(buttonH+poseStripControlGap),
		W: poseStripControlWidth(),
		H: buttonH,
	}
}

func (g *Game) poseStripNewRect() editor.Rect { return g.poseStripControlRect(0) }

func (g *Game) poseStripSetRect() editor.Rect { return g.poseStripControlRect(1) }

func (g *Game) poseStripDelRect() editor.Rect { return g.poseStripControlRect(2) }

func (g *Game) poseStripCopyRect() editor.Rect { return g.poseStripControlRect(3) }

func (g *Game) posePreview(id int) editor.PosePreview {
	poses := g.authoredPoses()
	if id < 0 || id >= len(poses) {
		return editor.PosePreview{}
	}
	sheet := g.model.ObjSheet
	if sheet == nil {
		sheet = &g.model.Sheet
	}
	return editor.ComposePose(*sheet, poses[id])
}

func posePreviewFit(width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		return poseStripPreviewH / 2, poseStripPreviewH
	}
	dw := max(16, (width*poseStripPreviewH+height/2)/height)
	return dw, poseStripPreviewH
}

func (g *Game) poseStripSlotWidth(id int) int {
	sheet := g.activeSheet()
	poses := g.authoredPoses()
	if sheet == nil || id < 0 || id >= len(poses) {
		return poseStripSlotMinW
	}
	_, _, width, height := editor.PosePixelBounds(*sheet, poses[id])
	dw, _ := posePreviewFit(width, height)
	return max(poseStripSlotMinW, dw+poseStripSlotPad*2)
}

func (g *Game) poseStripContentX(id int) int {
	x := 0
	for i := 0; i < id; i++ {
		x += g.poseStripSlotWidth(i)
	}
	return x
}

func (g *Game) poseStripCellRect(id int) editor.Rect {
	track := g.poseStripTrackRect()
	if track.W <= 0 || id < 0 || id >= len(g.authoredPoses()) {
		return editor.Rect{}
	}
	return editor.Rect{
		X: track.X + g.poseStripContentX(id) - g.poseStripScroll,
		Y: track.Y,
		W: g.poseStripSlotWidth(id),
		H: track.H,
	}
}

func (g *Game) poseIndexAt(mx, my int) int {
	if !rectContains(g.poseStripTrackRect(), mx, my) {
		return -1
	}
	x := g.poseStripTrackRect().X - g.poseStripScroll
	for id := range g.authoredPoses() {
		width := g.poseStripSlotWidth(id)
		if mx >= x && mx < x+width {
			return id
		}
		x += width
	}
	return -1
}

func (g *Game) clampPoseStripScroll() {
	total := 0
	for id := range g.authoredPoses() {
		total += g.poseStripSlotWidth(id)
	}
	maxScroll := max(0, total-g.poseStripTrackRect().W)
	if g.poseStripScroll < 0 {
		g.poseStripScroll = 0
	}
	if g.poseStripScroll > maxScroll {
		g.poseStripScroll = maxScroll
	}
}

func poseOverlayColor(index int) color.RGBA {
	colors := []color.RGBA{colorSprite, colorHighlight, colorMusic, colorCode, colorMap, colorRun}
	if index < 0 {
		return colorSprite
	}
	return colors[index%len(colors)]
}

func (g *Game) updatePoseStrip(mx, my int) {
	if g.objEditing {
		g.updateAnimationDock(mx, my)
		return
	}
	if !g.showsPoseStrip() {
		return
	}
	strip := g.poseStripRect()
	if !rectContains(strip, mx, my) {
		return
	}
	if _, yoff := ebiten.Wheel(); yoff != 0 && rectContains(g.poseStripTrackRect(), mx, my) {
		g.poseStripScroll -= int(yoff * poseStripWheelStep)
		g.clampPoseStripScroll()
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	switch {
	case rectContains(g.poseStripNewRect(), mx, my):
		g.capturePoseFromSelection(false)
	case rectContains(g.poseStripSetRect(), mx, my):
		g.capturePoseFromSelection(true)
	case rectContains(g.poseStripDelRect(), mx, my):
		g.deleteSelectedPose()
	case rectContains(g.poseStripCopyRect(), mx, my):
		g.copyPoseUsage()
	default:
		if id := g.poseIndexAt(mx, my); id >= 0 {
			if id == g.selectedPose && rectContains(g.poseStripLabelRect(id), mx, my) {
				g.beginPoseNameEdit()
				return
			}
			g.selectPose(id)
		}
	}
}

func (g *Game) capturePoseFromSelection(replace bool) {
	sheet := g.activeSheet()
	if sheet == nil {
		return
	}
	if replace && (g.selectedPose < 0 || g.selectedPose >= len(g.authoredPoses())) {
		g.statusMsg = "Select a registered pose to update"
		return
	}
	if !replace && len(g.authoredPoses()) >= assetsync.MaxPoseCount {
		g.statusMsg = fmt.Sprintf("Already have %d registered poses", assetsync.MaxPoseCount)
		return
	}
	startCol, startRow, width, height, ok := g.selectionPixelBlock()
	if !ok {
		g.statusMsg = "pose selection is empty"
		return
	}
	pose, err := editor.CapturePose(*sheet, startCol, startRow, width/8, height/8)
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	if g.model.Poses.Version == 0 {
		g.model.Poses.Version = 1
	}
	if replace {
		pose.Name = g.model.Poses.Poses[g.selectedPose].Name
		g.model.Poses.Poses[g.selectedPose] = pose
	} else {
		pose.Name = editor.NextPoseName(g.authoredPoses(), "idle")
		g.model.Poses.Poses = append(g.model.Poses.Poses, pose)
		g.selectedPose = len(g.model.Poses.Poses) - 1
	}
	g.selectPose(g.selectedPose)
	g.commitPoseEdit(fmt.Sprintf("pspr %s from %d painted tiles", editor.PoseFrameLabel(g.selectedPose, pose), len(pose.Parts)))
	if !replace {
		g.beginPoseNameEdit()
	}
}

func (g *Game) deleteSelectedPose() {
	g.cancelPoseNameEdit()
	if g.selectedPose < 0 || g.selectedPose >= len(g.authoredPoses()) {
		g.statusMsg = "Select a registered pose to delete"
		return
	}
	id := g.selectedPose
	editor.RemovePoseFromAnimationClips(&g.model.Animation, id)
	editor.RemovePoseFromTileLinks(&g.model.TileLinks, id)
	g.model.Poses.Poses = append(g.model.Poses.Poses[:id], g.model.Poses.Poses[id+1:]...)
	if g.selectedPose >= len(g.model.Poses.Poses) {
		g.selectedPose = len(g.model.Poses.Poses) - 1
	}
	if g.selectedPose >= 0 {
		g.selectPose(g.selectedPose)
	}
	g.commitPoseEdit(fmt.Sprintf("Deleted pose %d", id))
}

func (g *Game) togglePoseTile(tile int) {
	if tile < 0 || tile >= g.spriteN() {
		return
	}
	if g.selectedPose < 0 || g.selectedPose >= len(g.authoredPoses()) {
		if len(g.authoredPoses()) >= assetsync.MaxPoseCount {
			g.statusMsg = fmt.Sprintf("Already have %d registered poses", assetsync.MaxPoseCount)
			return
		}
		if g.model.Poses.Version == 0 {
			g.model.Poses.Version = 1
		}
		pose := assetsync.PoseRecord{
			Name:  editor.NextPoseName(g.authoredPoses(), "idle"),
			Parts: []assetsync.PosePartRecord{{Tile: tile, X: 0, Y: 0}},
		}
		g.model.Poses.Poses = append(g.model.Poses.Poses, pose)
		g.selectedPose = len(g.model.Poses.Poses) - 1
		g.commitPoseEdit(fmt.Sprintf("pspr %s tile %d", editor.PoseFrameLabel(g.selectedPose, pose), tile))
		return
	}
	pose, err := editor.TogglePosePart(g.model.Poses.Poses[g.selectedPose], tile, g.spriteCols())
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.model.Poses.Poses[g.selectedPose] = pose
	g.commitPoseEdit(fmt.Sprintf("pspr %s tile %d", editor.PoseFrameLabel(g.selectedPose, pose), tile))
}

func (g *Game) poseVisibleLabel(id int, pose assetsync.PoseRecord) string {
	if g.poseNameEditing && g.selectedPose == id {
		return editor.PoseFrameLabel(id, assetsync.PoseRecord{Name: g.poseNameDraft + "_"})
	}
	return editor.PoseFrameLabel(id, pose)
}

func (g *Game) poseStripLabelRect(id int) editor.Rect {
	if g.objEditing {
		return g.animationFrameLabelRect(id)
	}
	cell := g.poseStripCellRect(id)
	if cell.W <= 0 {
		return editor.Rect{}
	}
	poses := g.authoredPoses()
	if id < 0 || id >= len(poses) {
		return editor.Rect{}
	}
	label := g.poseVisibleLabel(id, poses[id])
	labelY := cell.Y + poseStripSlotPad + poseStripPreviewH + 4
	labelX := cell.X + (cell.W-smallTextWidth(label))/2
	track := g.poseStripTrackRect()
	if labelX < track.X {
		labelX = track.X + 2
	}
	return editor.Rect{X: labelX, Y: labelY, W: smallTextWidth(label), H: uiSmallTextHeight}
}

func (g *Game) beginPoseNameEdit() {
	if g.selectedPose < 0 || g.selectedPose >= len(g.authoredPoses()) {
		g.statusMsg = "Select a registered pose to rename"
		return
	}
	g.poseNameEditing = true
	g.poseNameDraft = g.authoredPoses()[g.selectedPose].Name
	g.statusMsg = "Type a name like idle_1, Enter to save, Esc to cancel"
	g.requestRender()
}

func (g *Game) poseNameEditClickKeepsFocus(mx, my int) bool {
	if !g.poseNameEditing {
		return false
	}
	return rectContains(g.poseStripLabelRect(g.selectedPose), mx, my)
}

func (g *Game) updatePoseNameTyping() bool {
	if !g.poseNameEditing {
		return false
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if unicode.IsControl(r) {
			continue
		}
		next := g.poseNameDraft + string(r)
		if len(next) > assetsync.MaxPoseNameBytes {
			continue
		}
		g.poseNameDraft = next
		g.requestRender()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && g.poseNameDraft != "" {
		_, size := utf8.DecodeLastRuneInString(g.poseNameDraft)
		g.poseNameDraft = g.poseNameDraft[:len(g.poseNameDraft)-size]
		g.requestRender()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.cancelPoseNameEdit()
		return false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		g.commitPoseNameEdit()
		return false
	}
	return true
}

func (g *Game) cancelPoseNameEdit() {
	g.poseNameEditing = false
	g.poseNameDraft = ""
	g.statusMsg = ""
	g.requestRender()
}

func (g *Game) commitPoseNameEdit() {
	if !g.poseNameEditing {
		return
	}
	id := g.selectedPose
	draft := g.poseNameDraft
	g.poseNameEditing = false
	g.poseNameDraft = ""
	if id < 0 || id >= len(g.authoredPoses()) {
		return
	}
	current := g.authoredPoses()[id].Name
	normalized, err := editor.NormalizePoseName(draft)
	if err != nil {
		g.statusMsg = err.Error()
		g.poseNameEditing = true
		g.poseNameDraft = draft
		return
	}
	if normalized == current {
		g.statusMsg = ""
		g.requestRender()
		return
	}
	if err := editor.RenamePose(&g.model.Poses, id, draft); err != nil {
		g.statusMsg = err.Error()
		g.poseNameEditing = true
		g.poseNameDraft = draft
		return
	}
	g.commitPoseEdit(fmt.Sprintf("pspr %s", editor.PoseFrameLabel(id, g.authoredPoses()[id])))
}

func (g *Game) commitPoseEdit(status string) {
	g.objUsage = nil
	g.invalidateAnimationPreview()
	g.dirty = true
	g.undo.forcePush(g.snapshot())
	g.statusMsg = status
	g.requestRender()
}

func (g *Game) drawPoseSheetOverlays(screen *ebiten.Image, canvas editor.Rect) {
	if screen == nil || !g.showsPoseStrip() {
		return
	}
	cols, rows := g.spriteCols(), g.spriteRows()
	poses := g.authoredPoses()
	clip := g.sheetContentClip(canvas)
	for index, pose := range poses {
		if g.selectedPose != index {
			continue
		}
		accent := poseOverlayColor(index)
		col, row, width, height, ok := editor.PoseSheetBounds(pose, cols, rows)
		if !ok {
			continue
		}
		region := sheetCameraRect(g.sheetCam, canvas, col, row, width, height)
		outline, visible := intersectEditorRects(region, clip)
		if !visible {
			continue
		}
		strokeRect(screen, outline, rgba(accent, 230))
	}
}

func (g *Game) drawPoseStrip(screen *ebiten.Image) {
	if g.objEditing {
		g.drawAnimationDock(screen)
		return
	}
	if screen == nil || !g.showsPoseStrip() {
		return
	}
	strip := g.poseStripRect()
	fillRoundedRect(screen, strip, 5, colorInk)
	strokeRoundedRect(screen, strip, 5, rgba(colorBorder, 180))
	g.clampPoseStripScroll()
	track := g.poseStripTrackRect()
	if len(g.authoredPoses()) == 0 {
		drawSmallTextCentered(screen, "SELECT ARTWORK, THEN REGISTER POSE", track, colorDim)
	}
	x := track.X - g.poseStripScroll
	for id, pose := range g.authoredPoses() {
		cell := editor.Rect{X: x, Y: track.Y, W: g.poseStripSlotWidth(id), H: track.H}
		x += cell.W
		if cell.X >= track.X+track.W {
			break
		}
		visible, ok := intersectEditorRects(cell, track)
		if !ok {
			continue
		}
		accent := poseOverlayColor(id)
		if g.selectedPose == id {
			fillRect(screen, visible, rgba(accent, 28))
			strokeRect(screen, visible, rgba(accent, 220))
		}
		preview := g.posePreview(id)
		dw, dh := posePreviewFit(preview.Width, preview.Height)
		px := cell.X + (cell.W-dw)/2
		py := cell.Y + poseStripSlotPad
		previewRect := editor.Rect{X: px, Y: py, W: dw, H: dh}
		if dest, destOK := intersectEditorRects(previewRect, track); destOK {
			drawCheckerboardColors(screen, dest, colorSheetCheckerA, colorSheetCheckerB)
			g.drawPosePreview(screen, preview, float64(px), float64(py), float64(dw), float64(dh), track)
		}
		label := g.poseVisibleLabel(id, pose)
		labelY := cell.Y + poseStripSlotPad + poseStripPreviewH + 4
		labelX := cell.X + (cell.W-smallTextWidth(label))/2
		if labelX < track.X {
			labelX = track.X + 2
		}
		if labelY+uiSmallTextHeight <= track.Y+track.H {
			drawSmallText(screen, label, labelX, labelY, accent)
		}
	}
	drawTactileButton(screen, g.poseStripNewRect(), "NEW", false, colorSprite, true)
	drawTactileButton(screen, g.poseStripSetRect(), "SET", false, colorSprite, true)
	drawTactileButton(screen, g.poseStripDelRect(), "DEL", false, colorDanger, true)
	drawTactileButton(screen, g.poseStripCopyRect(), "COPY LUA", false, colorSprite, g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()))
}

func (g *Game) drawPosePreview(screen *ebiten.Image, preview editor.PosePreview, originX, originY, destW, destH float64, clip editor.Rect) {
	if preview.Width <= 0 || preview.Height <= 0 || destW <= 0 || destH <= 0 {
		return
	}
	scaleX := destW / float64(preview.Width)
	scaleY := destH / float64(preview.Height)
	for py := 0; py < preview.Height; py++ {
		for px := 0; px < preview.Width; px++ {
			pixel := preview.Pixels[py*preview.Width+px]
			if isEraserIndex(pixel) {
				continue
			}
			x, w := scaledRun(originX, px, scaleX)
			y, h := scaledRun(originY, py, scaleY)
			target, visible := intersectEditorRects(editor.Rect{X: x, Y: y, W: w, H: h}, clip)
			if visible {
				g.fillColor(screen, target, pixel)
			}
		}
	}
}

func (g *Game) drawPoseBankHint(screen *ebiten.Image) {
	n := len(g.authoredPoses())
	if n == 0 || g.poseBankIsActive() {
		return
	}
	bank := "POSES"
	if g.model == nil || g.model.ObjSheet == nil {
		bank = "SHARED TILES"
	}
	label := fmt.Sprintf("%d POSES ON %s", n, bank)
	bg, _ := spriteBankToggleRects()
	x := bg.X - smallTextWidth(label) - 16
	if x < spriteHeaderRect.X+650 {
		return
	}
	drawSmallText(screen, label, x, bg.Y+(bg.H-uiSmallTextHeight)/2, colorHighlight)
}
