package main

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestCommonSceneKeepsIdenticalTextAndCoordinates(t *testing.T) {
	before := workload("unicode", 6, 8)
	if before != workload("unicode", 8, 12) {
		t.Fatal("common scene changed workload between sizes")
	}
	if strings.Count(before, " print(") != 8 {
		t.Fatal("common scene must have exactly eight text calls")
	}
	for _, name := range []string{"empty", "hud", "unicode", "dense", "edges"} {
		for _, cell := range [][2]int{{6, 8}, {8, 12}} {
			if _, err := luaparser.ParseString("probe.lua", workload(name, cell[0], cell[1])); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestMode0FixturesKeepExistingAcceptedCallShapes(t *testing.T) {
	for _, name := range []string{"empty", "hud", "edges"} {
		_, report, err := transpiler.TranspileWithReport("probe.lua", workload(name, 6, 8), transpiler.Options{Renderer: transpiler.RendererMode0, SheetW: 8, SheetH: 8})
		if err != nil {
			t.Fatal(err)
		}
		if report.Selected != transpiler.RendererMode0 {
			t.Fatal("Mode0 fixture became unsupported")
		}
	}
}
func TestDenseSceneReportsReducedCapacity(t *testing.T) {
	if got := strings.Count(workload("dense", 6, 8), " print("); got != 20 {
		t.Fatal(got)
	}
	if got := strings.Count(workload("dense", 6, 10), " print("); got != 16 {
		t.Fatal(got)
	}
	if got := strings.Count(workload("dense", 8, 12), " print("); got != 13 {
		t.Fatal(got)
	}
}
func TestPercentilePreservesInputAndSelectsMeasuredValues(t *testing.T) {
	values := []uint32{30, 10, 20, 40, 50}
	if got := percentile(values, 50); got != 30 {
		t.Fatal(got)
	}
	if values[0] != 30 {
		t.Fatal("percentile mutated captured samples")
	}
}
