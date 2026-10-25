# Example projects

Start with [Coin Quest](../docs/first-game.md): create a playable game with
original art, sound, and four complete lesson checkpoints:

```sh
go run ./cmd/mtile new -template coin-quest mygame
go run ./cmd/mtile edit mygame
```

For the projects below, run these commands from the repository root, replacing
`hello` with the chosen name:

```sh
go run ./cmd/mtile edit examples/hello
go run ./cmd/mtile build examples/hello
```

In Develop, select Build + Reload and click the preview to give it keyboard
focus. Arrow keys are the D-pad, Z is A, and X is B. See
[getting started](../docs/getting-started.md) for dependencies.

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
| [hello](hello/README.md) | Input, sprite coordinates, and a background | Move with the arrows; change the speed |
| [multi-file](multi-file/README.md) | Split a game into explicit `require` modules | Edit player behavior separately from the HUD |
| [nerdo](nerdo/README.md) | Poses, animation clips, facing, and color variants | Hold left/right, then release; create a recolor in Sprite Lab |
| [background-layers](background-layers/README.md) | Four map planes and independent parallax cameras | Hide individual planes in Map Lab |
| [coin-block](coin-block/README.md) | Change one map cell when game state changes | Press A to hit; B to reset |
| [tile-links](tile-links/README.md) | Paint flowers and lamps in Map Lab; linked tiles draw their poses and clips | Scroll; press A to pick a flower; unlink the flower tile in Sprite Lab |
| [audio-gb-melody](audio-gb-melody/README.md) | Start/stop a looping four-channel melody | A plays; B stops; edit one channel in Music Lab |
| [audio-gb-song](audio-gb-song/README.md) | Arrange an intro followed by a looping section | Change a part and hear the transition |

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
