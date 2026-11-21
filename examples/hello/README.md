# hello

The smallest native GBA example. It uses `_init`, `_update`, `_draw`, `btn`,
`cls`, `spr`, and `print`. In mGBA, click the window and use the arrow keys
(D-pad) to move the sprite one pixel per update on the 240x160 screen.

From the folder containing the [downloaded examples](../README.md#run-an-example), run:

```sh
mtile build examples/hello/
```

Build output reports the selected renderer. The ROM is `examples/hello/game.gba`;
use the editor preview or a GBA emulator to play it.
