package build

import (
	"encoding/json"
	"github.com/drpaneas/mtile/internal/transpiler"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectAudioTargetAndConditionalLink(t *testing.T) {
	for _, mode := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0} {
		report := transpiler.RendererReport{Selected: mode}
		if strings.Contains(runtimeBuildTags(report), "mtile_pcm") {
			t.Fatal("unused PCM linked")
		}
		report.PCMAudioLinked = true
		if !strings.Contains(runtimeBuildTags(report), "mtile_pcm") {
			t.Fatal("PCM not linked")
		}
	}
	runtimeDir, err := filepath.Abs("../../runtime")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{0x18, 0x19} {
		dir := filepath.Join(t.TempDir(), "with spaces")
		if err = os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "target.json")
		if err = os.WriteFile(path, []byte(`{"extra-files":["startup.s"],"cflags":["-Werror"]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err = addDirectAudioTarget(dir, runtimeDir, path, version); err != nil {
			t.Fatal(err)
		}
		var target struct {
			Extra []string `json:"extra-files"`
			Flags []string `json:"cflags"`
		}
		raw, _ := os.ReadFile(path)
		if err = json.Unmarshal(raw, &target); err != nil {
			t.Fatal(err)
		}
		if len(target.Extra) != 8 || !strings.Contains(strings.Join(target.Flags, " "), "gnu23") {
			t.Fatal("incomplete Maxmod target")
		}
		if strings.Contains(strings.Join(target.Flags, " "), "DEVKITPRO_COMPAT") != (version == 0x18) {
			t.Fatal("wrong MAS layout")
		}
	}
}
