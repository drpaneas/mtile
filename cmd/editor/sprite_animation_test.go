package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func TestAnimationPlaybackOrderSpeedAndEditingSelection(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.model.Animation.Clips = []assetsync.AnimationClip{{Name: "Idle", Frames: []int{1, 0, 1}, FPS: 10}}
	g.setAnimationWorkspaceActive(true)
	g.selectAnimationClip(1)
	g.selectedPose = 1
	g.startAnimationPreview()
	start := time.Unix(100, 0)
	g.advanceAnimationPreview(start)
	g.advanceAnimationPreview(start.Add(99 * time.Millisecond))
	if g.animationPreviewPose() != 1 {
		t.Fatal("preview advanced before the frame interval")
	}
	g.advanceAnimationPreview(start.Add(100 * time.Millisecond))
	if g.animationPreviewPose() != 0 || g.selectedPose != 1 {
		t.Fatal("playback did not follow clip order independently of editing selection")
	}
	g.advanceAnimationPreview(start.Add(450 * time.Millisecond))
	if g.animation.playhead != 1 || g.animationPreviewPose() != 0 {
		t.Fatal("late tick did not preserve elapsed-time playback position")
	}
	g.changeAnimationFPS(10)
	if g.model.Animation.Clips[0].FPS != 20 {
		t.Fatal("clip speed was not persisted")
	}
	g.advanceAnimationPreview(start.Add(time.Second))
	g.advanceAnimationPreview(start.Add(time.Second + 50*time.Millisecond))
	if g.animation.playhead != 2 {
		t.Fatal("new speed did not change frame interval")
	}
	g.animation.playing = false
	if g.animationPreviewPose() != 1 {
		t.Fatal("paused preview does not show selected timeline pose")
	}
	before := g.animation.playhead
	g.advanceAnimationPreview(start.Add(time.Minute))
	if g.animation.playhead != before {
		t.Fatal("paused preview advanced")
	}
	g.stepAnimationPreview()
	if g.animationPreviewPose() != 0 || g.selectedPose != 1 || g.animation.playing {
		t.Fatal("step changed editing selection or resumed playback")
	}
	g.selectedPose = 0
	if g.animationPreviewPose() != 0 || !g.animation.stepped {
		t.Fatal("editing selection changed the independent animation preview")
	}
}

func TestAnimationClipEditingAndUndo(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.model.EnsureObjPages()
	g.undo.reset(g.snapshot())
	g.createAnimationClip()
	if g.animation.clip != 1 || len(g.model.Animation.Clips[0].Frames) != 0 {
		t.Fatal("new clip should start empty")
	}
	g.animation.pickerPose = 0
	g.editAnimationClipFrame("add")
	g.animation.pickerPose = 1
	g.editAnimationClipFrame("add")
	g.animation.nameDraft = "Hero idle"
	if !g.commitAnimationNameEdit() || g.model.Animation.Clips[0].Name != "Hero idle" {
		t.Fatal("clip rename failed")
	}
	g.animation.pickerPose = 1
	g.animation.cursor = 0
	g.editAnimationClipFrame("add")
	if !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{0, 1, 1}) {
		t.Fatal("add did not insert after selected timeline entry")
	}
	g.editAnimationClipFrame("left")
	if !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{1, 0, 1}) {
		t.Fatal("move did not reorder playback")
	}
	g.editAnimationClipFrame("remove")
	if !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{0, 1}) {
		t.Fatal("remove deleted wrong occurrence")
	}
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("clip edit has no undo")
	}
	g.restore(snapshot)
	if !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{1, 0, 1}) {
		t.Fatal("undo lost ordered clip metadata")
	}
}

func TestAnimationPauseAndResumeRetainPreviewFrame(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.model.Animation.Clips = []assetsync.AnimationClip{{Name: "Idle", Frames: []int{0, 1}, FPS: 8}}
	g.setAnimationWorkspaceActive(true)
	g.selectedPose = 0
	g.startAnimationPreview()
	start := time.Unix(100, 0)
	g.advanceAnimationPreview(start)
	g.advanceAnimationPreview(start.Add(125 * time.Millisecond))
	if g.animationPreviewPose() != 1 {
		t.Fatal("preview did not advance")
	}
	layout := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
	g.clickAnimationWorkspace(layout, layout.Play.X+1, layout.Play.Y+1)
	if g.animation.playing || g.animationPreviewPose() != 1 || g.selectedPose != 0 {
		t.Fatal("pause snapped back to editing selection")
	}
	g.clickAnimationWorkspace(layout, layout.Play.X+1, layout.Play.Y+1)
	if !g.animation.playing || g.animationPreviewPose() != 1 {
		t.Fatal("resume restarted instead of retaining the paused frame")
	}
}

func TestAnimationRegisterPoseDoesNotChangeClipOrder(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.model.Animation.Clips = []assetsync.AnimationClip{{Name: "Idle", Frames: []int{0, 1}, FPS: 8}}
	g.selectAnimationClip(1)
	g.selectPose(1)
	if err := g.model.SetObjSpritePixel(6, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	g.undo.reset(g.snapshot())
	g.capturePoseFromSelection(false)
	if len(g.authoredPoses()) != 3 || !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{0, 1}) {
		t.Fatal("registering a pose unexpectedly changed a clip")
	}
	g.cancelPoseNameEdit()
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("capturing a frame did not create undo history")
	}
	g.restore(snapshot)
	if len(g.authoredPoses()) != 2 || !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{0, 1}) {
		t.Fatal("one undo did not restore both poses and clip order")
	}
	snapshot, ok = g.undo.redo()
	if !ok {
		t.Fatal("captured frame could not be redone")
	}
	g.restore(snapshot)
	if len(g.authoredPoses()) != 3 || !reflect.DeepEqual(g.model.Animation.Clips[0].Frames, []int{0, 1}) {
		t.Fatal("redo did not preserve independent clip order")
	}
}

func TestAnimationDockControlsFitMinimumWidth(t *testing.T) {
	layout := newAnimationDockLayout(editor.Rect{X: 12, Y: 20, W: 608, H: 240})
	controls := []editor.Rect{layout.Track, layout.Preview, layout.RegisterPose, layout.UpdatePose, layout.DeletePose, layout.ScrollPrev, layout.ScrollNext}
	for i, rect := range controls {
		if !rectInsideIDE(rect, layout.Panel) {
			t.Fatalf("control %d outside dock: %+v", i, rect)
		}
		for j := 0; j < i; j++ {
			if _, overlap := intersectEditorRects(rect, controls[j]); overlap {
				t.Fatalf("controls %d and %d overlap", i, j)
			}
		}
	}
}

func TestAnimationPreviewCacheInvalidatesAfterPixelEdit(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.prepareAnimationPreview(g.animationFrames(), 0)
	g.animation.previews[0] = editor.PosePreview{Width: 1, Height: 1, Pixels: []int{99}}
	g.invalidateAnimationPreview()
	if g.animation.previews != nil || g.animation.thumbs != nil {
		t.Fatal("stale preview caches survived artwork change")
	}
}

func TestAnimationWorkspaceAddsRegisteredPoseFromAnotherPage(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	page, err := g.model.AddObjPage()
	if err != nil {
		t.Fatal(err)
	}
	p := g.model.Animation.Pages[page]
	g.model.Poses.Poses = append(g.model.Poses.Poses, assetsync.PoseRecord{Name: "Enemy punch", Parts: []assetsync.PosePartRecord{{Tile: p.Y/8*g.spriteCols() + p.X/8}}})
	g.invalidateAnimationPreview()
	g.setAnimationWorkspaceActive(true)
	g.createAnimationClip()
	g.animation.nameDraft = "punch"
	if !g.commitAnimationNameEdit() {
		t.Fatal("could not name empty clip")
	}
	if len(g.activeAnimationClip().Frames) != 0 {
		t.Fatal("new clip auto-populated page poses")
	}
	selectedPage, selectedPose := g.objPage, g.selectedPose
	layout := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
	g.clickAnimationWorkspace(layout, layout.PosePicker.X+1, layout.PosePicker.Y+1)
	g.animation.pickerSearch = "enemy"
	if ids := g.animationRegisteredPoseIDs(); !reflect.DeepEqual(ids, []int{2}) {
		t.Fatalf("global pose search=%v", ids)
	}
	g.clickAnimationWorkspace(layout, layout.PickerList.X+1, layout.PickerList.Y+1)
	if g.animation.pickerOpen || g.animation.pickerPose != 2 {
		t.Fatal("dropdown did not select the matching pose")
	}
	g.clickAnimationWorkspace(layout, layout.AddPose.X+1, layout.AddPose.Y+1)
	if !reflect.DeepEqual(g.activeAnimationClip().Frames, []int{2}) {
		t.Fatal("Add Pose did not use global registered selection")
	}
	if g.objPage != selectedPage || g.selectedPose != selectedPose {
		t.Fatal("animation assembly changed the pose editing selection")
	}
}

func TestAnimationRegisterSaveDeleteAndIOErrors(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.setAnimationWorkspaceActive(true)
	g.createAnimationClip()
	g.animation.nameDraft = "idle"
	if !g.commitAnimationNameEdit() {
		t.Fatal("rename failed")
	}
	if g.registerAnimationClip() || !g.animation.messageError || !strings.Contains(g.animation.message, "at least one") {
		t.Fatal("registered an empty clip without a useful error")
	}
	g.animation.pickerPose = 1
	g.editAnimationClipFrame("add")
	g.changeAnimationFPS(4)
	g.toggleAnimationLoop()
	if !g.registerAnimationClip() {
		t.Fatal(g.animation.message)
	}
	saved, err := assetsync.LoadAnimationWorkspace(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Clips) != 1 || saved.Clips[0].Name != "idle" || saved.Clips[0].FPS != 12 || saved.Clips[0].Loop == nil || *saved.Clips[0].Loop || !reflect.DeepEqual(saved.Clips[0].Frames, []int{1}) {
		t.Fatalf("saved clip=%+v", saved.Clips)
	}
	if g.animation.newClips["idle"] {
		t.Fatal("registered clip still shown as unregistered")
	}
	poses := g.snapshot().clone().Poses
	g.deleteAnimationClip()
	if len(g.model.Animation.Clips) != 0 || !reflect.DeepEqual(poses, g.model.Poses) {
		t.Fatal("deleting a clip changed registered poses")
	}
	if !g.registerAnimationClip() {
		t.Fatal("could not save clip deletion")
	}
	saved, err = assetsync.LoadAnimationWorkspace(g.model.Dir)
	if err != nil || len(saved.Clips) != 0 {
		t.Fatalf("deleted clip remained on disk: %+v, %v", saved, err)
	}
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("clip deletion was not undoable")
	}
	g.restore(snapshot)
	g.selectAnimationClip(1)
	if len(g.model.Animation.Clips) != 1 {
		t.Fatal("undo did not restore the named clip")
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	g.model.Dir = blocked
	if g.registerAnimationClip() || !g.animation.messageError || !strings.Contains(g.animation.message, "Could not save") || len(g.model.Animation.Clips) != 1 {
		t.Fatal("save failure was hidden or discarded the clip")
	}
}

func TestAnimationOneShotHoldsLastPoseAndCodeUsesElapsedTime(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	loop := false
	g.model.Animation.Clips = []assetsync.AnimationClip{{Name: "punch", Frames: []int{0, 1, 0}, FPS: 10, Loop: &loop}}
	g.setAnimationWorkspaceActive(true)
	g.startAnimationPreview()
	start := time.Unix(100, 0)
	g.advanceAnimationPreview(start)
	g.advanceAnimationPreview(start.Add(200 * time.Millisecond))
	if !g.animation.playing || g.animation.playhead != 2 {
		t.Fatal("one-shot stopped before the final pose duration")
	}
	g.advanceAnimationPreview(start.Add(300 * time.Millisecond))
	if g.animation.playing || !g.animation.finished || g.animationPreviewPose() != 0 {
		t.Fatal("one-shot did not stop on its final pose")
	}
	g.advanceAnimationPreview(start.Add(time.Hour))
	if g.animation.playhead != 2 {
		t.Fatal("finished one-shot advanced")
	}
	g.stepAnimationPreview()
	if g.animation.playhead != 2 {
		t.Fatal("one-shot step wrapped")
	}
	want := `anim("punch", x, y, false, false, time() - started)`
	if g.animationUsage() != want {
		t.Fatalf("one-shot usage=%s", g.animationUsage())
	}
	oldWriter := writeAnimationUsageClipboard
	t.Cleanup(func() { writeAnimationUsageClipboard = oldWriter })
	var copied string
	writeAnimationUsageClipboard = func(text string) error { copied = text; return nil }
	layout := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
	g.clickAnimationWorkspace(layout, layout.CopyUsage.X+1, layout.CopyUsage.Y+1)
	if copied != want {
		t.Fatalf("copied usage=%q", copied)
	}
	g.startAnimationPreview()
	if g.animation.playhead != 0 || !g.animation.playing {
		t.Fatal("replay did not restart completed one-shot")
	}
	g.toggleAnimationLoop()
	if g.animationUsage() != `anim("punch", x, y)` {
		t.Fatal("looping usage should use the default game clock")
	}
}

func TestAnimationWorkspaceControlsFitAndDoNotOverlap(t *testing.T) {
	t.Cleanup(func() { setInterfaceTextSize(textComfort); setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	for size := textCompact; size < interfaceTextSizeCount; size++ {
		setInterfaceTextSize(size)
		for _, window := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1280, 1024}, {1920, 1200}} {
			setEditorLayout(window[0], window[1])
			g := &Game{}
			l := newAnimationWorkspaceLayout(g.animationWorkspaceRect())
			controls := []editor.Rect{l.ClipPrev, l.ClipName, l.ClipNext, l.NewClip, l.RenameClip, l.DeleteClip, l.SaveClip, l.PosePicker, l.AddPose, l.Order, l.RemovePose, l.MoveUp, l.MoveDown, l.OrderPrev, l.OrderNext, l.Preview, l.Play, l.Step, l.FPSMinus, l.FPS, l.FPSPlus, l.Loop, l.LoopHint, l.Usage, l.CopyUsage, l.Message}
			for i, r := range controls {
				if r.W <= 0 || r.H <= 0 || !rectInsideIDE(r, l.Panel) {
					t.Fatalf("%v %v control %d escapes workspace: %+v", size, window, i, r)
				}
				for j := 0; j < i; j++ {
					if _, ok := intersectEditorRects(r, controls[j]); ok {
						t.Fatalf("%v %v controls %d and %d overlap", size, window, i, j)
					}
				}
			}
			for _, r := range []editor.Rect{l.PickerSearch, l.PickerList, l.PickerPrev, l.PickerNext} {
				if !rectInsideIDE(r, l.PickerPopup) {
					t.Fatal("dropdown control escapes popup")
				}
			}
			if !rectInsideIDE(l.PickerPopup, l.Panel) {
				t.Fatal("dropdown escapes workspace")
			}
			for _, button := range []struct {
				r     editor.Rect
				label string
			}{{l.SaveClip, "REGISTER CLIP"}, {l.RemovePose, "REMOVE POSE"}, {l.MoveDown, "MOVE DOWN"}} {
				if smallTextWidth(button.label)+16 > button.r.W {
					t.Fatalf("button label does not fit: %s", button.label)
				}
			}
		}
	}
}

func TestAnimationRestoreClampsTransientSelections(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.setAnimationWorkspaceActive(true)
	g.createAnimationClip()
	g.animation.nameEditing = false
	g.animation.pickerOpen = true
	g.animation.pickerPose = 999
	g.animation.playing = true
	g.model.Animation.Clips = nil
	g.resetAnimationEditorAfterRestore()
	if g.animation.clip != 0 || g.animation.pickerOpen || g.animation.nameEditing || g.animation.playing || g.animation.pickerPose >= len(g.authoredPoses()) {
		t.Fatal("restore kept transient references to removed assets")
	}
}
