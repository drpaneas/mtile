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

With the normal no-allocation direct-call path, numeric tuple results remain a
material optimization in the current generated call graph.

| Metric | Tagged results | Numeric results | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 2,206,075 | 1,169,327 | -47.00% |
| p95 total cycles | 2,207,237 | 1,170,496 | -46.97% |
| p99 total cycles | 2,207,812 | 1,171,071 | -46.96% |
| Maximum total cycles | 2,541,097 | 1,504,356 | -40.80% |
| Median draw cycles | 201,502 | 195,584 | -2.94% |
| Median Mode 4 blit cycles | 76,924 | 76,924 | unchanged |
| Median update/overhead residual | 1,927,649 | 896,819 | -53.48% |
| Missed ticks (warm-up + capture) | 620 | 620 | intentionally over budget |
| EWRAM used | 183,456 B | 183,456 B | unchanged |
| Debug ROM size | 50,688 B | 50,532 B | -0.31% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The second pair disables the no-allocation direct-call optimization in both
builds. This keeps an identical root-protected, out-of-line call path and
exposes only the result ABI cost.

| Metric | Tagged results | Numeric results | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 2,438,893 | 1,363,142 | -44.11% |
| p95 total cycles | 2,440,046 | 1,364,295 | -44.09% |
| p99 total cycles | 2,440,619 | 1,364,868 | -44.08% |
| Maximum total cycles | 2,773,904 | 1,698,162 | -38.78% |
| Median draw cycles | 198,311 | 203,559 | +2.65% |
| Median Mode 4 blit cycles | 76,948 | 76,961 | effectively unchanged |
| Median update/overhead residual | 2,163,634 | 1,082,622 | -49.96% |
| Missed ticks (warm-up + capture) | 620 | 620 | intentionally over budget |
| EWRAM used | 183,456 B | 183,456 B | unchanged |
| Debug ROM size | 57,148 B | 56,716 B | -0.76% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The root-protected result is relevant to functions that allocate or cannot be
proven allocation-free. Both pairs demonstrate that specializing each tuple
slot matters independently of root-wrapper removal.

Re-run the paired proof after building the headless player:

```sh
go test -tags multireturnphysicsdiagnostic ./cmd/renderbench \
  -run TestMultiReturnPhysicsDiagnostic -count=1 -v
```
