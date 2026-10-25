# color

The `print` smoke project, a dedicated implicit-cursor smoke fixture. It
draws four lines of text with the implicit print cursor, one per call, using
the default draw color.

From the repository root:

```sh
go run ./cmd/mtile build testdata/projects/color/
```

The resulting `testdata/projects/color/game.gba` is a native Mode 4 ROM suitable for
mGBA.
