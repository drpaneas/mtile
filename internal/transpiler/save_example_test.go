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
import("testing";"strings";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func TestPlay(t *testing.T) {
 same:=func(c *rt.Context,v rt.Value,s string)bool { n,ok:=c.CompareStrings(v,c.InternStatic(s));return ok&&n==0 }
 hw.ResetSRAMForTest(255)
 var g Game;var c rt.Context;var memory rt.SaveMemory;c.AttachSaveMemory(&memory);g.Init(&c)
 step:=func(keys uint16){hw.SetKeyInputForTest(0x3ff &^ keys);c.PollInput();c.PollPauseMenu();if c.PauseActive(){c.PreparePauseMenu();c.PresentPauseMenu()}else{c.AdvanceTick();g.Update(&c)}}
 press:=func(keys uint16){step(0);step(keys)}
 wait:=func(){for i:=0;i<151;i++{step(0)}}
 for slot:=0;slot<3;slot++ {
  g.selected=rt.IntVal(slot);press(8)
  if g.mode!=rt.IntVal(1){t.Fatal("empty file did not open name registration")}
  g.name_cursor=rt.IntVal(slot);press(1)
  if !same(&c,g.hero_name,string(rune('A'+slot))){t.Fatal("name entry")}
  press(2)
  if !same(&c,g.hero_name,""){t.Fatal("backspace")}
  press(1)
  for i:=0;i<5;i++{press(1)}
  if !same(&c,g.hero_name,strings.Repeat(string(rune('A'+slot)),5)){t.Fatal("five-letter name limit")}
  g.name_cursor=rt.IntVal(26);press(8)
  if g.mode!=rt.IntVal(4)||!c.SavePending(){t.Fatal("registration not committed")}
  wait();if g.mode!=rt.IntVal(0){t.Fatal("registration did not return to files")}
  press(8);if g.mode!=rt.IntVal(2){t.Fatal("menu Start did not load registered file")}
  step(8);if c.PauseActive(){t.Fatal("held file-picker Start paused gameplay")}
  press(8);if !c.PauseActive(){t.Fatal("gameplay Start did not pause")}
  press(8);if c.PauseActive(){t.Fatal("gameplay Start did not resume")}
  g.player_x=rt.IntVal(112);g.player_y=rt.IntVal(88);step(0)
  if g.coins!=rt.IntVal(1){t.Fatal("pickup")}
  press(4);if g.mode!=rt.IntVal(3){t.Fatal("Select game menu")}
  press(8);if g.mode!=rt.IntVal(2)||c.PauseActive(){t.Fatal("game menu Start was intercepted")}
  step(8);if c.PauseActive(){t.Fatal("held game-menu Start paused gameplay")}
  press(4)
  press(1);if g.mode!=rt.IntVal(4)||g.saves!=rt.IntVal(2){t.Fatal("save and continue")}
  for i:=0;i<40;i++{step(1)}
  if g.saves!=rt.IntVal(2){t.Fatal("holding A repeated save")}
  wait();if g.mode!=rt.IntVal(2){t.Fatal("wait did not resume gameplay")}
  g.show_files(&c)
 }
 var boot Game;var bc rt.Context;var bm rt.SaveMemory;bc.AttachSaveMemory(&bm);boot.Init(&bc)
 for slot:=0;slot<3;slot++ {
  boot.active_slot=rt.IntVal(slot)
  if boot.restore_game(&bc)!=rt.IntVal(1)||!same(&bc,boot.hero_name,strings.Repeat(string(rune('A'+slot)),5))||boot.coins!=rt.IntVal(1)||boot.saves!=rt.IntVal(2){t.Fatal("cold slot restore",slot)}
 }
 g.active_slot=rt.IntVal(1);g.restore_game(&c);g.player_x=rt.IntVal(140)
 hw.SetSRAMWriteLimitForTest(0)
 if g.store_game(&c)!=rt.IntVal(-1)||g.player_x!=rt.IntVal(140)||g.saves!=rt.IntVal(2){t.Fatal("failed save changed run")}
 hw.SetSRAMWriteLimitForTest(-1)
 boot.active_slot=rt.IntVal(1);boot.restore_game(&bc)
 if boot.player_x!=rt.IntVal(112){t.Fatal("failed write destroyed old slot")}
 if g.store_game(&c)!=rt.IntVal(1)||g.saves!=rt.IntVal(3){t.Fatal("retry")}
 g.selected=rt.IntVal(1);g.show_files(&c);press(4);press(2)
 if g.mode!=rt.IntVal(0)||g.read_file(&c,rt.IntVal(1))!=rt.IntVal(1){t.Fatal("erase cancellation")}
 press(4);press(1);wait()
 if g.read_file(&c,rt.IntVal(1))!=rt.IntVal(0)||g.read_file(&c,rt.IntVal(0))!=rt.IntVal(1)||g.read_file(&c,rt.IntVal(2))!=rt.IntVal(1){t.Fatal("erase crossed slots")}
 // An invalid application record is reported without replacing live progress.
 for i:=0;i<64;i++{c.SaveSetV(rt.IntVal(i),rt.IntVal(0))};c.SaveSetV(rt.IntVal(0),rt.IntVal(1));c.SaveWriteSlot("mtile-three-files-v1",2)
 g.active_slot=rt.IntVal(2);old:=g.player_x
 if g.restore_game(&c)!=rt.IntVal(-6)||g.player_x!=old{t.Fatal("invalid payload changed live run")}
}
`

	for _, renderer := range []Renderer{RendererMode4, RendererAuto} {
		t.Run(renderer.String(), func(t *testing.T) {
			generated, report, err := TranspileWithReport("game.lua", string(source), Options{
				Renderer: renderer, DisableMonomorphicNumbers: true,
				SheetW: assets.SheetW, SheetH: assets.SheetH, MapW: assets.MapW, MapH: assets.MapH,
				Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM, Palette: assets.PaletteROM[:],
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.Selected != RendererMode4 {
				t.Fatalf("dynamic names require Mode 4: %+v", report)
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
