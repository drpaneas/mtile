package transpiler

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func directAudioFixture(t *testing.T) *assetsync.DirectAudioAssets {
	t.Helper()
	a, err := assetsync.LoadDirectAudio("../../examples/imported-audio")
	if err != nil || a == nil {
		t.Fatalf("fixture: %v", err)
	}
	return a
}

func TestDirectAudioTranspilesInBothRenderers(t *testing.T) {
	for _, mode := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		t.Run(mode.String(), func(t *testing.T) {
			opts := nativeTranspilerOptions(t)
			opts.DirectAudio = directAudioFixture(t)
			opts.Renderer = mode
			opts.SheetW, opts.SheetH = 8, 8
			got, report, err := TranspileWithReport("imported.lua", `function _init() pcm_music(0) sample(0,0.5,180) gba_note(1,69,0,24) end
function _draw() cls(1) end`, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !report.PCMAudioLinked || !report.NativeAudioLinked {
				t.Fatalf("audio not linked: %+v", report)
			}
			if report.HeapLinked {
				t.Fatal("fixed audio commands unnecessarily retain the Lua heap")
			}
			for _, want := range []string{"pcmBankROM", "pcmMusicIDs", "pcmSampleIDs", "ctx.AttachPCMAudio", "c.PCMMusicV(", "c.SampleV("} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %s", want)
				}
			}
			if report.Selected == RendererMode0 {
				dir := writeGeneratedPackage(t, got)
				cmd := exec.Command("go", "test", "-tags", "mtile_mode0,mtile_mode0_layers1", "./...")
				cmd.Dir = dir
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("Mode 0 compile: %v\n%s", err, output)
				}
			} else {
				compileGenerated(t, got)
			}
		})
	}
}

func TestDirectAudioRejectsStaticMistakesAndElidesUnusedBank(t *testing.T) {
	for _, source := range []string{`pcm_music(1)`, `pcm_music(-2)`, `sample(0,2)`, `sample(0,1,256)`, `sample(0.5)`} {
		opts := nativeTranspilerOptions(t)
		opts.DirectAudio = directAudioFixture(t)
		if _, err := Transpile("bad.lua", source, opts); err == nil || !strings.Contains(err.Error(), "bad.lua:") {
			t.Fatalf("accepted %s: %v", source, err)
		}
	}
	if _, err := Transpile("missing.lua", "pcm_music(0)", Options{}); err == nil || !strings.Contains(err.Error(), "import-audio") {
		t.Fatal(err)
	}
	got, report, err := TranspileWithReport("silent.lua", "function _draw() cls(1) end", Options{DirectAudio: directAudioFixture(t)})
	if err != nil || report.PCMAudioLinked || strings.Contains(got, "pcmBankROM") {
		t.Fatalf("unused audio linked: %v", err)
	}
}

func TestDirectAudioExampleParsesAndDocumentsBothAPIs(t *testing.T) {
	source, err := os.ReadFile("../../examples/imported-audio/game.lua")
	if err != nil {
		t.Fatal(err)
	}
	opts := nativeTranspilerOptions(t)
	opts.DirectAudio = directAudioFixture(t)
	if _, err = Transpile("example.lua", string(source), opts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sample", "pcm_music"} {
		if BuiltinRendererCompatibility(name) != RendererIndependent {
			t.Fatal(name)
		}
	}
}

func TestDirectAudioDeletedIDsFailAtSourceAndDynamicIDsKeepSentinels(t *testing.T) {
	for _, mode := range []Renderer{RendererMode0, RendererMode4, RendererAuto} {
		opts := nativeTranspilerOptions(t)
		opts.Renderer = mode
		opts.SheetW, opts.SheetH = 8, 8
		opts.DirectAudio = directAudioFixture(t)
		deleted := assetsync.DirectAudioItem{Name: "deleted", BankID: 65535, Deleted: true}
		opts.DirectAudio.Manifest.Version = 2
		opts.DirectAudio.Manifest.Music = append([]assetsync.DirectAudioItem{deleted}, opts.DirectAudio.Manifest.Music...)
		opts.DirectAudio.Manifest.Samples = append([]assetsync.DirectAudioItem{deleted}, opts.DirectAudio.Manifest.Samples...)
		for _, call := range []string{"sample(0)", "pcm_music(0)"} {
			source := "function _init() " + call + " end function _draw() cls(1) end"
			if _, err := Transpile("deleted.lua", source, opts); err == nil || !strings.Contains(err.Error(), "deleted.lua:1:") {
				t.Fatalf("%s accepted deleted ID: %v", mode, err)
			}
		}
		source := `function _init() local i=1 pcm_music(i) sample(i) end function _draw() cls(1) end`
		got, _, err := TranspileWithReport("live.lua", source, opts)
		if err != nil || !strings.Contains(got, "65535,") {
			t.Fatalf("%s: deleted slot mapping lost: %v", mode, err)
		}
	}
}
