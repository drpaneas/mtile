package assetsync

import (
	"reflect"
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
)

func TestDefaultFontIsOnlyArcade(t *testing.T) {
	f, err := LoadFont(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "arcade" || len(f.Glyphs) != 95 {
		t.Fatalf("default=%s, %d glyphs; want Arcade ASCII only", f.Name, len(f.Glyphs))
	}
	compiled, err := CompileFont(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Extra) != 0 || !reflect.DeepEqual(compiled, rt.DefaultFont()) {
		t.Fatal("runtime and compiler defaults must both be the same ASCII-only Arcade font")
	}
	for _, name := range []string{"greek", "german", "international"} {
		selected, err := LoadFont("", name)
		if err != nil {
			t.Fatal(err)
		}
		if len(selected.Glyphs) <= 95 {
			t.Fatalf("explicit %s selection lost its language glyphs", name)
		}
	}
}
