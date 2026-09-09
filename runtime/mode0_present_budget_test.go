//go:build mtile_mode0 && !tinygo

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
)

func TestMode0CommitBudgetCountsOnlyPendingMapTransfers(t *testing.T) {
	for seen := 0; seen < 1<<mode0LayerCount; seen++ {
		for dirty := 0; dirty < 1<<mode0LayerCount; dirty++ {
			c := &Context{paletteReady: true}
			c.mode0MapSeen = uint8(seen)
			want := uint32(22000)
			for layer := 0; layer < mode0LayerCount; layer++ {
				c.mode0MapDirty[layer] = dirty&(1<<layer) != 0
				if seen&dirty&(1<<layer) != 0 {
					want += 9000
				}
			}
			if !mode0SheetImmutable {
				want = mode0CommitReserve
			}
			if got := c.mode0RequiredCommitCycles(); got != want {
				t.Fatalf("seen %b dirty %b: reserve %d want %d", seen, dirty, got, want)
			}
			if want > mode0CommitReserve {
				t.Fatal("warm reserve exceeds complete VBlank window")
			}
		}
	}
}

func TestMode0CommitBudgetRetainsFullWindowForUnprovenWork(t *testing.T) {
	cases := map[string]*Context{
		"nil":               nil,
		"palette-not-ready": {},
		"dirty-palette":     {paletteReady: true, paletteDirty: true},
	}
	cold := &Context{paletteReady: true}
	cold.mode0ColdCycles = 1
	cases["cold-work"] = cold
	for name, c := range cases {
		if got := c.mode0RequiredCommitCycles(); got != mode0CommitReserve {
			t.Fatalf("%s: reserve %d want full %d", name, got, mode0CommitReserve)
		}
	}
}

func TestMode0CommitBudgetAddsBoundedNativeAudioIRQReserve(t *testing.T) {
	warmIRQ := uint32(22000 + audio.NativeServiceCycleBudget)
	if warmIRQ >= mode0CommitReserve {
		t.Fatal("one native audio IRQ already fills the complete VBlank window")
	}
	if !mode0PresentWindowFits(161, warmIRQ) {
		t.Fatal("warm native-audio commit must still fit one line into VBlank")
	}
	c := &Context{paletteReady: true, nativeAudio: new(audio.Controller)}
	want := warmIRQ
	if !mode0SheetImmutable {
		want = mode0CommitReserve
	}
	if got := c.mode0RequiredCommitCycles(); got != want {
		t.Fatalf("clean native-audio reserve %d want %d", got, want)
	}
	warm := uint32(22000)
	for layer := 0; layer < mode0LayerCount; layer++ {
		c.mode0MapSeen |= 1 << uint(layer)
		c.mode0MapDirty[layer] = true
		warm += 9000
	}
	want = mode0CommitWithNativeAudio(warm)
	if !mode0SheetImmutable {
		want = mode0CommitReserve
	}
	if got := c.mode0RequiredCommitCycles(); got != want {
		t.Fatalf("dirty maps plus native audio: reserve %d want %d", got, want)
	}
}

func TestMode0CommitWithNativeAudioCountsOverlappingIRQs(t *testing.T) {
	cases := []struct {
		warm uint32
		irqs uint32
	}{
		{1, 1},
		{22000, 1},
		{audio.NativeServicePeriodCycles - audio.NativeServiceCycleBudget, 1},
		{audio.NativeServicePeriodCycles - audio.NativeServiceCycleBudget + 1, 2},
		{audio.NativeServicePeriodCycles, 2},
	}
	for _, tc := range cases {
		got := mode0CommitWithNativeAudio(tc.warm)
		want := tc.warm + tc.irqs*audio.NativeServiceCycleBudget
		if want < tc.warm || want > mode0CommitReserve {
			want = mode0CommitReserve
		}
		if got != want {
			t.Fatalf("warm %d: reserve %d want %d (%d IRQs)", tc.warm, got, want, tc.irqs)
		}
	}
}
