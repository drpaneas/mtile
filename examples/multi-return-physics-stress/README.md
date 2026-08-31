# Multi-return physics stress benchmark

A deterministic Mode 4 workload with twelve bodies, four physics substeps,
and 240 numeric multi-return helper calls per 30 Hz tick. It isolates tuple
result specialization: parameters, numeric storage, and stable record fields
remain enabled in every build.

The baseline sets `DisableMonomorphicMultiReturns`, so each result crosses the
generated Go ABI as `rt.Value`. The optimized build proves each return position
independently and uses `rt.Number` for numeric slots. The HUD includes the tick
and fixed-point state checksum; exact frame comparison therefore covers both
rendering and accumulated simulation state.

## Paired Mode 4 results

With the normal no-allocation direct-call path, TinyGo inlined the helpers and
erased the tuple representation difference completely.

| Metric | Tagged results | Numeric results | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 642,296 | 642,296 | unchanged |
| p95 total cycles | 643,458 | 643,458 | unchanged |
| p99 total cycles | 644,033 | 644,033 | unchanged |
| Maximum total cycles | 977,318 | 977,318 | unchanged |
| Median update/overhead residual | 371,498 | 371,498 | unchanged |
| Debug ROM size | 43,996 B | 43,996 B | unchanged |
| Pixel differences across 600 frames | 0 | 0 | exact |

The second pair disables the no-allocation direct-call optimization in both
builds. This keeps an identical root-protected, out-of-line call path and
exposes only the result ABI cost.

| Metric | Tagged results | Numeric results | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 813,269 | 792,039 | -2.61% |
| p95 total cycles | 814,422 | 793,192 | -2.61% |
| p99 total cycles | 814,995 | 793,765 | -2.60% |
| Maximum total cycles | 1,148,280 | 1,127,050 | -1.85% |
| Median draw cycles | 196,712 | 194,690 | -1.03% |
| Median Mode 4 blit cycles | 76,932 | 76,924 | effectively unchanged |
| Median update/overhead residual | 539,625 | 520,425 | -3.56% |
| Missed ticks (warm-up + capture) | 620 | 620 | intentionally over budget |
| EWRAM used | 187,436 B | 187,436 B | unchanged |
| Debug ROM size | 50,012 B | 49,456 B | -1.11% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The out-of-line result is relevant to functions that allocate, cannot be
proven allocation-free, or are too large to inline. It is not an additional
2.61% gain when TinyGo already folds the complete call into its caller.

Re-run the paired proof after building the headless player:

```sh
go test -tags multireturnphysicsdiagnostic ./cmd/renderbench \
  -run TestMultiReturnPhysicsDiagnostic -count=1 -v
```
