package transpiler

import (
	"encoding/json"
	"go/format"
	"os"
	"strings"
	"testing"
)

// Website examples must pass the same parser and generator as cartridge source.
// The fixture supplies the assets explicitly required by the examples. This
// checks compilation, not visual parity or runtime performance.
func TestWebsiteAPIExamples(t *testing.T) {
	data, err := os.ReadFile("../../website/api.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct{ Name, Example string }
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.Name, func(t *testing.T) {
			opts := animationOptions()
			opts.PoseNames = []string{"hero_idle", "hero_hold"}
			opts.Renderer = RendererMode4
			opts.NativeAudio = nativeTranspilerOptions(t).NativeAudio
			if entry.Name == "sample" || entry.Name == "pcm_music" {
				opts.DirectAudio = directAudioFixture(t)
			}
			opts.PoseVariantBaseCount = len(opts.PoseIndex)
			opts.PoseVariantNames = []string{"alternate"}
			opts.PoseIndex = append(opts.PoseIndex, opts.PoseIndex...)
			source := "x=112\nhealth=3\nscore=0\n" + entry.Example
			if !strings.Contains(source, "function _draw()") {
				source += "\nfunction _draw() cls(0) end\n"
			}
			got, err := Transpile("website/"+entry.Name+".lua", source, opts)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := format.Source([]byte(got)); err != nil {
				t.Fatalf("invalid generated Go: %v", err)
			}
		})
	}
}
