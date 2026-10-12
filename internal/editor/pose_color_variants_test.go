package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestPoseColorVariantCRUDAndPersistence(t *testing.T) {
	m := &Model{Poses: assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "idle"}}}}
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(m.CreatePoseColorVariant("fire"))
	check(m.SetPoseColorReplacement("fire", 6, 10))
	check(m.DuplicatePoseColorVariant("fire", "ice"))
	check(m.SetPoseColorReplacement("ice", 6, 12))
	if got := m.Poses.Variants[0].Colors[0].To; got != 10 {
		t.Fatalf("duplicate changed original: %d", got)
	}
	check(m.RenamePoseColorVariant("ice", "cold"))
	check(m.RenamePoseColorVariant("cold", "cold"))
	check(m.SetPoseColorReplacement("cold", 7, 8))
	check(m.SetPoseColorReplacement("cold", 6, 6))
	if got := m.Poses.Variants[1].Colors; !reflect.DeepEqual(got, []assetsync.PoseColorSwap{{From: 7, To: 8}}) {
		t.Fatalf("identity removal: %v", got)
	}
	check(m.SetPoseColorReplacement("cold", 9, 9))
	check(m.DeletePoseColorVariant("fire"))
	dir := t.TempDir()
	check(os.MkdirAll(filepath.Join(dir, "assets"), 0755))
	check(assetsync.SavePoses(dir, &m.Poses))
	loaded, err := assetsync.LoadPoses(dir)
	check(err)
	if !reflect.DeepEqual(*loaded, m.Poses) {
		t.Fatalf("persistence: got %#v want %#v", *loaded, m.Poses)
	}
}

func TestPoseColorVariantErrorsDoNotMutate(t *testing.T) {
	m := &Model{Poses: assetsync.PoseFile{Version: 1}}
	if err := m.CreatePoseColorVariant("fire"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPoseColorReplacement("fire", 1, 255); err != nil {
		t.Fatal(err)
	}
	before := assetsync.PoseFile{Version: 1, Variants: []assetsync.PoseColorVariant{{Name: "fire", Colors: []assetsync.PoseColorSwap{{From: 1, To: 255}}}}}
	cases := []func() error{
		func() error { return m.CreatePoseColorVariant("") },
		func() error { return m.CreatePoseColorVariant(" spaced ") },
		func() error { return m.CreatePoseColorVariant(strings.Repeat("a", assetsync.MaxPoseNameBytes+1)) },
		func() error { return m.CreatePoseColorVariant("fire") },
		func() error { return m.DuplicatePoseColorVariant("missing", "new") },
		func() error { return m.DuplicatePoseColorVariant("fire", "fire") },
		func() error { return m.RenamePoseColorVariant("missing", "new") },
		func() error { return m.RenamePoseColorVariant("fire", " ") },
		func() error { return m.DeletePoseColorVariant("missing") },
		func() error { return m.SetPoseColorReplacement("missing", 1, 2) },
		func() error { return m.SetPoseColorReplacement("fire", 0, 2) },
		func() error { return m.SetPoseColorReplacement("fire", 1, 256) },
	}
	for i, call := range cases {
		if err := call(); err == nil {
			t.Fatalf("case %d accepted", i)
		}
		if !reflect.DeepEqual(m.Poses, before) {
			t.Fatalf("case %d mutated variants", i)
		}
	}
	var nilModel *Model
	if err := nilModel.CreatePoseColorVariant("new"); err == nil {
		t.Fatal("nil model accepted")
	}
}
