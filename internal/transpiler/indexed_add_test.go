package transpiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIndexedAddPreservesArrayAndHashFieldsInBothRenderers(t *testing.T) {
	const source = `
items={10,30,label=99}
function exercise()
  add(items,20,2)
end
function _draw() cls(0) end
`
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestIndexedAdd(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c);g.exercise(&c)
 if c.TableLen(g.items)!=3 {t.Fatal("indexed insertion did not grow array")}
 for i,want:=range []int{10,20,30} {if got:=c.TableGetInt(g.items,i+1).Int();got!=want {t.Fatalf("item%d=%d want%d",i+1,got,want)}}
 if c.TableGet(g.items,c.InternStatic("label")).Int()!=99 {t.Fatal("indexed insertion changed hash fields")}
}
`
	for _, renderer := range []Renderer{RendererMode0, RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			generated, report, err := TranspileWithReport("indexed-add.lua", source, Options{Renderer: renderer, SheetW: 8, SheetH: 8})
			if err != nil {
				t.Fatal(err)
			}
			if report.Selected != renderer || BuiltinRendererCompatibility("add") != RendererIndependent {
				t.Fatal("indexed add changed renderer eligibility")
			}
			dir := writeGeneratedPackage(t, generated)
			if err := os.WriteFile(filepath.Join(dir, "indexed_add_test.go"), []byte(harness), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"test", "-run=TestIndexedAdd"}
			if renderer == RendererMode0 {
				args = append(args, "-tags=mtile_mode0")
			}
			cmd := exec.Command("go", append(args, "./...")...)
			cmd.Dir, cmd.Env = dir, generatedGoCacheEnv()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("generated indexed add: %v\n%s", err, out)
			}
		})
	}
}
