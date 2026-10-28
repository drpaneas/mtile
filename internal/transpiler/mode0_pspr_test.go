package transpiler

import (
	"strings"
	"testing"

	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func psprMode0Opts(parts ...runtimeapi.PosePart) Options {
	return Options{
		Renderer:  RendererMode0,
		SheetW:    8,
		SheetH:    8,
		MapW:      30,
		MapH:      20,
		PoseIndex: []runtimeapi.PoseRange{{Start: 0, Count: uint16(len(parts))}},
		PoseParts: parts,
	}
}

func TestMode0AcceptsProvenPspr(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	_, report, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  pspr(0, 10, 10)
end
`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0PsprUnprovenIDBudgetsLibrary(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	_, _, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  pspr(player, 0, 0)
end
`, opts)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0PsprAllowsSprMix(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	_, _, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  spr(0, 0, 0)
  pspr(0, 8, 0)
end
`, opts)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0PsprAllowsThreeSites(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	_, _, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  pspr(0, 0, 0)
  pspr(0, 8, 0)
  pspr(0, 16, 0)
end
`, opts)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0PsprIfElseBudgetsOneLiveSite(t *testing.T) {
	opts := Options{
		Renderer: RendererMode0,
		SheetW:   8,
		SheetH:   8,
		MapW:     30,
		MapH:     20,
		PoseIndex: []runtimeapi.PoseRange{
			{Start: 0, Count: 1},
			{Start: 1, Count: 1},
		},
		PoseParts: []runtimeapi.PosePart{{Tile: 0}, {Tile: 0}},
	}
	_, report, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  if true then
    pspr(0, 0, 0)
  else
    pspr(1, 8, 0)
  end
end
`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 1 {
		t.Fatalf("if/else alternatives should budget one live OAM entry, got %d", report.Mode0OAMEntries)
	}
}

func TestMode0PsprWindowOverflow(t *testing.T) {
	parts := make([]runtimeapi.PosePart, 513)
	for i := range parts {
		parts[i] = runtimeapi.PosePart{Tile: uint16(i)}
	}
	opts := psprMode0Opts(parts...)
	_, _, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  pspr(0, 0, 0)
end
`, opts)
	if err == nil || !strings.Contains(err.Error(), "[M0_RESOURCE_LIMIT]") || !strings.Contains(err.Error(), "480 available") {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0PsprSpritesheetBankAllowsSset(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	_, _, err := TranspileWithReport("draw.lua", `
function _update()
  sset(0, 0, 1)
end
function _draw()
  cls()
  pspr(0, 0, 0)
end
`, opts)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0PsprObjectBankAllowsSset(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	opts.ObjSheetW = 8
	opts.ObjSheetH = 8
	opts.ObjSheet = make([]byte, 64)
	_, report, err := TranspileWithReport("draw.lua", `
function _update()
  sset(0, 0, 1)
end
function _draw()
  cls()
  pspr(0, 0, 0)
end
`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0PsprFiniteFrameSet(t *testing.T) {
	opts := Options{
		Renderer: RendererMode0,
		SheetW:   8,
		SheetH:   8,
		MapW:     30,
		MapH:     20,
		PoseIndex: []runtimeapi.PoseRange{
			{Start: 0, Count: 1},
			{Start: 1, Count: 1},
			{Start: 2, Count: 1},
			{Start: 3, Count: 1},
		},
		PoseParts: []runtimeapi.PosePart{{Tile: 0}, {Tile: 0}, {Tile: 0}, {Tile: 0}},
	}
	src, report, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  local frame=0
  if counter <=5 then frame=0
  elseif counter <=10 then frame=1
  elseif counter <=15 then frame=2
  else frame=3 end
  pspr(frame, 10, 10)
end
`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(src, "c.PSprSiteV(") {
		t.Fatalf("Mode 0 pspr should emit PSprSiteV:\n%s", src)
	}
}

func TestAutoPsprUnprovenUsesLibraryBound(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	opts.Renderer = RendererAuto
	_, report, err := TranspileWithReport("auto.lua", `
function _draw()
  cls()
  pspr(player, 0, 0)
end
`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Mode0OAMEntries != 1 {
		t.Fatalf("report = %+v", report)
	}
}
