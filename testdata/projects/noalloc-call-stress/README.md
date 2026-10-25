# Non-allocating call stress benchmark

A deterministic Mode 4 workload dedicated to measuring GC-root wrapper cost.
One live table parameter remains in scope while two pure numeric helpers run
384 times per 30 Hz tick. Numeric storage and function-signature specialization
remain enabled in both builds, isolating root-wrapper removal.

## Paired Mode 4 result

The benchmark builds two debug ROMs from this exact project. The baseline sets
`DisableNoAllocFunctionCalls`; the optimized build directly calls only
transitively proven non-allocating functions. Both run in the pinned headless
mGBA player for 20 warm-up ticks followed by 600 captured ticks.

| Metric | Root wrappers | Direct proven calls | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 330,184 | 239,432 | -27.49% |
| p95 total cycles | 330,246 | 239,494 | -27.48% |
| p99 total cycles | 330,262 | 239,510 | -27.48% |
| Maximum total cycles | 581,961 | 491,209 | -15.59% |
| Median draw cycles | 146,620 | 146,734 | +0.08% |
| Median Mode 4 blit cycles | 76,932 | 76,924 | unchanged |
| Median update/overhead residual | 106,632 | 15,774 | -85.21% |
| Missed ticks (warm-up + capture) | 1 | 0 | cold miss removed |
| EWRAM used | 183,364 B | 183,364 B | unchanged |
| Debug ROM size | 41,528 B | 40,700 B | -1.99% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The visible HUD includes the tick counter and fixed-point state checksum, so
pixel parity also covers accumulated computation state. The workload is
intentionally dominated by tiny helpers and should not be read as a universal
27.49% speedup for ordinary games.

Re-run the paired proof after building the headless player:

```sh
go test -tags noallocdiagnostic ./cmd/renderbench \
  -run TestNoAllocCallDiagnostic -count=1 -v
```
