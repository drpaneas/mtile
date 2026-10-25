package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

var writeAnimationUsageClipboard = hostclipboard.Write

const animationEntryHeight = 44
const animationPickerRowHeight = 36

type animationWorkspaceLayout struct {
	Panel, ClipPrev, ClipName, ClipNext, NewClip, RenameClip, DeleteClip, SaveClip         editor.Rect
	PosePicker, AddPose, Order, RemovePose, MoveUp, MoveDown, OrderPrev, OrderNext         editor.Rect
	Preview, Play, Step, FPSMinus, FPS, FPSPlus, Loop, LoopHint, Usage, CopyUsage, Message editor.Rect
	PickerPopup, PickerSearch, PickerList, PickerPrev, PickerNext                          editor.Rect
}

func newAnimationWorkspaceLayout(panel editor.Rect) animationWorkspaceLayout {
	rect := func(x, y, w, h int) editor.Rect { return editor.Rect{X: panel.X + x, Y: panel.Y + y, W: w, H: h} }
	selectorW := max(180, panel.W-640)
	rightW := min(340, max(260, panel.W/3))
	leftW := panel.W - rightW - 48
	rightX := panel.W - rightW - 16
	previewH := min(320, max(120, panel.H-416))
	l := animationWorkspaceLayout{
		Panel:    panel,
		ClipPrev: rect(16, 56, 28, 36), ClipName: rect(50, 56, selectorW-68, 36), ClipNext: rect(16+selectorW-28, 56, 28, 36),
		NewClip: rect(selectorW+32, 56, 108, 36), RenameClip: rect(selectorW+148, 56, 88, 36), DeleteClip: rect(selectorW+244, 56, 108, 36), SaveClip: rect(panel.W-160, 56, 144, 36),
		PosePicker: rect(16, 144, leftW-116, 36), AddPose: rect(16+leftW-108, 144, 108, 36),
		Order:      rect(16, 224, leftW, max(44, panel.H-368)),
		RemovePose: rect(16, panel.H-132, 116, 32), MoveUp: rect(140, panel.H-132, 96, 32), MoveDown: rect(244, panel.H-132, 108, 32),
		OrderPrev: rect(16+leftW-72, panel.H-132, 32, 32), OrderNext: rect(16+leftW-32, panel.H-132, 32, 32),
		Preview: rect(rightX, 144, rightW, previewH), Play: rect(rightX, 156+previewH, (rightW-8)/2, 32), Step: rect(rightX+(rightW+8)/2, 156+previewH, (rightW-8)/2, 32),
		FPSMinus: rect(rightX, 200+previewH, 32, 32), FPS: rect(rightX+40, 200+previewH, rightW-80, 32), FPSPlus: rect(rightX+rightW-32, 200+previewH, 32, 32), Loop: rect(rightX, 244+previewH, rightW, 32),
		LoopHint: rect(rightX, 284+previewH, rightW, 36),
		Message:  rect(16, panel.H-88, panel.W-32, 24), Usage: rect(16, panel.H-56, panel.W-152, 36), CopyUsage: rect(panel.W-128, panel.H-56, 112, 36),
	}
	l.PickerPopup = rect(16, 184, leftW-116, min(320, panel.H-208))
	p := l.PickerPopup
	l.PickerSearch = editor.Rect{X: p.X + 8, Y: p.Y + 8, W: p.W - 16, H: 32}
	l.PickerList = editor.Rect{X: p.X + 8, Y: p.Y + 48, W: p.W - 16, H: p.H - 96}
	l.PickerPrev = editor.Rect{X: p.X + 8, Y: p.Y + p.H - 36, W: 64, H: 28}
	l.PickerNext = editor.Rect{X: p.X + p.W - 72, Y: p.Y + p.H - 36, W: 64, H: 28}
	return l
}

func (g *Game) animationLoops() bool {
	clip := g.activeAnimationClip()
	return clip == nil || clip.Loop == nil || *clip.Loop
}

func (g *Game) toggleAnimationLoop() {
	if clip := g.activeAnimationClip(); clip != nil {
		loop := !g.animationLoops()
		clip.Loop = &loop
		g.animation.finished = false
		g.commitAnimationEdit("Updated animation looping")
	}
}

func (g *Game) animationUsage() string {
	clip := g.activeAnimationClip()
	if clip == nil {
		return ""
	}
	name := strings.ReplaceAll(strings.ReplaceAll(clip.Name, "\\", "\\\\"), "\"", "\\\"")
	if !g.animationLoops() {
		return "anim(\"" + name + "\", x, y, false, false, time() - started)"
	}
	return "anim(\"" + name + "\", x, y)"
}

func (g *Game) markAnimationClipsSaved() { clear(g.animation.newClips) }

func (g *Game) registerAnimationClip() bool {
	if g.model == nil {
		return false
	}
	defer func() { g.statusMsg = "" }()
	if g.animation.nameEditing && !g.commitAnimationNameEdit() {
		return false
	}
	if clip := g.activeAnimationClip(); clip != nil && len(clip.Frames) == 0 {
		g.animation.message = "Add at least one registered pose before registering this clip."
		g.animation.messageError = true
		g.requestRender()
		return false
	}
	if err := g.model.Save(); err != nil {
		g.animation.message = "Could not save clips: " + err.Error()
		g.animation.messageError = true
		g.requestRender()
		return false
	}
	g.markAnimationClipsSaved()
	g.animation.message = "Animation clips saved. Build the game to use them from Lua."
	g.animation.messageError = false
	g.requestRender()
	return true
}

func (g *Game) deleteAnimationClip() {
	clip := g.activeAnimationClip()
	if clip == nil {
		return
	}
	name, index := clip.Name, g.animation.clip-1
	g.model.Animation.Clips = append(g.model.Animation.Clips[:index], g.model.Animation.Clips[index+1:]...)
	editor.RemoveAnimationFromTileLinks(&g.model.TileLinks, name)
	delete(g.animation.newClips, name)
	g.selectAnimationClip(min(index+1, len(g.model.Animation.Clips)))
	g.commitAnimationEdit("Deleted clip " + name + ". Registered poses are unchanged; Undo restores the clip.")
}

func (g *Game) animationRegisteredPoseIDs() []int {
	query := strings.ToLower(strings.TrimSpace(g.animation.pickerSearch))
	if s := &g.animation; s.pickerMatchesValid && s.pickerMatchQuery == query && s.pickerMatchCount == len(g.authoredPoses()) {
		return s.pickerMatches
	}
	var ids []int
	for id, pose := range g.authoredPoses() {
		if query == "" || strings.Contains(strings.ToLower(pose.Name), query) || strings.Contains(strconv.Itoa(id), query) {
			ids = append(ids, id)
		}
	}
	g.animation.pickerMatches, g.animation.pickerMatchQuery, g.animation.pickerMatchCount, g.animation.pickerMatchesValid = ids, query, len(g.authoredPoses()), true
	return ids
}

func (g *Game) clampAnimationWorkspace(layout animationWorkspaceLayout) {
	rows := max(1, layout.Order.H/animationEntryHeight)
	g.animation.timelineScroll = min(max(0, g.animation.timelineScroll), max(0, len(g.animationFrames())-rows))
	if g.animation.pickerOpen {
		rows = max(1, layout.PickerList.H/animationPickerRowHeight)
		count := len(g.animationRegisteredPoseIDs())
		g.animation.pickerScroll = min(max(0, g.animation.pickerScroll), max(0, count-rows))
		g.animation.pickerCursor = min(max(0, g.animation.pickerCursor), max(0, count-1))
	}
}

func (g *Game) openAnimationPosePicker() {
	g.animation.pickerOpen = true
	g.animation.pickerSearch = ""
	g.animation.searchSelectAll = false
	g.animation.pickerScroll, g.animation.pickerCursor = 0, 0
	g.requestRender()
}

func (g *Game) selectAnimationEntry(index int) {
	if index < 0 || index >= len(g.animationFrames()) {
		return
	}
	g.animation.cursor, g.animation.playhead = index, index
	g.animation.playing, g.animation.stepped, g.animation.finished = false, true, false
	g.requestRender()
}

func (g *Game) clickAnimationWorkspace(layout animationWorkspaceLayout, mx, my int) {
	s := &g.animation
	if s.pickerOpen {
		switch {
		case rectContains(layout.PickerPrev, mx, my):
			s.pickerScroll -= max(1, layout.PickerList.H/animationPickerRowHeight)
			s.pickerCursor = max(0, s.pickerScroll)
		case rectContains(layout.PickerNext, mx, my):
			s.pickerScroll += max(1, layout.PickerList.H/animationPickerRowHeight)
			s.pickerCursor = s.pickerScroll
		case rectContains(layout.PickerSearch, mx, my):
			s.searchSelectAll = true
		case rectContains(layout.PickerList, mx, my):
			ids := g.animationRegisteredPoseIDs()
			index := s.pickerScroll + (my-layout.PickerList.Y)/animationPickerRowHeight
			if index >= 0 && index < len(ids) {
				s.pickerPose = ids[index]
				s.pickerOpen = false
			}
		case !rectContains(layout.PickerPopup, mx, my):
			s.pickerOpen = false
		}
		g.clampAnimationWorkspace(layout)
		g.requestRender()
		return
	}
	switch {
	case rectContains(layout.ClipPrev, mx, my):
		g.selectAnimationClip(s.clip - 1)
	case rectContains(layout.ClipNext, mx, my):
		g.selectAnimationClip(s.clip + 1)
	case rectContains(layout.NewClip, mx, my):
		g.createAnimationClip()
	case rectContains(layout.ClipName, mx, my), rectContains(layout.RenameClip, mx, my):
		g.beginAnimationNameEdit()
	case rectContains(layout.DeleteClip, mx, my):
		g.deleteAnimationClip()
	case rectContains(layout.SaveClip, mx, my):
		g.registerAnimationClip()
	case rectContains(layout.PosePicker, mx, my):
		g.openAnimationPosePicker()
	case rectContains(layout.AddPose, mx, my):
		if g.activeAnimationClip() == nil {
			s.message = "Create a clip, then choose registered poses to add."
			s.messageError = true
		} else {
			g.editAnimationClipFrame("add")
		}
	case rectContains(layout.RemovePose, mx, my):
		g.editAnimationClipFrame("remove")
	case rectContains(layout.MoveUp, mx, my):
		g.editAnimationClipFrame("left")
	case rectContains(layout.MoveDown, mx, my):
		g.editAnimationClipFrame("right")
	case rectContains(layout.OrderPrev, mx, my):
		s.timelineScroll--
	case rectContains(layout.OrderNext, mx, my):
		s.timelineScroll++
	case rectContains(layout.Order, mx, my):
		g.selectAnimationEntry(s.timelineScroll + (my-layout.Order.Y)/animationEntryHeight)
	case rectContains(layout.Play, mx, my):
		if s.playing {
			s.playing, s.stepped = false, true
		} else {
			g.startAnimationPreview()
		}
	case rectContains(layout.Step, mx, my):
		g.stepAnimationPreview()
	case rectContains(layout.FPSMinus, mx, my):
		g.changeAnimationFPS(-1)
	case rectContains(layout.FPSPlus, mx, my):
		g.changeAnimationFPS(1)
	case rectContains(layout.Loop, mx, my):
		g.toggleAnimationLoop()
	case rectContains(layout.CopyUsage, mx, my):
		if usage := g.animationUsage(); usage != "" {
			if err := writeAnimationUsageClipboard(usage); err != nil {
				s.message = err.Error()
				s.messageError = true
			} else {
				s.message = "Copied " + usage
				s.messageError = false
			}
		}
	}
	g.clampAnimationWorkspace(layout)
	g.requestRender()
}

func (g *Game) updateAnimationWorkspace(mx, my int) {
	if !g.animationWorkspaceActive() || g.model == nil {
		return
	}
	layout := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
	if _, wheel := ebiten.Wheel(); wheel != 0 {
		if g.animation.pickerOpen && rectContains(layout.PickerPopup, mx, my) {
			g.animation.pickerScroll -= int(wheel)
			g.animation.pickerCursor = max(0, g.animation.pickerScroll)
		} else if !g.animation.pickerOpen && rectContains(layout.Order, mx, my) {
			g.animation.timelineScroll -= int(wheel)
		}
		g.clampAnimationWorkspace(layout)
		g.requestRender()
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.clickAnimationWorkspace(layout, mx, my)
	}
}

// A dropdown search owns typing and mouse input until it closes, so letters
// cannot accidentally invoke canvas shortcuts or change the selected editor.
func (g *Game) updateAnimationWorkspaceTyping() bool {
	if !g.animationWorkspaceActive() || !g.animation.pickerOpen {
		return false
	}
	s := &g.animation
	beforeQuery, beforeScroll, beforeCursor, beforeSelect, beforeOpen := s.pickerSearch, s.pickerScroll, s.pickerCursor, s.searchSelectAll, s.pickerOpen
	g.updateAnimationWorkspace(ebiten.CursorPosition())
	if !s.pickerOpen {
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.pickerOpen = false
		g.requestRender()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		ids := g.animationRegisteredPoseIDs()
		if s.pickerCursor < len(ids) {
			s.pickerPose = ids[s.pickerCursor]
			s.pickerOpen = false
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		s.pickerCursor++
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		s.pickerCursor--
	}
	modifier := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if modifier && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		s.searchSelectAll = true
	}
	if !modifier {
		for _, r := range ebiten.AppendInputChars(nil) {
			if unicode.IsControl(r) {
				continue
			}
			if s.searchSelectAll {
				s.pickerSearch = ""
				s.searchSelectAll = false
			}
			s.pickerSearch += string(r)
			s.pickerScroll, s.pickerCursor = 0, 0
		}
	}
	if codeKeyRepeat(ebiten.KeyBackspace) {
		if s.searchSelectAll {
			s.pickerSearch = ""
		} else if s.pickerSearch != "" {
			_, size := utf8.DecodeLastRuneInString(s.pickerSearch)
			s.pickerSearch = s.pickerSearch[:len(s.pickerSearch)-size]
		}
		s.searchSelectAll = false
		s.pickerScroll, s.pickerCursor = 0, 0
	}
	layout := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
	if s.pickerCursor < s.pickerScroll {
		s.pickerScroll = s.pickerCursor
	}
	if visible := max(1, layout.PickerList.H/animationPickerRowHeight); s.pickerCursor >= s.pickerScroll+visible {
		s.pickerScroll = s.pickerCursor - visible + 1
	}
	g.clampAnimationWorkspace(layout)
	if beforeQuery != s.pickerSearch || beforeScroll != s.pickerScroll || beforeCursor != s.pickerCursor || beforeSelect != s.searchSelectAll || beforeOpen != s.pickerOpen {
		g.requestRender()
	}
	return true
}

func (g *Game) drawAnimationWorkspace(screen *ebiten.Image) {
	if !g.animationWorkspaceActive() || g.model == nil || g.activeSheet() == nil {
		return
	}
	l := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
	g.clampAnimationWorkspace(l)
	drawSpriteWorkbenchPanel(screen, l.Panel)
	drawTextScale(screen, "ANIMATION CLIPS", l.Panel.X+16, l.Panel.Y+12, 2, colorText)
	name := "No clips yet"
	clip := g.activeAnimationClip()
	if clip != nil {
		name = clip.Name
	}
	if g.animation.nameEditing {
		name = g.animation.nameDraft + "_"
	}
	drawTactileButton(screen, l.ClipName, truncateUIText(name, l.ClipName.W-16), g.animation.nameEditing, colorSprite, false)
	saveLabel := "SAVE CLIPS"
	if clip != nil {
		saveLabel = "SAVE CLIP"
		if g.animation.newClips[clip.Name] {
			saveLabel = "REGISTER CLIP"
		}
	}
	for _, b := range []struct {
		rect  editor.Rect
		label string
	}{
		{l.ClipPrev, "<"}, {l.ClipNext, ">"}, {l.NewClip, "NEW CLIP"}, {l.RenameClip, "RENAME"}, {l.DeleteClip, "DELETE CLIP"}, {l.SaveClip, saveLabel},
		{l.AddPose, "ADD POSE"}, {l.RemovePose, "REMOVE POSE"}, {l.MoveUp, "MOVE UP"}, {l.MoveDown, "MOVE DOWN"}, {l.OrderPrev, "<"}, {l.OrderNext, ">"}, {l.CopyUsage, "COPY CODE"},
	} {
		drawTactileButton(screen, b.rect, b.label, b.rect == l.SaveClip, colorSprite, true)
	}
	drawText(screen, "REGISTERED POSES", l.PosePicker.X, l.PosePicker.Y-24, colorText)
	poseLabel := "Choose a pose from any page"
	if id := g.animation.pickerPose; id >= 0 && id < len(g.authoredPoses()) {
		poseLabel = editor.PoseFrameLabel(id, g.authoredPoses()[id])
	}
	drawTactileButton(screen, l.PosePicker, truncateUIText(poseLabel, l.PosePicker.W-36), g.animation.pickerOpen, colorSprite, false)
	drawSmallText(screen, "v", l.PosePicker.X+l.PosePicker.W-18, l.PosePicker.Y+10, colorDim)
	frames := g.animationFrames()
	drawText(screen, fmt.Sprintf("PLAYBACK ORDER · %d POSES", len(frames)), l.Order.X, l.Order.Y-26, colorText)
	fillRect(screen, l.Order, colorInk)
	strokeRect(screen, l.Order, colorBorder)
	if len(frames) == 0 {
		message := "Choose a registered pose above, then ADD POSE."
		if clip == nil {
			message = "Create a clip to start arranging poses."
		}
		drawSmallTextCentered(screen, message, l.Order, colorDim)
	}
	for at := g.animation.timelineScroll; at < len(frames); at++ {
		y := l.Order.Y + (at-g.animation.timelineScroll)*animationEntryHeight
		if y+animationEntryHeight > l.Order.Y+l.Order.H {
			break
		}
		row := editor.Rect{X: l.Order.X + 1, Y: y + 1, W: l.Order.W - 2, H: animationEntryHeight - 2}
		labelColor, indexColor := colorText, colorDim
		if at == g.animation.cursor {
			fillRect(screen, row, colorSprite)
			strokeRect(screen, row, colorSprite)
			labelColor, indexColor = colorInk, colorInk
		}
		if g.animation.playing && at == g.animation.playhead {
			strokeRect(screen, row, colorMusic)
		}
		id := frames[at]
		if id < 0 || id >= len(g.authoredPoses()) {
			continue
		}
		thumb := editor.Rect{X: row.X + 44, Y: row.Y + 3, W: 36, H: 36}
		drawCheckerboardColors(screen, thumb, colorSheetCheckerA, colorSheetCheckerB)
		g.drawAnimationFrame(screen, g.animationPoseThumbnail(id), thumb)
		drawSmallTextCentered(screen, strconv.Itoa(at+1), editor.Rect{X: row.X + 4, Y: row.Y, W: 32, H: row.H}, indexColor)
		drawText(screen, truncateUIText(editor.PoseFrameLabel(id, g.authoredPoses()[id]), row.W-100), row.X+92, row.Y+(row.H-uiTextHeight)/2, labelColor)
	}
	previewID := g.animationPreviewPose()
	g.prepareAnimationPreview(frames, previewID)
	previewLabel := "PREVIEW"
	if bounds := g.animation.bounds; bounds.W > 0 && bounds.H > 0 {
		previewLabel = fmt.Sprintf("PREVIEW · %d × %d PX", bounds.W, bounds.H)
	}
	drawText(screen, truncateUIText(previewLabel, l.Preview.W), l.Preview.X, l.Preview.Y-24, colorText)
	drawCheckerboardColors(screen, l.Preview, colorSheetCheckerA, colorSheetCheckerB)
	if previewID >= 0 && previewID < len(g.authoredPoses()) {
		preview, ok := g.animation.previews[previewID]
		if !ok {
			if len(g.animation.previews) >= 8 {
				clear(g.animation.previews)
			}
			preview = editor.ComposeAnimationFrame(*g.activeSheet(), g.authoredPoses()[previewID], g.animation.bounds)
			g.animation.previews[previewID] = preview
		}
		g.drawAnimationFrame(screen, preview, l.Preview)
	}
	playLabel := "PLAY"
	if g.animation.playing {
		playLabel = "PAUSE"
	}
	drawTactileButton(screen, l.Play, playLabel, g.animation.playing, colorSprite, true)
	drawTactileButton(screen, l.Step, "STEP", false, colorSprite, true)
	drawTactileButton(screen, l.FPSMinus, "-", false, colorSprite, true)
	drawTactileButton(screen, l.FPSPlus, "+", false, colorSprite, true)
	drawTextCentered(screen, fmt.Sprintf("%d FPS", g.animationFPS()), l.FPS, colorText)
	loopLabel := "LOOP: OFF · PLAY ONCE"
	if g.animationLoops() {
		loopLabel = "LOOP: ON"
	}
	drawTactileButton(screen, l.Loop, loopLabel, g.animationLoops(), colorSprite, true)
	if !g.animationLoops() {
		drawSmallText(screen, "Set started = time()", l.LoopHint.X, l.LoopHint.Y, colorDim)
		drawSmallText(screen, "when the action begins.", l.LoopHint.X, l.LoopHint.Y+uiSmallTextHeight+3, colorDim)
	} else if len(frames) > 0 {
		drawSmallText(screen, "Area calculated from every pose.", l.LoopHint.X, l.LoopHint.Y, colorDim)
		drawSmallText(screen, "Flip keeps this area in place.", l.LoopHint.X, l.LoopHint.Y+uiSmallTextHeight+3, colorDim)
	}
	message := g.animation.message
	if message == "" {
		message = "Save Clip writes changes for your next game build."
		if clip == nil {
			message = "Create a clip, then choose registered poses to add."
		} else if g.animation.newClips[clip.Name] {
			message = "Register the clip to save it for your next game build."
		}
	}
	messageColor := colorDim
	if g.animation.messageError {
		messageColor = colorDanger
	}
	drawSmallText(screen, truncateTextToWidth(message, l.Message.W, uiSmallFace), l.Message.X, l.Message.Y+4, messageColor)
	fillRect(screen, l.Usage, colorInk)
	strokeRect(screen, l.Usage, colorBorder)
	usage := g.animationUsage()
	if usage == "" {
		usage = "Create a named clip to see its Lua call."
	}
	drawText(screen, truncateUIText(usage, l.Usage.W-16), l.Usage.X+8, l.Usage.Y+(l.Usage.H-uiTextHeight)/2, colorLCD)
	if g.animation.pickerOpen {
		g.drawAnimationPosePicker(screen, l)
	}
}

func (g *Game) drawAnimationPosePicker(screen *ebiten.Image, l animationWorkspaceLayout) {
	fillRect(screen, l.PickerPopup, colorButtonFace)
	strokeRect(screen, l.PickerPopup, colorSprite)
	fillRect(screen, l.PickerSearch, colorBackground)
	strokeRect(screen, l.PickerSearch, colorSprite)
	search := g.animation.pickerSearch + "_"
	if g.animation.pickerSearch == "" {
		search = "Search registered poses..."
	}
	drawText(screen, truncateUIText(search, l.PickerSearch.W-16), l.PickerSearch.X+8, l.PickerSearch.Y+8, colorText)
	ids := g.animationRegisteredPoseIDs()
	if len(ids) == 0 {
		label := "No matching poses"
		if len(g.authoredPoses()) == 0 {
			label = "Register poses in the POSES tab first."
		}
		drawSmallTextCentered(screen, label, l.PickerList, colorDim)
	}
	for at := g.animation.pickerScroll; at < len(ids); at++ {
		y := l.PickerList.Y + (at-g.animation.pickerScroll)*animationPickerRowHeight
		if y+animationPickerRowHeight > l.PickerList.Y+l.PickerList.H {
			break
		}
		row := editor.Rect{X: l.PickerList.X, Y: y, W: l.PickerList.W, H: animationPickerRowHeight}
		id := ids[at]
		labelColor := colorText
		if at == g.animation.pickerCursor {
			fillRect(screen, row, colorSprite)
			labelColor = colorInk
		}
		drawText(screen, truncateUIText(editor.PoseFrameLabel(id, g.authoredPoses()[id]), row.W-16), row.X+8, row.Y+(row.H-uiTextHeight)/2, labelColor)
	}
	drawTactileButton(screen, l.PickerPrev, "PREV", false, colorSprite, true)
	drawTactileButton(screen, l.PickerNext, "NEXT", false, colorSprite, true)
	drawSmallTextCentered(screen, fmt.Sprintf("%d registered poses", len(ids)), editor.Rect{X: l.PickerPrev.X + l.PickerPrev.W, Y: l.PickerPrev.Y, W: l.PickerNext.X - l.PickerPrev.X - l.PickerPrev.W, H: l.PickerPrev.H}, colorDim)
}
