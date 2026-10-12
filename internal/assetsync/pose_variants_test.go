package assetsync

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPoseColorVariantExpansion(t *testing.T) {
	a := &Assets{ObjSheetW: 16, ObjSheetH: 8, ObjSpriteCols: 2, ObjSpriteRows: 1, ObjSpriteN: 2, ObjSheetROM: make([]byte, 128), PoseIndex: []FlatPose{{Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}}, PoseParts: []FlatPosePart{{Tile: 1}}, AnimationFrames: []uint16{0}}
	a.ObjSheetROM[9] = 6
	a.ObjSheetROM[10] = 7
	base := bytes.Clone(a.ObjSheetROM)
	file := PoseFile{Variants: []PoseColorVariant{{Name: "fire", Colors: []PoseColorSwap{{From: 6, To: 7}, {From: 7, To: 9}}}, {Name: "identity"}}}
	if err := attachPoseColorVariants(a, file); err != nil {
		t.Fatal(err)
	}
	if a.PoseVariantBaseCount != 1 || len(a.PoseIndex) != 3 || !reflect.DeepEqual(a.PoseVariantNames, []string{"fire", "identity"}) {
		t.Fatalf("expanded index: %+v", a)
	}
	if !bytes.Equal(a.ObjSheetROM[:128], base) {
		t.Fatal("canonical artwork mutated")
	}
	for variant, want := range [][2]byte{{7, 9}, {6, 7}} {
		pose := a.PoseIndex[variant+1]
		if pose.Count != 1 || pose.FlipOffsetX != 8 || pose.FlipOffsetY != 8 {
			t.Fatalf("variant bounds=%+v", pose)
		}
		tile := int(a.PoseParts[pose.Start].Tile)
		at := tile/2*8*16 + tile%2*8
		if got := [2]byte{a.ObjSheetROM[at+1], a.ObjSheetROM[at+2]}; got != want {
			t.Fatalf("variant %d=%v want %v", variant, got, want)
		}
		if a.ObjSheetROM[at] != 0 {
			t.Fatal("transparent zero remapped")
		}
	}
	if !reflect.DeepEqual(a.AnimationFrames, []uint16{0}) {
		t.Fatal("animation canonical IDs changed")
	}
}

func TestPoseColorVariantSchemaAndVariantOnlyPersistence(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	file := PoseFile{Version: 1, Variants: []PoseColorVariant{{Name: "fire", Colors: []PoseColorSwap{{From: 6, To: 10}}}}}
	if err := SavePoses(dir, &file); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Variants, file.Variants) {
		t.Fatalf("variant-only persistence: got %+v", loaded)
	}
	for _, variants := range [][]PoseColorVariant{
		{{Name: ""}}, {{Name: "x"}, {Name: "x"}},
		{{Name: "x", Colors: []PoseColorSwap{{From: 0, To: 1}}}},
		{{Name: "x", Colors: []PoseColorSwap{{From: 1, To: 256}}}},
		{{Name: "x", Colors: []PoseColorSwap{{From: 1, To: 2}, {From: 1, To: 3}}}},
	} {
		bad := PoseFile{Version: 1, Variants: variants}
		if err := SavePoses(dir, &bad); err == nil {
			t.Fatalf("accepted invalid variants %+v", variants)
		}
	}
	if err := attachPoseColorVariants(&Assets{}, file); err == nil {
		t.Fatal("accepted variants without OBJ sheet")
	}
}
