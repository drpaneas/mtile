package transpiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestSaveExamplePlaythroughAndFailures(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "save-game")
	source, err := os.ReadFile(filepath.Join(dir, "game.lua"))
	if err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func TestPlay(t *testing.T) {
 hw.ResetSRAMForTest(255)
 var g Game;var c rt.Context;var memory rt.SaveMemory;c.AttachSaveMemory(&memory);g.Init(&c)
 if g.status!=rt.IntVal(0){t.Fatal("fresh cart did not report no save")}
 step:=func(keys uint16){hw.SetKeyInputForTest(0x3ff &^ keys);c.PollInput();c.AdvanceTick();g.Update(&c)}
 g.player_x=rt.IntVal(112);g.player_y=rt.IntVal(88);step(0)
 if g.coins!=rt.IntVal(1)||g.coin_x!=rt.IntVal(176){t.Fatal("pickup")}
 if g.__glu_pause_save(&c)!=1||g.status!=rt.IntVal(4)||g.saves!=rt.IntVal(1){t.Fatal("menu save")}
 for i:=0;i<25;i++{step(1)}
 if g.saves!=rt.IntVal(1){t.Fatal("holding A repeatedly writes")}
 step(0);step(0x10);g.__glu_pause_load(&c)
 if g.player_x!=rt.IntVal(112)||g.coins!=rt.IntVal(1)||g.saves!=rt.IntVal(1)||g.status!=rt.IntVal(1){t.Fatal("menu load")}
 g.player_x=rt.IntVal(140);hw.SetSRAMWriteLimitForTest(0);g.store_game(&c)
 if g.player_x!=rt.IntVal(140)||g.saves!=rt.IntVal(1)||g.status!=rt.IntVal(-1){t.Fatal("failed save lost progress or claimed success")}
 var boot Game;var bc rt.Context;var bm rt.SaveMemory;bc.AttachSaveMemory(&bm);boot.Init(&bc)
 if boot.player_x!=rt.IntVal(112)||boot.saves!=rt.IntVal(1){t.Fatal("failure destroyed previous save")}
 hw.SetSRAMWriteLimitForTest(-1);g.store_game(&c)
 if g.saves!=rt.IntVal(2)||g.status!=rt.IntVal(4){t.Fatal("retry")}
 g.new_run(&c)
 if g.coins!=rt.IntVal(0)||g.saves!=rt.IntVal(0){t.Fatal("new run")}
 g.restore_game(&c)
 if g.player_x!=rt.IntVal(140)||g.saves!=rt.IntVal(2)||g.coins!=rt.IntVal(1){t.Fatal("new run erased cartridge save")}
 hw.WriteSRAMByte(0,0);hw.WriteSRAMByte(512,0);g.restore_game(&c)
 if g.player_x!=rt.IntVal(140)||g.saves!=rt.IntVal(2)||g.status!=rt.IntVal(-2){t.Fatal("failed load changed running game")}
}

`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		t.Run(renderer.String(), func(t *testing.T) {
			generated, report, err := TranspileWithReport("game.lua", string(source), Options{
				Renderer: renderer, DisableMonomorphicNumbers: true,
				SheetW: assets.SheetW, SheetH: assets.SheetH, MapW: assets.MapW, MapH: assets.MapH,
				Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM, Palette: assets.PaletteROM[:],
			})
			if err != nil {
				t.Fatal(err)
			}
			if renderer != RendererMode4 && report.Selected != RendererMode0 {
				t.Fatalf("AUTO fallback: %+v", report)
			}
			pkg := writeGeneratedPackage(t, generated)
			if err := os.WriteFile(filepath.Join(pkg, "play_test.go"), []byte(harness), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"test", "-run=Test"}
			if report.Selected == RendererMode0 {
				args = append(args, "-tags=mtile_mode0,mtile_mode0_obj,mtile_mode0_text")
			}
			cmd := exec.Command("go", append(args, "./...")...)
			cmd.Dir, cmd.Env = pkg, generatedGoCacheEnv()
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}
