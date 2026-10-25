# README preview

`background-layers.gif` and its PNG still are captured from the actual
`examples/background-layers` ROM running in the pinned headless mGBA helper.
No game logic, assets, or frame contents are substituted for the capture.
The preview is scaled 3× with nearest-neighbour sampling; GIF audio is omitted.
`capture.json` records the ROM hash and sampling settings.

To regenerate from the repository root, build the helper as described in
[the mGBA notice](../third-party/mgba.md), then use Python with Pillow installed:

```sh
go run ./cmd/mtile build -o build/readme-demo.gba examples/background-layers
python3 scripts/docs/capture_preview.py \
  --player build/player/mtile-player \
  --rom build/readme-demo.gba \
  --out docs/media
```

The artwork and capture come from Mtile's own example and are covered by the
repository's [MIT license](../../LICENSE).

## Coin Quest

`coin-quest/preview.png` shows the generated beginner game after a scripted
rightward movement collects the first coin. The image is a real mGBA frame,
scaled 3× with nearest-neighbour sampling. Its adjacent `capture.json` records
the ROM hash and exact inputs.

With the same helper and Python/Pillow setup, reproduce it from a new project:

```sh
go run ./cmd/mtile new -template coin-quest build/coin-quest-preview
go run ./cmd/mtile build build/coin-quest-preview
python3 scripts/docs/capture_coin_quest.py \
  --player build/player/mtile-player \
  --rom build/coin-quest-preview/game.gba \
  --out docs/media/coin-quest
```

Choose an unused project directory if regenerating; `new` refuses to overwrite
existing work. The template's original artwork is also MIT licensed.

## Four small teaching games

The four focused games have native `preview.png`, `play.gif`, and `capture.json`
files in their example directories: [Tidewalk](../../examples/tidewalk/README.md),
[Moth Study](../../examples/moth-study/README.md),
[Moon Door](../../examples/moon-door/README.md), and
[Lantern Rooms](../../examples/lantern-rooms/README.md).

Reproduce and verify them with the same headless mGBA helper:

```sh
go run ./scripts/docs/verify_learning_games.go \
  -player build/player/mtile-player \
  -out /tmp/mtile-learning-proof -publish
```

This builds every complete lesson in both forced renderers, plays its controls,
and compares native pixels and gameplay watches at matching 30 Hz game frames.
The checks cover map edges, clip playback and facing, action restart/hold/pause,
and both room crossings with their entry positions. Mode 0 must also meet its
poststartup frame deadlines. Debug telemetry is read only.

`-publish` refreshes the shipped game's three capture files after verification.
The PNG is 3× and the GIF 2× nearest-neighbour; GIFs use the captured native
colors and omit audio. The JSON records ROM hashes, selected lesson, preview
frame, inputs, parity totals, and successful checkpoints. To check one project,
add `-project moon-door`; add `-lesson 02-pause` to select one lesson.

The asset generators keep their authored proofs separate from these native
captures. Run the verifier again after changing assets or game code.

`go run ./scripts/docs/verify_learning_games.go -gallery` combines the four
published PNGs into `learning-games.png`, without altering their pixels.
Its order is Tidewalk, Moth Study, Moon Door, and Lantern Rooms.

## Editor screenshots

`editor/*.png` are actual mTile `Game.Draw` renders, captured by the native
Ebitengine harness at 1600×1000 with default COMFORT text and CLEAN effects.
They are not AI-generated images or reconstructed mockups. No image scaling,
annotation, or compositing is applied to the distributed PNGs. Window-manager
chrome and the OS cursor are outside the render target.

| Image | Checked-in project / state |
| --- | --- |
| shared-tiles.png | background-layers, shared tile 1 selected |
| poses.png | nerdo, first registered pose selected |
| animations.png | nerdo, first saved clip selected, stopped |
| color-variants.png | nerdo, disposable `alternate` variant; first opaque source color replaced by index 12 |
| map-rooms.png | background-layers, Rooms enabled, first room selected, BG1 active |
| audio.png | audio-gb-melody, initial song view |
| develop.png | nerdo, source view before Build + Reload; no simulated game or telemetry |
| preferences.png | nerdo, default preferences open |

The harness copies projects into temporary directories and never saves the
tutorial variant back into the example. `editor/capture.json` records the
project mapping and capture parameters. Regenerate on a desktop session with
the editor's normal native graphics/audio dependencies:

```sh
MTILE_DOC_SCREENSHOTS="$PWD/docs/media/editor" \
  go test -tags mtile_editor_bench ./cmd/editor \
  -run '^TestDocumentationScreenshots$' -count=1 -timeout=90s
```

The test skips unless that environment variable is set. The source is
[`cmd/editor/docs_screenshots_test.go`](../../cmd/editor/docs_screenshots_test.go).
These captures document the editor using the projects listed above. Screenshot
capture does not grant additional rights to any underlying third-party artwork;
retain the applicable project and third-party notices when reusing it.
