# color

The `print` smoke project, identical to what `mtile new` generates. It
draws four lines of text with the implicit print cursor, one per call, using
the default draw color.

From the repository root:

```sh
go run ./cmd/mtile build examples/color/
```

The resulting `examples/color/game.gba` is a native Mode 4 ROM suitable for
mGBA.
