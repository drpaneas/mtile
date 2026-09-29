package assetsync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedAlignedObjectBankKeepsPixelsAndEmptyBank(t *testing.T) {
	for _, painted := range []bool{false, true} {
		t.Run(fmt.Sprint(painted), func(t *testing.T) {
			dir := t.TempDir()
			sheet, mapData := nativeFixture()
			writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
			obj := Spritesheet{Width: 8, Height: 8}
			size, want := 0, 0
			if painted {
				obj.Sprites = []SpriteRecord{testSprite(0, 0, 0, 8, 8, 7, 0)}
				size, want = 64, 7
			}
			if err := SaveObjSheet(dir, &obj); err != nil {
				t.Fatal(err)
			}
			if err := SavePoses(dir, &PoseFile{Version: 1, Poses: []PoseRecord{{Name: "pose", Parts: []PosePartRecord{{Tile: 0}}}}}); err != nil {
				t.Fatal(err)
			}
			source, err := GenerateGo(dir, "main")
			if err != nil {
				t.Fatal(err)
			}
			genDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(genDir, "assets.go"), source, 0600); err != nil {
				t.Fatal(err)
			}
			runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
			if err != nil {
				t.Fatal(err)
			}
			mod := fmt.Sprintf("module generated\n\ngo 1.27.1\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\nreplace github.com/drpaneas/mtile/runtime => %s\n", runtimeDir)
			if err := os.WriteFile(filepath.Join(genDir, "go.mod"), []byte(mod), 0600); err != nil {
				t.Fatal(err)
			}
			harness := fmt.Sprintf(`package main
import("testing";"unsafe";rt "github.com/drpaneas/mtile/runtime")
func TestObjectBank(t *testing.T) {
 if unsafe.Alignof(objSheetROM)<4 || unsafe.Offsetof(objSheetROM.Pixels)!=0 {t.Fatal("pixel alignment contract")}
 if len(objSheetROM.Pixels)!=%d {t.Fatal("pixel length changed")}
 var c rt.Context
 if !InitAssets(&c) {t.Fatal("empty/painted object bank rejected")}
 c.Cls(0);c.PSpr(0,16,16,false,false)
 if c.Pget(16,16)!=%d {t.Fatal("generated bank pixel mismatch")}
}`, size, want)
			if err := os.WriteFile(filepath.Join(genDir, "harness_test.go"), []byte(harness), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("go", "test", "-run", "TestObjectBank", "./...")
			cmd.Dir = genDir
			cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mtile-generated-gocache"))
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("generated assets failed: %v\n%s", err, out.String())
			}
		})
	}
}
