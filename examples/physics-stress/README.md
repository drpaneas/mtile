# Physics stress benchmark

A deterministic Mode 4 compiler workload with sixteen record-shaped bodies,
fixed-point integration, gravity, drag, wall response, impulse response, overlap
correction, and all 120 body-pair checks on every 30 Hz tick.

The workload exists to compare Lua numeric lowering. Source, initial state,
renderer, warm-up, sample count, and assets must remain identical between the
two ROMs in a paired benchmark.

## Paired Mode 4 result

The benchmark builds two debug ROMs from this exact project. The tagged build
sets `DisableMonomorphicNumbers`; the optimized build uses the default numeric
lowering. Both retain stable table-field caching. The pinned headless mGBA
player runs 20 warm-up ticks followed by 600 captured ticks.

| Metric | Tagged `rt.Value` numbers | Monomorphic `rt.Number` | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 923,949 | 799,393 | -13.48% |
| p95 total cycles | 960,653 | 831,615 | -13.43% |
| p99 total cycles | 972,311 | 841,893 | -13.41% |
| Maximum total cycles | 1,319,715 | 1,194,763 | -9.47% |
| Median draw cycles | 243,532 | 240,504 | -1.24% |
| Median Mode 4 blit cycles | 76,932 | 76,924 | unchanged |
| Median update/overhead residual | 603,485 | 481,965 | -20.14% |
| Missed ticks (warm-up + capture) | 620 | 620 | intentionally over budget |
| EWRAM used | 187,592 B | 187,544 B | -48 B |
| Debug ROM size | 65,072 B | 47,812 B | -26.52% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The workload deliberately exceeds the 561,792-cycle 30 Hz budget in both
forms. Its purpose is to amplify arithmetic cost, not to represent a shippable
body count. The visible HUD includes the numeric state checksum, so the
zero-pixel assertion covers both body positions and accumulated physics state.
On the Apple M2 Pro host microbenchmark, a warm stable numeric field read was
3.07 ns/op versus 3.47 ns/op for the tagged `Value` read; both paths reported
zero allocations. Most of the ROM-level gain therefore comes from native
locals, globals, loop counters, and arithmetic rather than the field load alone.

Re-run the paired proof after building the headless player:

```sh
go test -tags physicsdiagnostic ./cmd/renderbench \
  -run TestPhysicsMode4Diagnostic -count=1 -v
```
