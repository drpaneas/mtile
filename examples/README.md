# Example projects

Open the [Mtile desktop download](https://github.com/drpaneas/mtile/releases/latest)
to play [Coin Quest](../docs/first-game.md). The app includes the game, its
editable art and sound, and four Lua lessons.

## Run an example

For the other games below, open the [release page](https://github.com/drpaneas/mtile/releases/latest)
and download **Source code (zip)** under **Assets** for the same version as your
Mtile app. Extract it and open a terminal in the folder that contains
`examples/`. These projects run with the [CLI included in the desktop download](../docs/getting-started.md#use-mtile-in-a-terminal).

To open Hello in the editor or export its ROM, run the corresponding command:

```sh
mtile edit examples/hello
mtile build examples/hello
```

In Develop, select **Build + Reload** and click the preview. The arrow keys
are the D-pad, Z is A, and X is B. The build command writes
`examples/hello/game.gba`. Replace `hello` with another project below.

## Four focused little games

Pick the feature you want; these are independent projects, not a required
sequence. Each includes original ready artwork, complete runnable lessons,
and one visible checkpoint at a time. See [choose one small game](../docs/small-games.md).

| Project | What to learn | What to try |
| --- | --- | --- |
| [Tidewalk](tidewalk/README.md) | One map → bounded camera → two-plane parallax | Walk along the pier; change the distant shore's camera speed |
| [Moth Study](moth-study/README.md) | One pose → looping clip → input and facing | Draw a still moth, then let a saved clip move its wings |
| [Moon Door](moon-door/README.md) | Start, restart, and finish one animation | Tap A to open a brass gate; add pause only in the optional lesson |
| [Lantern Rooms](lantern-rooms/README.md) | Select a room → cross an edge → choose a spawn | Visit the hanging garden and walk home |

## Other small examples

| Project | What to learn | What to try |
| --- | --- | --- |
| [font-demo](font-demo/README.md) | Select/draw one fixed-size font, preview text, and print Greek/German | Open FONT, copy a preset, paint a character and rebuild |
| [hello](hello/README.md) | Input, sprite coordinates, and a background | Move with the arrows; change the speed |
| [save-game](save-game/README.md) | Three named cartridge files, independent slots, Save/Load and failure handling | Select a file, name your hero, then use Select for the game menu |
| [multi-file](multi-file/README.md) | Split a game into explicit `require` modules | Edit player behavior separately from the HUD |
| [nerdo](nerdo/README.md) | Poses, animation clips, facing, and color variants | Hold left/right, then release; create a recolor in Sprite Lab |
| [background-layers](background-layers/README.md) | Four map planes and independent parallax cameras | Hide individual planes in Map Lab |
| [coin-block](coin-block/README.md) | Change one map cell when game state changes | Press A to hit; B to reset |
| [tile-links](tile-links/README.md) | Paint flowers and lamps in Map Lab; linked tiles draw their poses and clips | Scroll; press A to pick a flower; unlink the flower tile in Sprite Lab |
| [audio-gb-melody](audio-gb-melody/README.md) | Start/stop a looping four-channel melody | A plays; B stops; edit one channel in Music Lab |
| [audio-gb-song](audio-gb-song/README.md) | Arrange an intro followed by a looping section | Change a part and hear the transition |
| [audio-only](audio-only/README.md) | Hear four imported tracker voices with no gameplay | Read the matched Butano example and measured CPU/RAM explanation |

## Go further

| Project | What it demonstrates |
| --- | --- |
| [lumen](lumen/README.md) | A 12-room Mode 0 flight game with poses, named clips, four-layer parallax, original music, and a dawn ending |
| [pong](pong/README.md) | A complete game with opponent, collision, scoring, and sound |
| [mapdemo](mapdemo/README.md) | Camera, collision, and ROM streaming across a large world |
| [shared-tile-animation](shared-tile-animation/README.md) | Animate every occurrence of a shared background tile together |

Engineering fixtures live in [testdata/projects](../testdata/projects/README.md);
ports and private renderer experiments live in [research](../research/README.md).
Their workload constraints differ from teaching examples. Local untracked
projects are not part of this catalog.
