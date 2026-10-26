package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Authored clips occupy 1..N; zero means no clip is selected. Pose registration
// and clip assembly have independent selections and share only global pose IDs.
type animationEditorState struct {
	clip, fps, cursor, playhead, scroll    int
	playing, stepped                       bool
	workspace, finished                    bool
	lastTick                               time.Time
	observedPose                           int
	nameEditing, nameSelectAll             bool
	nameDraft                              string
	pageFrames                             []int
	pageFramesValid                        bool
	cachedFrames                           []int
	bounds                                 editor.Rect
	previews, thumbs                       map[int]editor.PosePreview
	pickerOpen, searchSelectAll            bool
	pickerPose, pickerScroll, pickerCursor int
	pickerSearch                           string
	pickerMatches                          []int
	pickerMatchQuery                       string
	pickerMatchCount                       int
	pickerMatchesValid                     bool
	timelineScroll                         int
	newClips                               map[string]bool
	message                                string
	messageError                           bool
}

type animationDockLayout struct {
	Panel, Track, Preview                                                  editor.Rect
	RegisterPose, UpdatePose, DeletePose, CopyPose, ScrollPrev, ScrollNext editor.Rect
}

const animationFrameCellWidth = 84

func newAnimationDockLayout(panel editor.Rect) animationDockLayout {
	button := func(x, w int) editor.Rect { return editor.Rect{X: panel.X + x, Y: panel.Y + panel.H - 44, W: w, H: 32} }
	return animationDockLayout{
		Panel:        panel,
		Track:        editor.Rect{X: panel.X + 8, Y: panel.Y + 36, W: panel.W - 172, H: panel.H - 92},
		Preview:      editor.Rect{X: panel.X + panel.W - 152, Y: panel.Y + 36, W: 144, H: panel.H - 92},
		RegisterPose: button(8, 128), UpdatePose: button(144, 116), DeletePose: button(268, 116),
		CopyPose:   editor.Rect{X: panel.X + panel.W - 292, Y: panel.Y + 4, W: 132, H: 28},
		ScrollPrev: button(panel.W-72, 28), ScrollNext: button(panel.W-36, 28),
	}
}

func (g *Game) animationWorkspaceActive() bool { return g.objEditing && g.animation.workspace }

func (g *Game) setAnimationWorkspaceActive(active bool) {
	g.animation.workspace = active
	g.animation.playing, g.animation.stepped, g.animation.pickerOpen = false, false, false
	g.animation.nameEditing = false
	g.cancelPoseNameEdit()
	if active && g.model != nil && g.activeAnimationClip() == nil && len(g.model.Animation.Clips) > 0 {
		g.selectAnimationClip(1)
	}
	if active && g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()) {
		g.animation.pickerPose = g.selectedPose
	}
	g.invalidateAnimationPreview()
}

func (g *Game) pageAnimationPoses() []int {
	if !g.animation.pageFramesValid {
		g.animation.pageFrames = g.visiblePagePoseIDs()
		g.animation.pageFramesValid = true
	}
	return g.animation.pageFrames
}

func (g *Game) animationFrames() []int {
	if g.model == nil {
		return nil
	}
	if g.animation.workspace && g.animation.clip > 0 && g.animation.clip <= len(g.model.Animation.Clips) {
		return g.model.Animation.Clips[g.animation.clip-1].Frames
	}
	if g.animation.workspace {
		return nil
	}
	return g.pageAnimationPoses()
}

func (g *Game) activeAnimationClip() *assetsync.AnimationClip {
	if g.model == nil || g.animation.clip <= 0 || g.animation.clip > len(g.model.Animation.Clips) {
		return nil
	}
	return &g.model.Animation.Clips[g.animation.clip-1]
}

func (g *Game) animationFPS() int {
	if clip := g.activeAnimationClip(); clip != nil {
		return min(60, max(1, clip.FPS))
	}
	if g.animation.fps == 0 {
		return 8
	}
	return min(60, max(1, g.animation.fps))
}

func (g *Game) invalidateAnimationPreview() {
	g.animation.cachedFrames = nil
	g.animation.previews = nil
	g.animation.thumbs = nil
	g.animation.pageFrames, g.animation.pageFramesValid = nil, false
	g.animation.lastTick = time.Time{}
	g.animation.pickerOpen = false
	g.animation.pickerMatches, g.animation.pickerMatchesValid = nil, false
	if g.model != nil {
		g.animation.clip = min(max(0, g.animation.clip), len(g.model.Animation.Clips))
		if g.activeAnimationClip() == nil {
			g.animation.nameEditing = false
			g.animation.playing = false
		}
		if n := len(g.authoredPoses()); n == 0 {
			g.animation.pickerPose = -1
		} else {
			g.animation.pickerPose = min(max(0, g.animation.pickerPose), n-1)
		}
		if clip := g.activeAnimationClip(); clip != nil {
			g.animation.cursor = min(max(0, g.animation.cursor), max(0, len(clip.Frames)-1))
			g.animation.playhead = min(max(0, g.animation.playhead), max(0, len(clip.Frames)-1))
		}
	}
	g.requestRender()
}

func (g *Game) resetAnimationEditorAfterRestore() {
	g.animation.nameEditing, g.animation.pickerOpen = false, false
	g.animation.playing, g.animation.stepped, g.animation.finished = false, false, false
	g.invalidateAnimationPreview()
}

func (g *Game) advanceAnimationPreview(now time.Time) {
	s := &g.animation
	if !g.animationWorkspaceActive() || g.mode != modeSprites || !s.playing {
		s.lastTick = time.Time{}
		return
	}
	frames := g.animationFrames()
	if len(frames) == 0 {
		s.playing = false
		s.lastTick = time.Time{}
		g.requestRender()
		return
	}
	if s.lastTick.IsZero() || now.Before(s.lastTick) {
		s.lastTick = now
		return
	}
	interval := time.Second / time.Duration(g.animationFPS())
	steps := now.Sub(s.lastTick) / interval
	if steps > 0 {
		if !g.animationLoops() && steps >= time.Duration(len(frames)-s.playhead) {
			s.playhead, s.playing, s.stepped, s.finished = len(frames)-1, false, true, true
		} else if g.animationLoops() {
			s.playhead = (s.playhead + int(steps%time.Duration(len(frames)))) % len(frames)
		} else {
			s.playhead += int(steps)
		}
		s.lastTick = s.lastTick.Add(steps * interval)
		g.requestRender()
	}
}

func (g *Game) animationPreviewPose() int {
	s := &g.animation
	frames := g.animationFrames()
	if g.animationWorkspaceActive() {
		if len(frames) == 0 {
			return -1
		}
		index := s.cursor
		if s.playing || s.stepped {
			index = s.playhead
		}
		return frames[min(max(0, index), len(frames)-1)]
	}
	if s.observedPose != g.selectedPose {
		s.observedPose, s.stepped = g.selectedPose, false
	}
	if (s.playing || s.stepped) && len(frames) > 0 {
		s.playhead = max(0, s.playhead) % len(frames)
		return frames[s.playhead]
	}
	if g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()) {
		return g.selectedPose
	}
	if len(frames) > 0 {
		return frames[0]
	}
	return -1
}

func (g *Game) startAnimationPreview() {
	frames := g.animationFrames()
	if !g.animationWorkspaceActive() || len(frames) == 0 {
		g.statusMsg = "Add registered poses before playing an animation"
		return
	}
	s := &g.animation
	if s.finished {
		s.playhead = 0
	} else if s.stepped {
		s.playhead = max(0, s.playhead) % len(frames)
	} else {
		s.playhead = min(max(0, s.cursor), len(frames)-1)
	}
	s.playing, s.stepped, s.finished, s.lastTick = true, false, false, time.Time{}
	g.requestRender()
}

func (g *Game) stepAnimationPreview() {
	frames := g.animationFrames()
	if !g.animationWorkspaceActive() || len(frames) == 0 {
		return
	}
	s := &g.animation
	if !s.playing && !s.stepped {
		s.playhead = min(max(0, s.cursor), len(frames)-1)
	}
	if g.animationLoops() {
		s.playhead = (s.playhead + 1) % len(frames)
	} else {
		s.playhead = min(s.playhead+1, len(frames)-1)
	}
	s.finished = !g.animationLoops() && s.playhead == len(frames)-1
	s.playing, s.stepped, s.observedPose, s.lastTick = false, true, g.selectedPose, time.Time{}
	g.requestRender()
}

func (g *Game) commitAnimationEdit(message string) {
	g.dirty = true
	g.invalidateAnimationPreview()
	g.undo.forcePush(g.snapshot())
	g.statusMsg = message
	g.animation.message, g.animation.messageError = message, false
}

func (g *Game) selectAnimationClip(index int) {
	count := len(g.model.Animation.Clips)
	g.animation.clip = 0
	if count > 0 {
		g.animation.clip = ((index-1)%count+count)%count + 1
	}
	g.animation.cursor, g.animation.playhead, g.animation.scroll = 0, 0, 0
	g.animation.timelineScroll = 0
	g.animation.pickerOpen, g.animation.finished = false, false
	g.animation.playing, g.animation.stepped, g.animation.nameEditing = false, false, false
	g.invalidateAnimationPreview()
}

func (g *Game) createAnimationClip() {
	index := editor.AddAnimationClip(&g.model.Animation, nil, g.animationFPS())
	g.animation.workspace = true
	g.selectAnimationClip(index + 1)
	if g.animation.newClips == nil {
		g.animation.newClips = make(map[string]bool)
	}
	g.animation.newClips[g.activeAnimationClip().Name] = true
	g.commitAnimationEdit("Created an empty clip. Choose registered poses to add.")
	g.beginAnimationNameEdit()
}

func (g *Game) editAnimationClipFrame(action string) {
	clip := g.activeAnimationClip()
	if clip == nil {
		return
	}
	s := &g.animation
	s.cursor = min(max(0, s.cursor), max(0, len(clip.Frames)-1))
	switch action {
	case "add":
		id := g.animation.pickerPose
		if id < 0 || id >= len(g.authoredPoses()) {
			g.statusMsg = "Choose a registered pose to add to this clip"
			return
		}
		at := min(s.cursor+1, len(clip.Frames))
		clip.Frames = append(clip.Frames, 0)
		copy(clip.Frames[at+1:], clip.Frames[at:])
		clip.Frames[at], s.cursor = id, at
	case "remove":
		if len(clip.Frames) == 0 {
			return
		}
		clip.Frames = append(clip.Frames[:s.cursor], clip.Frames[s.cursor+1:]...)
		s.cursor = min(s.cursor, max(0, len(clip.Frames)-1))
	case "left", "right":
		delta := -1
		if action == "right" {
			delta = 1
		}
		next := editor.MoveAnimationClipFrame(clip, s.cursor, delta)
		if next == s.cursor {
			return
		}
		s.cursor = next
	}
	s.playing, s.stepped, s.finished = false, false, false
	g.commitAnimationEdit("Updated " + clip.Name + " playback order")
	if g.animationWorkspaceActive() {
		rows := max(1, newAnimationWorkspaceLayout(g.animationWorkspaceRect()).Order.H/animationEntryHeight)
		if s.cursor < s.timelineScroll {
			s.timelineScroll = s.cursor
		}
		if s.cursor >= s.timelineScroll+rows {
			s.timelineScroll = s.cursor - rows + 1
		}
	}
}

func (g *Game) changeAnimationFPS(delta int) {
	fps := min(60, max(1, g.animationFPS()+delta))
	if fps == g.animationFPS() {
		return
	}
	g.animation.lastTick = time.Time{}
	if clip := g.activeAnimationClip(); clip != nil {
		clip.FPS = fps
		g.commitAnimationEdit(fmt.Sprintf("%s: %d FPS", clip.Name, fps))
	} else {
		g.animation.fps = fps
		g.requestRender()
	}
}

func (g *Game) clampAnimationScroll(layout animationDockLayout) {
	visible := max(1, layout.Track.W/animationFrameCellWidth)
	g.animation.scroll = min(max(0, g.animation.scroll), max(0, len(g.pageAnimationPoses())-visible))
}

func animationFrameRect(layout animationDockLayout, index, scroll int) editor.Rect {
	return editor.Rect{X: layout.Track.X + (index-scroll)*animationFrameCellWidth, Y: layout.Track.Y, W: animationFrameCellWidth - 4, H: layout.Track.H}
}

func (g *Game) animationFrameLabelRect(id int) editor.Rect {
	layout := newAnimationDockLayout(g.animationDockRect())
	for index, frame := range g.pageAnimationPoses() {
		if frame != id {
			continue
		}
		rect := animationFrameRect(layout, index, g.animation.scroll)
		rect.Y, rect.H = rect.Y+rect.H-uiSmallTextHeight-6, uiSmallTextHeight
		if visible, ok := intersectEditorRects(rect, layout.Track); ok {
			return visible
		}
	}
	return editor.Rect{}
}

func (g *Game) updateAnimationDock(mx, my int) {
	if !g.objEditing || g.model == nil {
		return
	}
	layout := newAnimationDockLayout(g.animationDockRect())
	if !rectContains(layout.Panel, mx, my) {
		return
	}
	if _, wheel := ebiten.Wheel(); wheel != 0 && rectContains(layout.Track, mx, my) {
		g.animation.scroll -= int(wheel)
		g.clampAnimationScroll(layout)
		g.requestRender()
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.clickAnimationDock(layout, mx, my)
	}
}

func (g *Game) clickAnimationDock(layout animationDockLayout, mx, my int) {
	s := &g.animation
	switch {
	case rectContains(poseVariantsButton(layout.Panel), mx, my):
		g.poseVariants.open = true
		g.poseVariants.source = max(1, g.currentColor)
		g.poseVariants.selected = min(max(0, g.poseVariants.selected), max(0, len(g.model.Poses.Variants)-1))
		g.requestRender()
	case rectContains(layout.RegisterPose, mx, my):
		g.capturePoseFromSelection(false)
		if index := slices.Index(g.pageAnimationPoses(), g.selectedPose); index >= 0 {
			visible := max(1, layout.Track.W/animationFrameCellWidth)
			if index < s.scroll {
				s.scroll = index
			}
			if index >= s.scroll+visible {
				s.scroll = index - visible + 1
			}
		}
	case rectContains(layout.UpdatePose, mx, my):
		g.capturePoseFromSelection(true)
	case rectContains(layout.DeletePose, mx, my):
		g.deleteSelectedPose()
	case rectContains(layout.CopyPose, mx, my):
		g.copyPoseUsage()
	case rectContains(layout.ScrollPrev, mx, my):
		s.scroll--
	case rectContains(layout.ScrollNext, mx, my):
		s.scroll++
	case rectContains(layout.Track, mx, my):
		index := (mx-layout.Track.X)/animationFrameCellWidth + s.scroll
		poses := g.pageAnimationPoses()
		if index >= 0 && index < len(poses) {
			id := poses[index]
			if id == g.selectedPose && rectContains(g.animationFrameLabelRect(id), mx, my) {
				g.beginPoseNameEdit()
			} else {
				g.selectPose(id)
			}
		}
	}
	g.clampAnimationScroll(layout)
	g.requestRender()
}

func (g *Game) animationNameEditing() bool { return g.animation.nameEditing }

func (g *Game) beginAnimationNameEdit() {
	if clip := g.activeAnimationClip(); clip != nil {
		g.cancelPoseNameEdit()
		g.animation.nameEditing, g.animation.nameSelectAll = true, true
		g.animation.nameDraft = clip.Name
		g.statusMsg = "Name this animation, Enter to save, Esc to cancel"
		g.requestRender()
	}
}

func (g *Game) commitAnimationNameEdit() bool {
	clip := g.activeAnimationClip()
	if clip == nil {
		g.animation.nameEditing = false
		return false
	}
	name := strings.TrimSpace(g.animation.nameDraft)
	copyWorkspace := g.model.Animation
	copyWorkspace.Clips = slices.Clone(copyWorkspace.Clips)
	copyWorkspace.Clips[g.animation.clip-1].Name = name
	sheet := g.activeSheet()
	if err := assetsync.ValidateAnimationWorkspace(copyWorkspace, sheet.Width, sheet.Height, len(g.authoredPoses())); err != nil {
		g.statusMsg = err.Error()
		g.animation.message, g.animation.messageError = err.Error(), true
		return false
	}
	if g.animation.newClips[clip.Name] && name != clip.Name {
		delete(g.animation.newClips, clip.Name)
		g.animation.newClips[name] = true
	}
	editor.RenameAnimationInTileLinks(&g.model.TileLinks, clip.Name, name)
	clip.Name = name
	g.animation.nameEditing = false
	g.commitAnimationEdit("Animation: " + name)
	return true
}

func (g *Game) updateAnimationNameTyping() bool {
	s := &g.animation
	if !s.nameEditing {
		return false
	}
	if g.animationWorkspaceActive() && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		layout := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
		if !rectContains(layout.ClipName, mx, my) {
			if g.commitAnimationNameEdit() {
				g.clickAnimationWorkspace(layout, mx, my)
			}
			return true
		}
		s.nameSelectAll = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.nameEditing = false
		g.statusMsg = ""
		g.requestRender()
		return true
	}
	modifier := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	if modifier && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		s.nameSelectAll = true
	}
	if !modifier {
		for _, r := range ebiten.AppendInputChars(nil) {
			if unicode.IsControl(r) {
				continue
			}
			draft := s.nameDraft
			if s.nameSelectAll {
				draft = ""
			}
			if len(draft)+utf8.RuneLen(r) <= assetsync.MaxPoseNameBytes {
				s.nameDraft, s.nameSelectAll = draft+string(r), false
			}
		}
	}
	if codeKeyRepeat(ebiten.KeyBackspace) {
		if s.nameSelectAll {
			s.nameDraft = ""
		} else if s.nameDraft != "" {
			_, size := utf8.DecodeLastRuneInString(s.nameDraft)
			s.nameDraft = s.nameDraft[:len(s.nameDraft)-size]
		}
		s.nameSelectAll = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		g.commitAnimationNameEdit()
	}
	g.requestRender()
	return true
}

func (g *Game) prepareAnimationPreview(frames []int, previewID int) {
	s := &g.animation
	ids := frames
	if previewID >= 0 && !slices.Contains(ids, previewID) {
		ids = append(slices.Clone(ids), previewID)
	}
	if s.previews == nil || !slices.Equal(ids, s.cachedFrames) {
		s.cachedFrames = slices.Clone(ids)
		s.bounds = editor.AnimationPreviewBounds(*g.activeSheet(), g.authoredPoses(), ids)
		s.previews = make(map[int]editor.PosePreview)
	}
	if s.thumbs == nil {
		s.thumbs = make(map[int]editor.PosePreview)
	}
}

func (g *Game) drawAnimationFrame(screen *ebiten.Image, preview editor.PosePreview, rect editor.Rect) {
	if preview.Width == 0 || preview.Height == 0 {
		return
	}
	scale := min(float64(rect.W-8)/float64(preview.Width), float64(rect.H-8)/float64(preview.Height))
	w, h := float64(preview.Width)*scale, float64(preview.Height)*scale
	g.drawPosePreview(screen, preview, float64(rect.X)+(float64(rect.W)-w)/2, float64(rect.Y)+(float64(rect.H)-h)/2, w, h, rect)
}

func (g *Game) drawAnimationDock(screen *ebiten.Image) {
	if !g.objEditing || g.model == nil || g.activeSheet() == nil {
		return
	}
	layout := newAnimationDockLayout(g.animationDockRect())
	fillRoundedRect(screen, layout.Panel, 5, colorInk)
	strokeRoundedRect(screen, layout.Panel, 5, colorBorder)
	g.clampAnimationScroll(layout)
	poses := g.pageAnimationPoses()
	title := truncateTextToWidth(fmt.Sprintf("REGISTERED POSES ON THIS PAGE · %d", len(poses)), layout.CopyPose.X-layout.Track.X-8, uiSmallFace)
	drawSmallText(screen, title, layout.Track.X, layout.Panel.Y+10, colorText)
	drawSmallText(screen, "POSE PREVIEW", layout.Preview.X, layout.Panel.Y+10, colorDim)
	drawTactileButton(screen, poseVariantsButton(layout.Panel), "COLOR VARIANTS", false, colorSprite, true)
	if g.animation.thumbs == nil {
		g.animation.thumbs = make(map[int]editor.PosePreview)
	}
	if len(poses) == 0 {
		drawSmallTextCentered(screen, "Select artwork, then REGISTER POSE", layout.Track, colorDim)
	}
	for index := g.animation.scroll; index < len(poses); index++ {
		cell := animationFrameRect(layout, index, g.animation.scroll)
		if cell.X+cell.W > layout.Track.X+layout.Track.W {
			break
		}
		id := poses[index]
		if id < 0 || id >= len(g.authoredPoses()) {
			continue
		}
		if id == g.selectedPose {
			fillRect(screen, cell, rgba(colorSprite, 28))
			strokeRect(screen, cell, colorSprite)
		}
		if g.objShowReuse && g.poseSharesSelectedTile(id) {
			strokeRect(screen, cell, colorHighlight)
		}
		thumbRect := editor.Rect{X: cell.X + 4, Y: cell.Y + 4, W: cell.W - 8, H: cell.H - uiSmallTextHeight - 14}
		drawCheckerboardColors(screen, thumbRect, colorSheetCheckerA, colorSheetCheckerB)
		g.drawAnimationFrame(screen, g.animationPoseThumbnail(id), thumbRect)
		label := truncateTextToWidth(g.poseVisibleLabel(id, g.authoredPoses()[id]), cell.W-6, uiSmallFace)
		drawSmallTextCentered(screen, label, editor.Rect{X: cell.X, Y: cell.Y + cell.H - uiSmallTextHeight - 6, W: cell.W, H: uiSmallTextHeight}, colorText)
	}
	drawCheckerboardColors(screen, layout.Preview, colorSheetCheckerA, colorSheetCheckerB)
	if g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()) {
		g.drawAnimationFrame(screen, g.animationPoseThumbnail(g.selectedPose), layout.Preview)
	}
	for _, button := range []struct {
		rect  editor.Rect
		label string
	}{
		{layout.RegisterPose, "REGISTER POSE"}, {layout.UpdatePose, "UPDATE POSE"}, {layout.DeletePose, "DELETE POSE"}, {layout.ScrollPrev, "<"}, {layout.ScrollNext, ">"},
	} {
		drawTactileButton(screen, button.rect, button.label, false, colorSprite, true)
	}
	drawTactileButton(screen, layout.CopyPose, "COPY LUA CALL", false, colorSprite, g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()))
}

func (g *Game) animationPoseThumbnail(id int) editor.PosePreview {
	if g.animation.thumbs == nil {
		g.animation.thumbs = make(map[int]editor.PosePreview)
	}
	thumb, ok := g.animation.thumbs[id]
	if !ok {
		if len(g.animation.thumbs) >= 32 {
			clear(g.animation.thumbs)
		}
		thumb = g.posePreview(id)
		g.animation.thumbs[id] = thumb
	}
	return thumb
}
