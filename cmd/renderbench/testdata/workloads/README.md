# Renderbench workloads

These Lua files replace `examples/testio/game.lua` through `renderbench`'s
`-source` flag. They deliberately reuse the same map, sheet, palette, toolchain,
input trace, warm-up, and capture count so a paired Mode 4/Mode 0 comparison
changes only the renderer.

Example:

```sh
go run ./cmd/renderbench \
  -project examples/testio \
  -source cmd/renderbench/testdata/workloads/mset16.lua \
  -player build/player/mtile-player \
  -out /tmp/mtile-mset16
```

`fset1.lua` is a characterization workload: it is exact and keeps ample frame
headroom, but it intentionally fails the conservative 20% median-speed AUTO
gate. That measured crossover is why `tile-v1` keeps runtime `fset` projects on
Mode 4 while still permitting a forced Mode 0 build.
