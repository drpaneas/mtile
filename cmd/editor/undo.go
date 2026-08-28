package main

import (
	"encoding/json"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
)

const (
	undoMaxDepth = 50
	undoDebounce = 150 * time.Millisecond
)

// undoSnapshot is a full copy of everything undo/redo needs to restore, not
// just the pixels that changed. Full snapshots are simpler than per-edit
// diffs and are the right trade-off for a tool that is not performance
// sensitive and where undo depth is capped at 50 anyway.
type undoSnapshot struct {
	Sheet              assetsync.Spritesheet       `json:"sheet"`
	Map                assetsync.Map               `json:"map"`
	SFX                assetsync.SFXLibrary        `json:"sfx"`
	Music              assetsync.MusicLibrary      `json:"music"`
	Samples            assetsync.SampleLibrary     `json:"samples"`
	AudioSettings      assetsync.AudioSettings     `json:"audioSettings"`
	NativeAudio        assetsync.NativeAudioAssets `json:"nativeAudio"`
	Project            assetsync.Project           `json:"project"`
	Palette            assetsync.Palette           `json:"palette"`
	CurrentSprite      int                         `json:"currentSprite"`
	CurrentColor       int                         `json:"currentColor"`
	GridCols           int                         `json:"gridCols"`
	GridRows           int                         `json:"gridRows"`
	NativeChannel      int                         `json:"nativeChannel"`
	NativeSong         int                         `json:"nativeSong"`
	NativeSequence     int                         `json:"nativeSequence"`
	NativeBar          int                         `json:"nativeBar"`
	NativeSelectedStep int                         `json:"nativeSelectedStep"`
	NativeOctave       int                         `json:"nativeOctave"`
	NativeNoteLength   int                         `json:"nativeNoteLength"`
	NativeNoteTicks    int                         `json:"nativeNoteTicks"`
	NativeGridQuantize uint8                       `json:"nativeGridQuantize"`
	NativeInstrument   [4]int                      `json:"nativeInstrument"`
	NativeAdvanced     bool                        `json:"nativeAdvanced"`
	NativeSFXMode      bool                        `json:"nativeSFXMode"`
	NativeSFXID        int                         `json:"nativeSFXID"`
}

// clone deep-copies a snapshot through a JSON round trip. This is slower
// than hand-written slice copies but is trivially correct for the nested
// slice-of-slices sprite pixel data, and undo/redo is not a hot path.
func (s undoSnapshot) clone() undoSnapshot {
	data, err := json.Marshal(s)
	if err != nil {
		return s
	}
	var out undoSnapshot
	if err := json.Unmarshal(data, &out); err != nil {
		return s
	}
	return out
}

// undoStack holds bounded undo/redo history in memory only; it is never
// written to disk. Individual edits are not persisted as they happen (see
// Game.persist for when saves actually happen).
type undoStack struct {
	past     []undoSnapshot
	future   []undoSnapshot
	lastPush time.Time
}

func (u *undoStack) reset(initial undoSnapshot) {
	u.past = []undoSnapshot{initial.clone()}
	u.future = nil
	u.lastPush = time.Time{}
}

// push records a new state, debounced so a fast pixel-drag doesn't fill the
// stack with one entry per frame. Any pending redo history is discarded, matching
// standard undo/redo semantics: a new edit after an undo abandons the
// undone future.
func (u *undoStack) push(s undoSnapshot) {
	now := time.Now()
	if !u.lastPush.IsZero() && now.Sub(u.lastPush) < undoDebounce {
		return
	}
	u.lastPush = now
	u.past = append(u.past, s.clone())
	if len(u.past) > undoMaxDepth {
		u.past = u.past[len(u.past)-undoMaxDepth:]
	}
	u.future = nil
}

// forcePush is push without debouncing, for discrete actions (flag toggle,
// palette edit) where every action must be individually undoable and none
// of them arrive fast enough to need coalescing.
func (u *undoStack) forcePush(s undoSnapshot) {
	u.lastPush = time.Time{}
	u.push(s)
}

func (u *undoStack) undo() (undoSnapshot, bool) {
	if len(u.past) < 2 {
		return undoSnapshot{}, false
	}
	// The next edit after an undo must be recorded immediately. Otherwise a
	// fast undo-then-edit can be swallowed by the pixel-drag debounce window,
	// leaving stale redo history available and violating normal undo semantics.
	u.lastPush = time.Time{}
	last := u.past[len(u.past)-1]
	u.past = u.past[:len(u.past)-1]
	u.future = append(u.future, last)
	return u.past[len(u.past)-1].clone(), true
}

func (u *undoStack) redo() (undoSnapshot, bool) {
	if len(u.future) == 0 {
		return undoSnapshot{}, false
	}
	u.lastPush = time.Time{}
	next := u.future[len(u.future)-1]
	u.future = u.future[:len(u.future)-1]
	u.past = append(u.past, next)
	return next.clone(), true
}
