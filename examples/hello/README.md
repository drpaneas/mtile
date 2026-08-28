# hello

The smallest native GBA example. It uses `_init`, `_update`, `_draw`, `btn`,
`cls`, `spr`, and `print`. In mGBA, click the window and use the arrow keys
(D-pad) to move the sprite one pixel per frame on the 240x160 screen.

From the repository root:

```sh
go run ./cmd/mtile build examples/hello/
```

The resulting `examples/hello/game.gba` is a native Mode 4 ROM suitable for
mGBA.
