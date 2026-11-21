# Pong

A native Mtile/GBA port of the
[gosprite64 Pong example](https://github.com/drpaneas/gosprite64/tree/main/examples/pong).

The court is fitted to the GBA's 240x160 screen. Each 30 Hz Mtile update runs
two gameplay simulation ticks to preserve gosprite64's 60 Hz motion and
collision behavior. The six source sound cues are recreated with editable Game
Boy PSG instruments, while a quiet CH3/CH4 court loop leaves CH1/CH2 free for
paddle, wall, goal, and round-start effects.

## Controls

- D-pad Up/Down: move the blue paddle
- The red paddle is computer-controlled

## Build

From the folder containing the [downloaded examples](../README.md#run-an-example), run:

```sh
mtile build examples/pong
```

## Mode 4 table-field benchmark

The game intentionally keeps the original record-style `player`, `computer`,
and `ball` tables. A paired debug-ROM run used the same source and audio assets,
the pinned headless mGBA player, 20 warm-up ticks, and 600 captured 30 Hz ticks.
Only the transpiler's static-field lowering changed.

| Metric | Legacy dynamic lookup | Stable-field lookup | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 525,024 | 268,213 | -48.91% |
| p95 total cycles | 557,674 | 282,054 | -49.42% |
| p99 total cycles | 597,329 | 324,474 | -45.68% |
| Maximum total cycles | 726,610 | 464,957 | -36.01% |
| Median draw cycles | 235,158 | 169,424 | -27.95% |
| Median Mode 4 blit cycles | 78,107 | 78,107 | unchanged |
| Missed ticks | 22 | 0 | -22 |
| EWRAM used | 188,408 B | 188,592 B | +184 B |
| Debug ROM size | 106,820 B | 109,036 B | +2,216 B |

A separate deterministic trace compared every `ball`, `player`, `computer`,
and score value after each of 400 updates; legacy and stable-field builds were
identical. The host microbenchmark for one string-key field read measured
28.4 ns/op dynamically versus 3.48 ns/op through the warm cache (8.16x), with
zero allocations in both cases.

A later runtime and compiler pass reduced the current game's median from
263,509 to 211,544 cycles (19.7%), with unchanged Lua source, identical pixels
over 600 measured ticks, and zero misses. It specializes axis-aligned drawing
and recognizes numeric locals in table initializers. The debug ROM also shrank
4,324 bytes; IWRAM grew 92 bytes and EWRAM usage stayed the same. See the
[Pong performance report](../../docs/pong-performance-2026-09-05.md) for the
paired measurements and reproduction commands.
