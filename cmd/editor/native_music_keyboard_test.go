package main

import (
	"testing"

	"github.com/drpaneas/mtile/internal/performance"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestNativeMusicShortcutKeepsUndoAndRedo(t *testing.T) {
	g := newNativeMusicTestGame(t)
	t.Cleanup(g.closeNativePerformance)
	g.mode = modeMusic
	g.nativeMuteMask = 1
	g.nativeOctave = 5
	g.undo.reset(g.snapshot())
	if err := g.model.SetNativeNoteDuration(0, 0, 8, 65, 4); err != nil {
		t.Fatal(err)
	}
	g.undo.forcePush(g.snapshot())
	undone, ok := g.undo.undo()
	if !ok {
		t.Fatal("expected an edit to undo")
	}
	g.restore(undone)
	pressedZ := func(key ebiten.Key) bool { return key == ebiten.KeyZ }

	// Music keyboard input receives the same keypress after the editor undoes.
	g.updateNativeMusicKeyboard(false, true, pressedZ)
	if _, _, inserted := g.nativeEventAtStep(0); inserted || len(g.undo.future) != 1 {
		t.Fatalf("modified Z inserted a note or discarded redo: inserted=%v redo=%d", inserted, len(g.undo.future))
	}

	g.updateNativeMusicKeyboard(false, false, pressedZ)
	if event, _, inserted := g.nativeEventAtStep(0); !inserted || event.Note != 72 {
		t.Fatalf("unmodified Z did not insert note 72: inserted=%v note=%d", inserted, event.Note)
	}
}

func TestNativePerformanceShortcutKeepsNoteOff(t *testing.T) {
	g := &Game{nativeOctave: 4, nativeSession: performance.NewSession(nil, defaultPerformanceSoundBank())}
	g.nativeSession.Arm(true, 0)
	noKeys := func(ebiten.Key) bool { return false }
	pressedA := func(key ebiten.Key) bool { return key == ebiten.KeyA }
	pressedShortcutKeys := func(key ebiten.Key) bool { return key == ebiten.KeyS || key == ebiten.KeyZ }

	if g.updateNativePerformanceKeyboard(true, pressedShortcutKeys, noKeys) || g.nativeOctave != 4 {
		t.Fatal("modified keypress changed live performance")
	}
	if !g.updateNativePerformanceKeyboard(false, pressedA, noKeys) || !g.nativeSession.Snapshot().Allocation.Active[0] {
		t.Fatal("unmodified A did not start a live note")
	}
	if !g.updateNativePerformanceKeyboard(true, pressedShortcutKeys, pressedA) {
		t.Fatal("release of a held note was ignored with a modifier down")
	}
	if active := g.nativeSession.Snapshot().Allocation.Active; active != [4]bool{} || g.nativeOctave != 4 {
		t.Fatalf("modified release did not stop the held note cleanly: active=%v octave=%d", active, g.nativeOctave)
	}
}
