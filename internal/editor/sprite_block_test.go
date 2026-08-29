package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func spriteBlockTestModel() *Model {
	colors := make([]assetsync.RGB15, 16)
	return &Model{
		Sheet:         assetsync.Spritesheet{Width: 32, Height: 32},
		ActivePalette: assetsync.Palette{Name: "test", Colors: colors},
	}
}

func spriteRecordByID(sheet assetsync.Spritesheet, id int) (assetsync.SpriteRecord, bool) {
	for _, sprite := range sheet.Sprites {
		if sprite.ID == id {
			return sprite, true
		}
	}
	return assetsync.SpriteRecord{}, false
}

func TestSetSpritePixelBlockWritesTilesAtomicallyAndPreservesFlags(t *testing.T) {
	m := spriteBlockTestModel()
	if err := m.SetSpriteFlag(5, 3, true); err != nil {
		t.Fatal(err)
	}
	pixels := make([]int, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			pixels[y*16+x] = 1 + (y/8)*2 + x/8
		}
	}

	changed, err := m.SetSpritePixelBlock(1, 1, 16, 16, pixels)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("non-empty block reported no change")
	}
	for _, check := range []struct {
		id, color int
	}{{5, 1}, {6, 2}, {9, 3}, {10, 4}} {
		sprite, ok := spriteRecordByID(m.Sheet, check.id)
		if !ok || sprite.Pixels[0][0] != check.color {
			t.Fatalf("sprite %d = %#v, want color %d", check.id, sprite, check.color)
		}
	}
	if sprite, _ := spriteRecordByID(m.Sheet, 5); sprite.Flags&(1<<3) == 0 {
		t.Fatalf("paste cleared existing flags: %#08b", sprite.Flags)
	}
}

func TestSetSpritePixelBlockRejectsReservedAndBoundsWithoutMutation(t *testing.T) {
	m := spriteBlockTestModel()
	pixels := make([]int, 16*8)
	for i := range pixels {
		pixels[i] = 1
	}

	for _, test := range []struct {
		name          string
		col, row      int
		wantErrorText string
	}{
		{name: "reserved", col: 0, row: 0, wantErrorText: "sprite 0"},
		{name: "bounds", col: 3, row: 3, wantErrorText: "exceeds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := cloneSpritesheet(m.Sheet)
			changed, err := m.SetSpritePixelBlock(test.col, test.row, 16, 8, pixels)
			if err == nil || !strings.Contains(err.Error(), test.wantErrorText) {
				t.Fatalf("error = %v, want text %q", err, test.wantErrorText)
			}
			if changed {
				t.Fatal("rejected block reported a change")
			}
			if !reflect.DeepEqual(m.Sheet, before) {
				t.Fatal("rejected block partially mutated the spritesheet")
			}
		})
	}
}

func TestSetSpritePixelBlockKeepsTransparentUnauthoredTilesSparse(t *testing.T) {
	m := spriteBlockTestModel()
	changed, err := m.SetSpritePixelBlock(1, 0, 8, 8, make([]int, 64))
	if err != nil {
		t.Fatal(err)
	}
	if changed || len(m.Sheet.Sprites) != 0 {
		t.Fatalf("empty paste changed=%t sprites=%d, want no authored record", changed, len(m.Sheet.Sprites))
	}
}
