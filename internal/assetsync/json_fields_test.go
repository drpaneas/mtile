package assetsync

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestAssetRecordsPreserveUnknownJSON(t *testing.T) {
	// Test both codecs: authored files use the strict v2 decoder, while editor
	// undo snapshots and several callers use encoding/json directly.
	for _, value := range []any{
		&Project{}, &RGB15{}, &Palette{}, &SpriteRecord{}, &Spritesheet{},
		&PosePartRecord{}, &PoseRecord{}, &PoseColorVariant{}, &PoseColorSwap{}, &PoseFile{},
		&MapLayer{}, &Map{}, &AnimationWorkspace{}, &AnimationPage{}, &AnimationClip{},
		&TileLink{}, &TileLinkFile{},
	} {
		t.Run(reflect.TypeOf(value).Elem().Name(), func(t *testing.T) {
			const source = `{"future":{"integer":9007199254740993,"decimal":1.234567890123456789,"array":[null,true,"café 🎮",{"x":1}]}}`
			for _, decode := range []func([]byte, any) error{
				func(raw []byte, dst any) error { return decodeAuthoredJSON(raw, dst, false) },
				json.Unmarshal,
			} {
				target := reflect.New(reflect.TypeOf(value).Elem()).Interface()
				if err := decode([]byte(source), target); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					raw, err := json.MarshalIndent(target, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					var before, after map[string]json.RawMessage
					if err := json.Unmarshal([]byte(source), &before); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(raw, &after); err != nil {
						t.Fatal(err)
					}
					var compact bytes.Buffer
					if err := json.Compact(&compact, after["future"]); err != nil {
						t.Fatalf("unknown field lost: %s (%v)", raw, err)
					}
					if !bytes.Equal(compact.Bytes(), before["future"]) {
						t.Fatalf("unknown value changed: %s", raw)
					}
					target = reflect.New(reflect.TypeOf(value).Elem()).Interface()
					if err := decode(raw, target); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestUnknownJSONDoesNotRestoreRemovedKnownFields(t *testing.T) {
	var project Project
	if err := decodeAuthoredJSON([]byte(`{"ENTRY":"game.lua","notes":null}`), &project, false); err != nil {
		t.Fatal(err)
	}
	project.Entry = ""
	raw, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["ENTRY"]; ok {
		t.Fatalf("case alias returned as an unknown field: %s", raw)
	}
	if _, ok := fields["entry"]; ok || string(fields["notes"]) != "null" {
		t.Fatalf("known removal or unknown null changed: %s", raw)
	}
}

func TestUnknownJSONNullKeepsExistingRecord(t *testing.T) {
	project := Project{ActivePalette: "aurora", Renderer: "auto"}
	if err := decodeAuthoredJSON([]byte(`{"notes":{"keep":true},"ENTRY":null}`), &project, false); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeAuthoredJSON([]byte(`null`), &project, false); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || project.ActivePalette != "aurora" || project.Renderer != "auto" {
		t.Fatalf("null/default semantics changed: %s -> %s", before, after)
	}
}
