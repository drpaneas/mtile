package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func legacyObjectPagesFixture() *Model {
	sheet := &assetsync.Spritesheet{Width: 800, Height: 640}
	poses := assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "empty"}}}
	for i, point := range [][2]int{{0, 0}, {792, 312}, {472, 632}, {792, 632}} {
		id := point[1]/8*100 + point[0]/8
		sprite := filledCell(id, point[0], point[1], i+1)
		sprite.Flags = i + 1
		sheet.Sprites = append(sheet.Sprites, sprite)
		poses.Poses = append(poses.Poses, assetsync.PoseRecord{Parts: []assetsync.PosePartRecord{{Tile: id, X: -8, Y: 16, FlipX: i%2 == 0, FlipY: i%2 != 0}}})
	}
	poses.Poses = append(poses.Poses, assetsync.PoseRecord{Parts: []assetsync.PosePartRecord{{Tile: 3999}, {Tile: 7999, X: 8}}})
	loop := false
	return &Model{ObjSheet: sheet, Poses: poses, Animation: assetsync.AnimationWorkspace{Version: 1,
		Pages: []assetsync.AnimationPage{{Name: "Old left", Width: 480, Height: 320}, {Name: "Old right", X: 480, Width: 320, Height: 320}, {Name: "Old lower", Y: 320, Width: 800, Height: 320}},
		Clips: []assetsync.AnimationClip{{Name: "Idle", Frames: []int{1, 5, 1, 2}, FPS: 12}, {Name: "Once", Frames: []int{4, 3, 2}, FPS: 7, Loop: &loop}},
	}}
}

func objectPageSnapshot(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertUniformObjectPages(t *testing.T, m *Model) {
	t.Helper()
	if m.ObjSheet.Width != ObjPageWidth || len(m.Animation.Pages) == 0 || m.ObjSheet.Height != min(len(m.Animation.Pages)*ObjPageHeight, objMaxSheetHeight) {
		t.Fatalf("nonuniform bank %dx%d with %d pages", m.ObjSheet.Width, m.ObjSheet.Height, len(m.Animation.Pages))
	}
	for i, page := range m.Animation.Pages {
		if page.X != 0 || page.Y != i*ObjPageHeight || page.Width != ObjPageWidth || page.Height != objPageHeight(i) {
			t.Fatalf("page %d is not a canonical OBJ board: %+v", i, page)
		}
	}
}

func posePagePreviews(m *Model) []PosePreview {
	previews := make([]PosePreview, len(m.Poses.Poses))
	for i, pose := range m.Poses.Poses {
		previews[i] = ComposePose(*m.ObjSheet, pose)
	}
	return previews
}

func TestNormalizeObjectPagesPreservesLegacyPosePixelsAndIDs(t *testing.T) {
	m := legacyObjectPagesFixture()
	beforeFrames := posePagePreviews(m)
	beforeClips := objectPageSnapshot(t, m.Animation.Clips)
	beforePoses := m.Poses
	// Preserve unreferenced painted tiles and flag-only cells as well.
	m.ObjSheet.Sprites = append(m.ObjSheet.Sprites, filledCell(60, 480, 0, 9), filledCell(61, 488, 0, 0))
	m.ObjSheet.Sprites[len(m.ObjSheet.Sprites)-1].Flags = 42
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	if len(m.Animation.Pages) != 1 {
		t.Fatalf("legacy empty space was retained as %d pages", len(m.Animation.Pages))
	}
	if !reflect.DeepEqual(beforeFrames, posePagePreviews(m)) || !bytes.Equal(beforeClips, objectPageSnapshot(t, m.Animation.Clips)) {
		t.Fatal("normalization changed composed frame pixels or clip order/speed/loop")
	}
	for id, pose := range m.Poses.Poses {
		old := beforePoses.Poses[id]
		if pose.Name != old.Name || len(pose.Parts) != len(old.Parts) {
			t.Fatalf("global pose %d changed name or part count", id)
		}
		for i, part := range pose.Parts {
			part.Tile = old.Parts[i].Tile
			if part != old.Parts[i] {
				t.Fatalf("pose %d part %d changed local coordinates or flips", id, i)
			}
		}
	}
	// Shared source references must retain their editing relationship.
	if m.Poses.Poses[2].Parts[0].Tile != m.Poses.Poses[5].Parts[0].Tile || m.Poses.Poses[4].Parts[0].Tile != m.Poses.Poses[5].Parts[1].Tile {
		t.Fatal("normalization duplicated a shared logical source tile")
	}
	colors, flags := map[int]int{}, map[int]int{}
	for _, sprite := range m.ObjSheet.Sprites {
		colors[sprite.Pixels[0][0]]++
		flags[sprite.Flags]++
	}
	for _, color := range []int{1, 2, 3, 4, 9} {
		if colors[color] != 1 {
			t.Fatalf("painted color %d was lost or duplicated: %v", color, colors)
		}
	}
	for _, flag := range []int{1, 2, 3, 4, 42} {
		if flags[flag] != 1 {
			t.Fatalf("tile flags %d were lost or duplicated: %v", flag, flags)
		}
	}
	before := objectPageSnapshot(t, m)
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	m.EnsureObjPages()
	if !bytes.Equal(before, objectPageSnapshot(t, m)) {
		t.Fatal("normalization is not idempotent")
	}
}

func TestNormalizeObjectPagesKeepsFourFighterFramesTogether(t *testing.T) {
	m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 800, Height: 640}, Poses: assetsync.PoseFile{Version: 1}}
	for frame := 0; frame < 4; frame++ {
		pose := assetsync.PoseRecord{}
		for y := 0; y < 10; y++ {
			for x := 0; x < 5; x++ {
				id := y*100 + frame*5 + x
				m.ObjSheet.Sprites = append(m.ObjSheet.Sprites, filledCell(id, (frame*5+x)*8, y*8, frame+1))
				pose.Parts = append(pose.Parts, assetsync.PosePartRecord{Tile: id, X: x * 8, Y: y * 8})
			}
		}
		m.Poses.Poses = append(m.Poses.Poses, pose)
	}
	before := posePagePreviews(m)
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	if len(m.Animation.Pages) != 1 || !reflect.DeepEqual(before, posePagePreviews(m)) {
		t.Fatal("four 40x80 frames did not fit intact on one 512x320 board")
	}
	for id, pose := range m.Poses.Poses {
		_, _, width, height, ok := PoseSheetBounds(pose, objPageCols, objPageRows)
		if !ok || width != 5 || height != 10 {
			t.Fatalf("pose %d was scattered: source rectangle %dx%d", id, width, height)
		}
	}
}

func TestNormalizeObjectPagesPreservesUnregisteredArtworkLayout(t *testing.T) {
	m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 128, Height: 512}, Poses: assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "registered", Parts: []assetsync.PosePartRecord{{Tile: 0}, {Tile: 0, X: 8, FlipX: true}}}}}, Animation: assetsync.AnimationWorkspace{Version: 1, Pages: []assetsync.AnimationPage{
		{Name: "First", Width: 128, Height: 256}, {Name: "Work in progress", Y: 256, Width: 128, Height: 256},
	}}}
	m.ObjSheet.Sprites = append(m.ObjSheet.Sprites, filledCell(0, 0, 0, 3))
	// An unfinished 40x80 figure on page two has no pose references. Its full
	// pixel layout, including transparent gaps, still belongs to the author.
	for y := 0; y < 10; y++ {
		for x := 0; x < 5; x++ {
			if x == 2 && y > 3 {
				continue
			}
			px, py := 40+x*8, 256+72+y*8
			sprite := filledCell(py/8*16+px/8, px, py, 1+(x+y)%14)
			if x == 0 && y == 0 {
				sprite.Flags = 99
			}
			m.ObjSheet.Sprites = append(m.ObjSheet.Sprites, sprite)
		}
	}
	before := *m.ObjSheet
	beforePoses := posePagePreviews(m)
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	if len(m.Animation.Pages) != 1 || !reflect.DeepEqual(beforePoses, posePagePreviews(m)) {
		t.Fatal("legacy pages did not fit together or registered pose pixels changed")
	}
	anchorX, anchorY := -1, -1
	for _, sprite := range m.ObjSheet.Sprites {
		if sprite.Flags == 99 {
			anchorX, anchorY = sprite.X, sprite.Y
		}
	}
	if anchorX < 0 || anchorY < 0 {
		t.Fatal("unfinished artwork anchor was lost")
	}
	for y := 0; y < 80; y++ {
		for x := 0; x < 40; x++ {
			if got, want := sheetPixel(*m.ObjSheet, anchorX+x, anchorY+y), sheetPixel(before, 40+x, 328+y); got != want {
				t.Fatalf("unfinished figure pixel (%d,%d) = %d, want %d; artwork was rearranged", x, y, got, want)
			}
		}
	}
	if parts := m.Poses.Poses[0].Parts; parts[0].Tile != parts[1].Tile {
		t.Fatal("page preservation duplicated a shared source tile")
	}
}

func fullObjectPageBankFixture() *Model {
	m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 2048, Height: 2048, Sprites: []assetsync.SpriteRecord{filledCell(65535, 2040, 2040, 7)}}, Poses: assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Parts: []assetsync.PosePartRecord{{Tile: 0}, {Tile: 15}}}, {},
	}}}
	for id := 0; id < 65536; id++ {
		if id != 0 && id != 15 {
			m.Poses.Poses[1].Parts = append(m.Poses.Poses[1].Parts, assetsync.PosePartRecord{Tile: id})
		}
	}
	return m
}

func TestNormalizeObjectPagesFallsBackWithoutLosingFullBank(t *testing.T) {
	m := fullObjectPageBankFixture()
	// Preserving all 128 legacy page rectangles would waste eight rows per
	// target board. The fallback must reclaim that padding to retain every cell.
	for y := 0; y < 2048; y += 256 {
		for x := 0; x < 2048; x += 128 {
			m.Animation.Pages = append(m.Animation.Pages, assetsync.AnimationPage{Name: fmt.Sprintf("legacy %d", len(m.Animation.Pages)+1), X: x, Y: y, Width: 128, Height: 256})
		}
	}
	// A whole rectangle for tiles0/15 wastes14 cells; the compact fallback
	// must recover that space instead of losing any of the65536 references.
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	if len(m.Animation.Pages) != 26 || sheetPixel(*m.ObjSheet, 511, 8191) != 7 {
		t.Fatal("full-bank fallback lost capacity or the last tile's pixels")
	}
	seen := make(map[int]bool)
	for _, pose := range m.Poses.Poses {
		for _, part := range pose.Parts {
			seen[part.Tile] = true
		}
	}
	if len(seen) != 65536 {
		t.Fatalf("preserved %d of65536 logical tile references", len(seen))
	}
}

func TestAddObjectPageCapacityFailureDoesNotMutateModel(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		m := fullObjectPageBankFixture()
		if initialized {
			if err := m.NormalizeObjPages(); err != nil {
				t.Fatal(err)
			}
		}
		before := objectPageSnapshot(t, m)
		if _, err := m.AddObjPage(); err == nil {
			t.Fatal("adding a page beyond65536 tiles succeeded")
		}
		if !bytes.Equal(before, objectPageSnapshot(t, m)) {
			t.Fatalf("failed add mutated the model (normalized=%v)", initialized)
		}
	}
}

func TestAddObjectPagesUsesAllSlotsWithSmallerTerminalPage(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateObjSheet(ObjPageWidth, ObjPageHeight); err != nil {
		t.Fatal(err)
	}
	if err := m.SetObjSpritePixel(2559, 7, 7, 6); err != nil {
		t.Fatal(err)
	}
	m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "first", Parts: []assetsync.PosePartRecord{{Tile: 2559, X: -8, FlipX: true}}}}}
	firstPose := ComposePose(*m.ObjSheet, m.Poses.Poses[0])
	for i := 1; i < 26; i++ {
		width, height := m.NextObjPageDimensions()
		wantHeight := 320
		if i == 25 {
			wantHeight = 192
		}
		if width != 512 || height != wantHeight {
			t.Fatalf("next page %d = %dx%d, want 512x%d", i+1, width, height, wantHeight)
		}
		index, err := m.AddObjPage()
		if err != nil || index != i {
			t.Fatalf("add page %d = %d, %v", i+1, index, err)
		}
		page := m.Animation.Pages[i]
		if page.X != 0 || page.Y != i*320 || page.Width != 512 || page.Height != wantHeight {
			t.Fatalf("page %d = %+v", i+1, page)
		}
		if m.Poses.Poses[0].Parts[0].Tile != 2559 || !reflect.DeepEqual(firstPose, ComposePose(*m.ObjSheet, m.Poses.Poses[0])) {
			t.Fatalf("adding page %d moved existing source tiles or changed pose pixels", i+1)
		}
	}
	if m.ObjSheet.Width != 512 || m.ObjSheet.Height != 8192 || m.ObjSheet.Width/8*(m.ObjSheet.Height/8) != 65536 {
		t.Fatalf("full source bank = %dx%d", m.ObjSheet.Width, m.ObjSheet.Height)
	}
	if err := m.SetObjSpritePixel(65535, 7, 7, 7); err != nil {
		t.Fatalf("painting the final tile: %v", err)
	}
	if got := sheetPixel(*m.ObjSheet, 511, 8191); got != 7 {
		t.Fatalf("final pixel = %d, want 7", got)
	}
	if err := m.SetObjSpritePixel(65536, 0, 0, 7); err == nil {
		t.Fatal("painting past the terminal page succeeded")
	}
	m.Poses.Poses = append(m.Poses.Poses, assetsync.PoseRecord{Name: "last", Parts: []assetsync.PosePartRecord{{Tile: 65535, X: 8, Y: 16, FlipY: true}}})
	loop := false
	m.Animation.Clips = []assetsync.AnimationClip{{Name: "edges", Frames: []int{1, 0, 1}, FPS: 12, Loop: &loop}}
	if width, height := m.NextObjPageDimensions(); width != 0 || height != 0 {
		t.Fatalf("full bank offers another %dx%d page", width, height)
	}
	before := objectPageSnapshot(t, m)
	if _, err := m.AddObjPage(); err == nil || !bytes.Equal(before, objectPageSnapshot(t, m)) {
		t.Fatal("adding past the terminal page succeeded or changed the model")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, reloaded)
	if !reflect.DeepEqual(m.ObjSheet, reloaded.ObjSheet) || !reflect.DeepEqual(m.Poses, reloaded.Poses) || !reflect.DeepEqual(m.Animation, reloaded.Animation) {
		t.Fatal("saving/reopening repacked the terminal page or changed pose/clip data")
	}
}

func TestObjectPagePackingReservesFullWidthRowsAndTerminalCapacity(t *testing.T) {
	needed := make([]int, 65536)
	groups := make([][]int, 26)
	for id := range needed {
		needed[id] = id
		groups[id/2560] = append(groups[id/2560], id)
	}
	mapping, count, ok := packObjPageCells(64, 65536, needed, groups, true)
	if !ok || count != 26 {
		t.Fatalf("full-width rectangular packing = %d pages, success %v", count, ok)
	}
	for id, target := range mapping {
		if target != id {
			t.Fatalf("full-width rectangle tile %d mapped to %d; rows overlap or capacity was lost", id, target)
		}
	}
}

func TestEnsureObjectPagesRecognizesTerminalPageWithoutRepacking(t *testing.T) {
	m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 512, Height: 8192, Sprites: []assetsync.SpriteRecord{filledCell(65535, 504, 8184, 7)}}}
	beforeSheet := objectPageSnapshot(t, m.ObjSheet)
	m.EnsureObjPages()
	assertUniformObjectPages(t, m)
	if len(m.Animation.Pages) != 26 || m.Animation.Pages[25].Height != 192 || !bytes.Equal(beforeSheet, objectPageSnapshot(t, m.ObjSheet)) {
		t.Fatal("ensuring pages lost the terminal board or changed source art")
	}
	before := objectPageSnapshot(t, m)
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	m.EnsureObjPages()
	if !bytes.Equal(before, objectPageSnapshot(t, m)) {
		t.Fatal("normalizing a valid terminal board is not idempotent")
	}
}

func TestNormalizeObjectPagesInvalidReferenceIsTransactional(t *testing.T) {
	m := legacyObjectPagesFixture()
	m.Poses.Poses[1].Parts[0].Tile = 8000
	before := objectPageSnapshot(t, m)
	if err := m.NormalizeObjPages(); err == nil || !bytes.Equal(before, objectPageSnapshot(t, m)) {
		t.Fatal("invalid tile reference succeeded or partially changed the model")
	}
}

func TestNormalizeObjectPagesKeepsOldNamesWithoutGeneratedCollisions(t *testing.T) {
	m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 800, Height: 640}, Poses: assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{}}}, Animation: assetsync.AnimationWorkspace{Version: 1, Pages: []assetsync.AnimationPage{{Name: "OBJ 2", Width: 800, Height: 640}}}}
	for id := 0; id < 5000; id++ {
		m.Poses.Poses[0].Parts = append(m.Poses.Poses[0].Parts, assetsync.PosePartRecord{Tile: id})
	}
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	if len(m.Animation.Pages) != 2 || m.Animation.Pages[0].Name != "OBJ 2" {
		t.Fatalf("normalized pages = %+v", m.Animation.Pages)
	}
	seen := make(map[string]bool)
	for _, page := range m.Animation.Pages {
		if seen[page.Name] {
			t.Fatalf("generated page name %q collides with an existing name", page.Name)
		}
		seen[page.Name] = true
	}
}

func TestAddObjectPageIsUniformBlankAndPreservesExistingArt(t *testing.T) {
	m := legacyObjectPagesFixture()
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	beforePoses, beforeSprites, beforeClips := objectPageSnapshot(t, m.Poses), objectPageSnapshot(t, m.ObjSheet.Sprites), objectPageSnapshot(t, m.Animation.Clips)
	index, err := m.AddObjPage()
	if err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	if index != 1 || len(m.Animation.Pages) != 2 || !bytes.Equal(beforePoses, objectPageSnapshot(t, m.Poses)) || !bytes.Equal(beforeSprites, objectPageSnapshot(t, m.ObjSheet.Sprites)) || !bytes.Equal(beforeClips, objectPageSnapshot(t, m.Animation.Clips)) {
		t.Fatal("adding a page changed previous artwork, frames, or clips")
	}
	for y := ObjPageHeight; y < 2*ObjPageHeight; y++ {
		for x := 0; x < ObjPageWidth; x++ {
			if sheetPixel(*m.ObjSheet, x, y) != 0 {
				t.Fatal("new page is not blank")
			}
		}
	}
}

func TestObjectPagesAndClipsNormalizeThroughOpenSave(t *testing.T) {
	dir := editorFixture(t)
	legacy := legacyObjectPagesFixture()
	beforeFrames := posePagePreviews(legacy)
	beforeClips := objectPageSnapshot(t, legacy.Animation.Clips)
	if err := assetsync.SaveObjSheet(dir, legacy.ObjSheet); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SavePoses(dir, &legacy.Poses); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveAnimationWorkspace(dir, &legacy.Animation); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "objsheet.json")
	beforeDisk, _ := os.ReadFile(path)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	afterDisk, _ := os.ReadFile(path)
	if !bytes.Equal(beforeDisk, afterDisk) {
		t.Fatal("opening legacy artwork wrote migration to disk before Save")
	}
	if !reflect.DeepEqual(beforeFrames, posePagePreviews(m)) || !bytes.Equal(beforeClips, objectPageSnapshot(t, m.Animation.Clips)) {
		t.Fatal("opening legacy artwork changed frames or clips")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(objectPageSnapshot(t, m.ObjSheet), objectPageSnapshot(t, reloaded.ObjSheet)) || !bytes.Equal(objectPageSnapshot(t, m.Poses), objectPageSnapshot(t, reloaded.Poses)) || !bytes.Equal(objectPageSnapshot(t, m.Animation), objectPageSnapshot(t, reloaded.Animation)) {
		t.Fatal("save/open changed normalized artwork or clip metadata")
	}
}

func TestNerdoObjectPagesPreserveEveryPosePixelThroughOpenSave(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "nerdo"))); err != nil {
		t.Fatal(err)
	}
	sheet, err := assetsync.LoadObjSheet(dir)
	if err != nil {
		t.Fatal(err)
	}
	poses, err := assetsync.LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sheet == nil || len(poses.Poses) == 0 {
		t.Skip("live Nerdo has no registered poses; synthetic fixtures cover preservation")
	}
	before := posePagePreviews(&Model{ObjSheet: sheet, Poses: *poses})
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, m)
	if !reflect.DeepEqual(before, posePagePreviews(m)) {
		t.Fatal("opening Nerdo changed a composed pose pixel")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, reloaded)
	if !reflect.DeepEqual(before, posePagePreviews(reloaded)) {
		t.Fatal("saving/reopening Nerdo changed a composed pose pixel")
	}
}
