//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This separately proves the native register/OAM path. The ordinary renderbench
// command measures the uninstrumented example and compares every RGB pixel.
// No probe, runtime helper, or extra Lua API enters the editable example.
func TestParallaxRectanglesHardwareDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_PARALLAX_DIAGNOSTIC") != "1" {
		t.Skip("set MTILE_PARALLAX_DIAGNOSTIC=1 for native four-BG/OBJ register evidence")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.MkdirTemp("", "mtile-parallax-hardware-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
	project := filepath.Join(out, "project")
	if err := tileCanvasCopyTree(filepath.Join(repository, "testdata", "projects", "parallax-rectangles"), project, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "game.lua")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source = append([]byte("probe_display=0 probe_priority=0 probe_objects=0\n"), source...)
	if err := os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", filepath.Join(repository, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork = true, true
	wrapper := filepath.Join(out, "tinygo-register-probe")
	python := "#!/usr/bin/env python3\nimport os,re,sys\nfrom pathlib import Path\nreal=" + strconv.Quote(service.TinyGoBin) + `
if len(sys.argv)>1 and sys.argv[1]=='build':
    path=Path('main.go')
    text=path.read_text()
    if 'func parallaxProbeDisplay()' in text:
        os.execv(real,[real]+sys.argv[1:])
    text=text.replace('import (','import (\n"github.com/drpaneas/mtile/runtime/hw"',1)
    for name in ('display','priority','objects'):
        text,count=re.subn(r'debugMailbox.Add\("probe_'+name+r'",[^\n]*',
            'debugMailbox.Add("probe_'+name+'", parallaxProbe'+name.title()+'())',text)
        if count!=1:
            raise RuntimeError('missing reserved '+name+' watch')
    text+='''
func parallaxProbeDisplay() rt.Value {
    return rt.NumVal(rt.Number(hw.GetReg16(hw.DISPCNT)))
}
func parallaxProbePriority() rt.Value {
    var value uint32
    for i:=0;i<4;i++ { value |= uint32(hw.GetReg16(hw.BG0CNT+uintptr(i*2))&3)<<uint(i*2) }
    return rt.NumVal(rt.Number(value))
}
func parallaxProbeObjects() rt.Value {
    var value uint32
    for i:=0;i<128;i++ {
        at:=hw.OAM+uintptr(i*8)
        a0,a1,a2:=hw.GetReg16(at),hw.GetReg16(at+2),hw.GetReg16(at+4)
        if a0&0x0300==0x0200 { continue }
        value++
        if a0&255==136 && a1&511==116 && a2&1023==10 { value|=1<<8 }
    }
    return rt.NumVal(rt.Number(value))
}
'''
    path.write_text(text)
os.execv(real,[real]+sys.argv[1:])
`
	if err := os.WriteFile(wrapper, []byte(python), 0755); err != nil {
		t.Fatal(err)
	}
	service.TinyGoBin = wrapper
	result, captured, err := buildAndCapture(service, transpiler.RendererMode0, project,
		filepath.Join(repository, "build", "player", "mtile-player"), out, 20, 260)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("linked IWRAM=%d/%d EWRAM=%d free=%d", result.IWRAMUsed, build.IWRAMCapacity, result.EWRAMUsed, result.EWRAMFree)
	if generated, err := os.ReadFile(filepath.Join(result.WorkDir, "main.go")); err == nil {
		if err := os.WriteFile(filepath.Join(out, "generated.go"), generated, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for index, watches := range captured.watches {
		values := make(map[string]uint32, len(watches))
		for _, watch := range watches {
			values[watch.Name] = uint32(watch.Raw)
		}
		if values["probe_display"]&0x1f87 != 0x1f00 ||
			values["probe_priority"] != 0x1b || values["probe_objects"] != 43|1<<8 {
			t.Fatalf("native registers/OAM mismatch at sample%d: %+v", index, values)
		}
	}
	data, err := json.MarshalIndent(captured.watches, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "register-watches.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePNG(filepath.Join(out, "mode0-final.png"), captured.lastRGB); err != nil {
		t.Fatal(err)
	}
	// BGxHOFS/BGxVOFS are write-only on GBA. The independent pixel-parity
	// capture proves their visible scroll result; never read them as evidence.
	t.Log(fmt.Sprintf("verified %d samples: Mode0, all four BGs, priorities3/2/1/0, 43 active OBJs including actor", len(captured.watches)))
}
