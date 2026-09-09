# Your first game in mTile

[Getting started](getting-started.md) · [Documentation index](README.md)

**Coin Quest** is a small, complete game: move an explorer, collect five coins,
hear each pickup, and restart. It includes original editable sprites and a
two-note hardware sound effect. Plan for about 15 minutes of learning once
your tools are installed; first-time builds can take longer.

![Coin Quest after collecting the first coin, captured from its actual GBA ROM.](media/coin-quest/preview.png)

*An actual mGBA capture. [Reproduction details](media/README.md#coin-quest).*

## Check your setup

Follow the [installation requirements](getting-started.md#requirements), then
run these commands from your source checkout:

```sh
go run ./cmd/mtile doctor
go run ./cmd/mtile new -template coin-quest mygame
go run ./cmd/mtile build mygame
go run ./cmd/mtile edit mygame
```

Using an installed desktop bundle? Replace `go run ./cmd/mtile` with `mtile`.
Keep flags before the project directory. Creation refuses to overwrite an
existing directory; choose another name if `mygame` already exists.

`doctor` checks Go, the selected TinyGo version, GBA target resources, and the
mTile runtime. Failed checks include suggested fixes and return a nonzero exit
status. It does not install software or validate the desktop display, audio,
or an actual ROM build. Those are exercised by the following commands.

## Play, then learn

Choose **Develop** → **Build + Reload** and click the GBA preview. Move with
the arrow keys, collect five coins, then press **Z** (GBA A) to restart.
Press Escape to return keyboard focus to the editor.

Open the generated `mygame/README.md` for the complete walkthrough, also
available [here](../internal/build/templates/coin-quest/README.md).
It travels with the project and works offline.

| Checkpoint | What you learn | What to check |
| --- | --- | --- |
| `lessons/01-movement.lua` | Input, sprite coordinates, screen boundaries | The explorer stops at each edge of the play area |
| `lessons/02-coins.lua` | Collision and score | Each pickup awards one coin |
| `lessons/03-sound.lua` | Playing a hardware sound clip | Each pickup produces a rising chime |
| `lessons/04-complete.lua` | Win condition and restart | Five coins win; A resets the game |

Every checkpoint is a complete program. Copy its contents into `game.lua`,
save, and rebuild to try it. The finished version is installed as `game.lua`
from the start, so there is always something playable to return to.

## Your first changes

Change `speed = 2`, set a different `goal`, or redraw sprite 1 in **Sprites**.
Save and rebuild to see your changes. The generated walkthrough explains the
code and the sound editor step by step.

Share the resulting `mygame/game.gba` with a friend who has a GBA emulator.
Keep the project folder for future edits. Next, explore
[maps and backgrounds](graphics-model.md) or the [example projects](../examples/README.md).
