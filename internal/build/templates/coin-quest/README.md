# Your first game: Coin Quest

Collect five coins with your little explorer, hear a chime with every pickup,
and press A to play again. This project includes original artwork and sound.
You can edit both in mTile. No external game assets are needed.

**Aim for about 15 minutes after setup.** First-time compiler downloads/builds
can take longer. You do not need to know Go or GBA hardware registers.

**First visit? Do sections 1 and 2 only:** play the game, then change its speed
and see the result. That is a complete first session. The later lessons explain
one piece at a time when you are ready; you do not need to read them all now.

<span id="play-the-complete-game"></span>

## 1. Play the complete game

If you have not created the project yet, run:

```sh
mtile new -template coin-quest mygame
```

From a source checkout, use `go run ./cmd/mtile` instead of `mtile`.
If this README is already inside your generated project, skip creation.

If you use an installed bundle, run these commands from the folder containing
this project, replacing `mygame` with its directory name:

```sh
mtile doctor
mtile build mygame
mtile edit mygame
```

From a source checkout, use `go run ./cmd/mtile` in place of `mtile` and run
commands from the repository root. A project path containing spaces needs quotes.
`doctor` checks the ROM tools; it does not test the editor's graphics or audio.

Choose **Develop**, then **Build + Reload** (Ctrl/Cmd+R). Click the game preview
before using the keyboard. The arrow keys move; **Z is GBA A**, which restarts.
Escape returns focus to the code editor. Use **Export .gba** to write the release ROM.

**Checkpoint:** collect five coins. The counter stops at 5/5 and shows
“You win” Press Z to reset the counter and position.

<span id="your-first-change"></span>

## 2. Change one value

In **Develop**, open `game.lua`. Change `speed = 2` to `speed = 3` and build
again. Click the preview and move with the arrows.

**Checkpoint:** the explorer moves faster. You have completed the
edit → build → play loop. Change the speed back to `2` if you prefer.

**You can stop here.** Keep your working project and return when you want to
try another change or understand one part of the code.

<details markdown="1">
<summary>Optional: change the goal or the artwork</summary>

Try one experiment at a time, saving and rebuilding after each change:

- Change `goal = 5` to `goal = 8`. **Checkpoint:** the game now asks for eight
  coins before you win.
- Choose **Sprites** and edit sprite **1**, the explorer. Sprite **2** is the
  coin; color 0 is transparent. **Checkpoint:** your artwork appears in the game.

The `speed` value measures pixels per axis per update; this example uses
mTile's usual approximately 30 updates per second.

</details>

<span id="learn-one-piece-at-a-time"></span>

## 3. Learn movement from a working checkpoint

The `lessons/` directory contains four complete Lua programs. Open one in
**Develop** and click **Run Lesson** beside the source tabs. The GBA preview
shows the lesson name and runs your current Lua edits with the project's saved
assets. Use **Build + Reload** to try another edit in the selected lesson.

Choose **Back to My Game** to reopen `game.lua` and restart your own game
with your current edits. Your lesson and game edits stay in their own tabs.

Start with `lessons/01-movement.lua`.

- `_update()` changes the game state; `_draw()` displays it.
- `btn(0)` through `btn(3)` read left, right, up, and down.
- `spr(1, player_x, player_y)` draws sprite 1 at its top-left position.
- `mid(8, player_x, 224)` keeps the player between the two horizontal bounds.
  The vertical bounds leave space for the title and controls.

**Checkpoint:** hold each arrow key. The explorer stops before leaving the
play area. Change the speed and check that the boundaries still work.

## 4. Add coins and collision

Open `lessons/02-coins.lua` and click **Run Lesson**. `collect_coin()` checks
whether the two 8×8 sprite boxes overlap.

```lua
if abs(player_x - coin_x) < 8 and abs(player_y - coin_y) < 8 then
```

Both distances must be less than eight pixels. `abs()` makes the distance
positive whichever side you approach from. This is a box collision, so it
includes transparent pixels around the artwork.

Each pickup adds one to `score` and moves the coin to a predictable new
position. `%` is the remainder operator; here it keeps positions in the play
area. `print(score, 200, 8, 15)` displays the number beside the label.
Keeping labels and numbers in separate calls also lets the hardware renderer
draw this game efficiently.

**Checkpoint:** collect three coins and watch the score increase exactly once
per pickup. This lesson intentionally has no win condition yet.

## 5. Give pickups a sound

Open `lessons/03-sound.lua` and click **Run Lesson**. The new `sfx(0)` call
plays the project's **Collect a coin** effect. It contains two short notes
using the **Coin sparkle** sound on hardware channel CH1.

The sound is already included under `assets/audio/`. Open **Audio** to inspect
or edit it, then save and rebuild. `sfx(0)` selects clip 0; a clip sequences
notes, while a sound defines their tone. You do not need to edit the JSON.

**Checkpoint:** each pickup plays a short rising chime. If silent, check the
Preview Audio volume in Preferences (Ctrl/Cmd+`,`) and your computer's audio output.

## 6. Add a finish and restart

Open `lessons/04-complete.lua` and click **Run Lesson**.

- `score < goal` allows movement and pickups until you win.
- `restart()` resets both sprites and the score.
- `btnp(5)` detects a press of A. Holding a button also repeats after a delay;
  a quick tap is enough to restart.

**Checkpoint:** reach 5/5, verify that the coin disappears and the score stops,
then press Z. You can restart during play too.

## 7. Export and share

Choose **Export .gba** and save `game.gba`. Export builds the main game even
while a lesson is selected. Share the ROM with someone who has a GBA emulator,
or copy it to compatible GBA flash-cartridge hardware. Save the entire project
folder separately so you can continue editing it.

Try a small extension: change the explorer's colors, design a different chime,
or increase the goal. Keep a working copy before making a larger change.

## If something goes wrong

| Symptom | Next step |
| --- | --- |
| Build tools are missing | Run `mtile doctor` and follow its fixes. For source use, install Go before running the CLI. |
| The source preview cannot start | Initialize submodules with `git submodule update --init --recursive`; check CMake and your C compiler. |
| Keys do nothing | Click the GBA preview; Z is A. |
| The preview is still showing the main game | Open the lesson's tab and click **Run Lesson**. |
| A code edit fails | Select the reported diagnostic; restore a working lesson if needed. |
| You want to start over | Create a different directory with `mtile new -template coin-quest fresh-game`. Existing projects are never overwritten. |
