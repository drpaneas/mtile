package luaparser

import "testing"

func TestDirectSoundLuaCallShapes(t *testing.T) {
	for _, source := range []string{`pcm_music(0)`, `pcm_music(0, false)`, `pcm_music(-1)`, `sample(0, 0.75, 160)`, `sample(-1)`} {
		chunk, err := ParseString("audio.lua", source)
		if err != nil || len(chunk.Body) != 1 {
			t.Fatalf("%s: %v", source, err)
		}
	}
}
