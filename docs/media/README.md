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
