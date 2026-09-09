package transpiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCoinQuestLessonsRemainHardwareRendererEligible(t *testing.T) {
	root := filepath.Join("..", "build", "templates", "coin-quest")
	lessons, err := filepath.Glob(filepath.Join(root, "lessons", "*.lua"))
	if err != nil || len(lessons) != 4 {
		t.Fatalf("expected four checkpoints: %v, %v", lessons, err)
	}
	for _, path := range append(lessons, filepath.Join(root, "game.lua")) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			_, report, err := TranspileWithReport("game.lua", string(source), Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
			if err != nil {
				t.Fatal(err)
			}
			if report.Selected != RendererMode0 {
				t.Fatalf("beginner checkpoint fell back from hardware rendering: %+v", report)
			}
		})
	}
}

func TestCoinQuestMovementPickupWinAndRestart(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "build", "templates", "coin-quest", "game.lua"))
	if err != nil {
		t.Fatal(err)
	}
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func TestPlay(t *testing.T) {
 var g Game; var c rt.Context; g.Init(&c)
 step := func(keys uint16) { hw.SetKeyInputForTest(0x03ff &^ keys); c.PollInput(); c.AdvanceTick(); g.Update(&c) }
 for i:=0;i<150;i++ { step(0x20|0x40) }
 if g.player_x!=rt.FromInt(8)||g.player_y!=rt.FromInt(32) { t.Fatal("top-left boundary") }
 for i:=0;i<150;i++ { step(0x10|0x80) }
 if g.player_x!=rt.FromInt(224)||g.player_y!=rt.FromInt(128) { t.Fatal("bottom-right boundary") }
 g.restart(&c)
 g.player_x=g.coin_x+rt.FromInt(8); g.player_y=g.coin_y; step(0)
 if g.score!=0 { t.Fatal("touching edges counted as overlap") }
 for score:=1;score<=5;score++ {
  g.player_x=g.coin_x; g.player_y=g.coin_y; step(0)
  if g.score!=rt.FromInt(score) { t.Fatalf("pickup %d: score=%v",score,g.score) }
  step(0)
  if g.score!=rt.FromInt(score) { t.Fatal("one pickup counted twice") }
  if g.coin_x<rt.FromInt(8)||g.coin_x>rt.FromInt(224)||g.coin_y<rt.FromInt(32)||g.coin_y>rt.FromInt(128) { t.Fatal("unreachable coin") }
 }
 x,y:=g.player_x,g.player_y;step(0x10)
 if g.player_x!=x||g.player_y!=y||g.score!=rt.FromInt(5) { t.Fatal("game did not stop after winning") }
 step(2) // B must not restart: the on-screen prompt says A.
 if g.score!=rt.FromInt(5) { t.Fatal("B incorrectly restarts") }
 step(1) // Physical GBA A, also Z in the editor preview.
 if g.score!=0||g.player_x!=rt.FromInt(24)||g.player_y!=rt.FromInt(72)||g.coin_x!=rt.FromInt(112)||g.coin_y!=rt.FromInt(72) { t.Fatal("A did not reset all game state") }
}
`
	for _, renderer := range []Renderer{RendererMode0, RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			generated, err := Transpile("game.lua", string(source), Options{Renderer: renderer, SheetW: 240, SheetH: 160})
			if err != nil {
				t.Fatal(err)
			}
			dir := writeGeneratedPackage(t, generated)
			if err := os.WriteFile(filepath.Join(dir, "play_test.go"), []byte(harness), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"test", "-run=TestPlay"}
			if renderer == RendererMode0 {
				args = append(args, "-tags=mtile_mode0,mtile_mode0_obj,mtile_mode0_text")
			}
			cmd := exec.Command("go", append(args, "./...")...)
			cmd.Dir, cmd.Env = dir, generatedGoCacheEnv()
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("playthrough failed: %v\n%s", err, output)
			}
		})
	}
}
