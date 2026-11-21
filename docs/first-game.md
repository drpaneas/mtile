# Your first game in mTile

[Getting started](getting-started.md) · [Documentation index](README.md)

In **Coin Quest** you move an explorer, collect five coins, hear each pickup,
and restart. The desktop download includes the game, its editable artwork
and a two-note hardware sound effect. Allow about 15 minutes to play it,
change its speed and try it again. The four lessons below are optional.

![Coin Quest after collecting the first coin, captured from its actual GBA ROM.](media/coin-quest/preview.png)

*An actual mGBA capture. [Reproduction details](media/README.md#coin-quest).*

## Check your setup

[Download Mtile from GitHub Releases](https://github.com/drpaneas/mtile/releases/latest).
Extract the whole archive, then double-click **Mtile.exe** on Windows,
**Mtile.app** on Mac, or **Mtile** on Linux. Coin Quest opens in **Develop**.
Its editable files live in `mTile Projects/Coin Quest` inside your home folder.
The download includes everything you need to play, change the Lua and export
a ROM offline. See the [desktop installation guide](getting-started.md#using-a-desktop-bundle)
for help choosing an archive.

## Play the complete game

Move with the arrow keys, collect five coins, then press **Z** (GBA A) to
restart. If the arrows move the text cursor, click the game preview. Press
Escape when you are ready to edit the code.

## Your first change

In **Develop**, open `game.lua`. Find `speed = 2` near the top and change it to
`speed = 3`. Save, select **Build + Reload**, and click the preview again.

**Checkpoint:** the explorer moves faster. You have completed the same
edit → build → play loop you will use for every new feature. Change it back
to `2` if you prefer the original pace.

**You can stop here.** To share your modified game, select **Export .gba**.
The editor reports where it saved the cartridge. Keep the project folder so
you can edit it later.

For another small change, recolor sprite **1**, the explorer, in **Sprites**.
Save and rebuild to see the new artwork. Open `README.md` inside your Coin Quest
project for the complete offline walkthrough.

## Learn one piece at a time

When you want to understand the code, start with `01-movement.lua`. Each lesson
adds one idea; use the next one after its checkpoint makes sense.

| Checkpoint | What you learn | What to check |
| --- | --- | --- |
| `lessons/01-movement.lua` | Input, sprite coordinates, screen boundaries | The explorer stops at each edge of the play area |
| `lessons/02-coins.lua` | Collision and score | Each pickup awards one coin |
| `lessons/03-sound.lua` | Playing a hardware sound clip | Each pickup produces a rising chime |
| `lessons/04-complete.lua` | Win condition and restart | Five coins win; A resets the game |

Open a lesson in **Develop** and click **Run Lesson** beside the source tabs.
The GBA preview shows which lesson is running. **Build + Reload** tries your
latest edits to the selected lesson. Choose **Back to My Game** to reopen the
main source file and restart your own game.

Lesson previews and **Back to My Game** build a temporary ROM using your
current Lua buffers and the project's saved assets. Save either source tab
when you want to keep its edits on disk.

## Choose one next step

Choose [one small game](small-games.md) for the feature you want next: a pose
and looping clip, an action that starts and finishes, a scrolling coast, or a
transition between two rooms. Each has ready artwork and separate runnable
lessons. You do not need to work through all four.

Pick the feature you want next and keep a working copy before experimenting.
