package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestUndoRedoPreservesUnknownAssetFields(t *testing.T) {
	var initial undoSnapshot
	if err := json.Unmarshal([]byte(`{
		"project":{"entry":"game.lua","future":{"large":9007199254740993}},
		"sheet":{"future":true,"sprites":[{"future":"sprite"}]},
		"objSheet":{"future":"objects"},
		"poses":{"future":"poses","poses":[{"future":"pose","parts":[{"future":"part"}]}]},
		"animation":{"future":"animation","pages":[{"future":"page"}],"clips":[{"future":"clip"}]},
		"map":{"future":"map","layers":[{"id":0,"future":"layer"}]},
		"palette":{"future":"palette","colors":[{"future":"color"}]},
		"tileLinks":{"future":"links","links":[{"future":"link"}]}
	}`), &initial); err != nil {
		t.Fatal(err)
	}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	want := encode(initial)
	var history undoStack
	history.reset(initial)
	edited := initial.clone()
	edited.Project.Entry = "changed.lua"
	edited.Poses.Poses[0].Name = "changed"
	history.forcePush(edited)
	restored, ok := history.undo()
	if !ok || !bytes.Equal(encode(restored), want) {
		t.Fatal("undo lost unknown data or shared mutable records with an edit")
	}
	restored, ok = history.redo()
	if !ok || !bytes.Equal(encode(restored), encode(edited)) {
		t.Fatal("redo lost unknown data")
	}
}

func TestEnsureEditableMapPreservesUnknownFields(t *testing.T) {
	g := newTestGame(t, 16, 16, 1, 1)
	var empty assetsync.Map
	if err := json.Unmarshal([]byte(`{"version":2,"width":0,"height":0,"future":true,"layers":[{"id":0,"future":"BG0"},{"id":1},{"id":2},{"id":3}]}`), &empty); err != nil {
		t.Fatal(err)
	}
	g.model.Map = empty
	g.ensureEditableMap()
	if g.model.Map.Width != gbaScreenWidthTiles || g.model.Map.Height != gbaScreenHeightTiles ||
		g.model.Map.UnknownFields != empty.UnknownFields || g.model.Map.Layers[0].UnknownFields != empty.Layers[0].UnknownFields {
		t.Fatal("initializing empty map cells lost authored metadata")
	}
}
