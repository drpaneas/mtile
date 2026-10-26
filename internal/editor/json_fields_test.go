package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// Annotate every record, including nested colors, layers, frames, and parts.
// Unique labels let the inventory detect both lost and duplicated metadata.
func annotateUnknownJSON(t *testing.T, label string, raw []byte) []byte {
	t.Helper()
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	var visit func(any, string)
	visit = func(value any, path string) {
		switch value := value.(type) {
		case map[string]any:
			for name, child := range value {
				visit(child, path+"/"+name)
			}
			value["_future"] = map[string]any{"record": path, "large": json.Number("9007199254740993"), "nested": []any{nil, true, "café 🎮"}}
		case []any:
			for i, child := range value {
				visit(child, fmt.Sprintf("%s/%d", path, i))
			}
		}
	}
	visit(value, label)
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func unknownJSONInventory(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]string)
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "_future" {
					fields := child.(map[string]any)
					label := fields["record"].(string)
					if _, exists := result[label]; exists {
						t.Fatalf("metadata duplicated: %s", label)
					}
					encoded, err := json.Marshal(child)
					if err != nil {
						t.Fatal(err)
					}
					result[label] = string(encoded)
				} else {
					visit(child)
				}
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(value)
	return result
}

func annotateRecord(t *testing.T, label string, record any) {
	t.Helper()
	raw := objectPageSnapshot(t, record)
	if err := json.Unmarshal(annotateUnknownJSON(t, label, raw), record); err != nil {
		t.Fatal(err)
	}
}

func TestEditorSavePreservesUnknownJSON(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	m.Project.Entry = "game.lua"
	m.Map, err = assetsync.NewLayeredMap(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Layer order is not semantic; resizing must keep metadata with its ID.
	m.Map.Layers[0], m.Map.Layers[3] = m.Map.Layers[3], m.Map.Layers[0]
	m.ObjSheet = &assetsync.Spritesheet{Width: ObjPageWidth, Height: ObjPageHeight, Sprites: []assetsync.SpriteRecord{filledCell(1, 8, 0, 7), filledCell(2, 16, 0, 0)}}
	m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "one", Parts: []assetsync.PosePartRecord{{Tile: 1}}}},
		Variants: []assetsync.PoseColorVariant{{Name: "fire", Colors: []assetsync.PoseColorSwap{{From: 7, To: 10}}}}}
	m.Animation = assetsync.AnimationWorkspace{Version: 1,
		Pages: []assetsync.AnimationPage{{Name: "OBJ 1", Width: ObjPageWidth, Height: ObjPageHeight}},
		Clips: []assetsync.AnimationClip{{Name: "idle", Frames: []int{0}, FPS: 8}, {Name: "run", Frames: []int{0, 0}, FPS: 12}}}
	if err := m.SetSpritePixel(1, 0, 0, 7); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkTileToPose(2, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkTileToPose(1, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	files := []string{"project.json", "assets/palettes/wide.json", "assets/spritesheet.json", "assets/objsheet.json", "assets/map.json", "assets/poses.json", "assets/animation.json", "assets/tilelinks.json"}
	want := make(map[string]map[string]string)
	for _, file := range files {
		path := filepath.Join(m.Dir, file)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = annotateUnknownJSON(t, file, raw)
		want[file] = unknownJSONInventory(t, raw)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for round := range 2 {
		m, err = Open(m.Dir)
		if err != nil {
			t.Fatal(err)
		}
		if round == 1 {
			m.Project.Entry = ""
			if err := m.SetActiveColor(1, assetsync.RGB15{R: 31}); err != nil {
				t.Fatal(err)
			}
			if err := m.SetSpritePixel(1, 1, 1, 8); err != nil {
				t.Fatal(err)
			}
		}
		layers := make(map[int]assetsync.UnknownFields)
		for _, layer := range m.Map.Layers {
			layers[layer.ID] = layer.UnknownFields
		}
		if err := m.ResizeMap(2, 2); err != nil {
			t.Fatal(err)
		}
		for _, layer := range m.Map.Layers {
			if layer.UnknownFields != layers[layer.ID] {
				t.Fatalf("layer %d inherited another layer's metadata", layer.ID)
			}
			oldLink, _ := m.TileLinkFor(1)
			if err := m.LinkTileToAnimation(1, "idle"); err != nil {
				t.Fatal(err)
			}
			link, _ := m.TileLinkFor(1)
			if link.UnknownFields != oldLink.UnknownFields || link.Pose != nil {
				t.Fatal("changing link target lost metadata or retained the old known target")
			}
			m.Animation.Clips[0], m.Animation.Clips[1] = m.Animation.Clips[1], m.Animation.Clips[0]
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(filepath.Join(m.Dir, file))
			if err != nil {
				t.Fatal(err)
			}
			if got := unknownJSONInventory(t, raw); !reflect.DeepEqual(got, want[file]) {
				t.Fatalf("round %d: %s metadata changed: got %v, want %v", round, file, got, want[file])
			}
			if round == 1 && file == "project.json" && bytes.Contains(raw, []byte(`"entry"`)) {
				t.Fatal("save restored the removed known entry field")
			}
		}
	}
}

func TestObjectTransformsPreserveUnknownJSON(t *testing.T) {
	for _, transform := range []struct {
		name string
		run  func(*Model) error
	}{
		{"resize", func(m *Model) error { return m.ResizeObjSheet(24, 16) }},
		{"normalize", func(m *Model) error { return m.NormalizeObjPages() }},
		{"move", func(m *Model) error { return m.MoveObjSpritePixelBlock(1, 1, 1, 1, 0, 0) }},
	} {
		t.Run(transform.name, func(t *testing.T) {
			m := objectResizeFixture()
			m.ActivePalette = editorPalette("wide", 256)
			m.ObjSheet.Sprites = append(m.ObjSheet.Sprites, filledCell(2, 0, 8, 0))
			m.Poses.Variants = []assetsync.PoseColorVariant{{Name: "fire", Colors: []assetsync.PoseColorSwap{{From: 7, To: 10}}}}
			m.Animation = assetsync.AnimationWorkspace{Version: 1, Pages: []assetsync.AnimationPage{{Name: "old", Width: 16, Height: 16}}}
			annotateRecord(t, "sheet", m.ObjSheet)
			annotateRecord(t, "poses", &m.Poses)
			annotateRecord(t, "animation", &m.Animation)
			before := unknownJSONInventory(t, objectPageSnapshot(t, m))
			if err := transform.run(m); err != nil {
				t.Fatal(err)
			}
			if got := unknownJSONInventory(t, objectPageSnapshot(t, m)); !reflect.DeepEqual(got, before) {
				t.Fatalf("metadata changed: got %v, want %v", got, before)
			}
		})
	}
}

func TestObjectTransformsRejectUnknownMetadataLoss(t *testing.T) {
	t.Run("shrink metadata-only tile", func(t *testing.T) {
		m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 16, Height: 16, Sprites: []assetsync.SpriteRecord{filledCell(3, 8, 8, 0)}}}
		annotateRecord(t, "sheet", m.ObjSheet)
		before := objectPageSnapshot(t, m)
		if err := m.ResizeObjSheet(8, 8); err == nil || !strings.Contains(err.Error(), "unknown metadata") {
			t.Fatalf("shrink error = %v", err)
		}
		if !bytes.Equal(before, objectPageSnapshot(t, m)) {
			t.Fatal("rejected shrink changed the document")
		}
	})
	t.Run("normalization removes annotated page", func(t *testing.T) {
		m := legacyObjectPagesFixture()
		annotateRecord(t, "animation", &m.Animation)
		before := objectPageSnapshot(t, m)
		if err := m.NormalizeObjPages(); err == nil || !strings.Contains(err.Error(), "unknown JSON fields") {
			t.Fatalf("normalization error = %v", err)
		}
		if !bytes.Equal(before, objectPageSnapshot(t, m)) {
			t.Fatal("rejected normalization changed the document")
		}
	})
}

func TestEmptyAssetFilesRetainUnknownJSON(t *testing.T) {
	for _, file := range []string{"poses.json", "animation.json", "tilelinks.json"} {
		t.Run(file, func(t *testing.T) {
			dir := editorFixture(t)
			path := filepath.Join(dir, "assets", file)
			raw := []byte(`{"version":1,"notes":{"keep":true}}`)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				m, err := Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				if err := m.Save(); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Contains(got, []byte(`"keep": true`)) {
					t.Fatalf("empty %s lost unknown metadata: %s, %v", file, got, err)
				}
			}
		})
	}
}

func TestUnknownJSONDoesNotBypassSchemaVersions(t *testing.T) {
	for _, file := range []string{"poses.json", "animation.json", "tilelinks.json", "map.json"} {
		t.Run(file, func(t *testing.T) {
			dir := editorFixture(t)
			path := filepath.Join(dir, "assets", file)
			raw := []byte(`{"version":999,"width":1,"height":1,"cells":[0],"future":true}`)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "version") {
				t.Fatalf("unsupported version was admitted: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("opening an unsupported schema modified the file")
			}
		})
	}
}
