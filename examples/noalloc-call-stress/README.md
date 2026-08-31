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
| Median total cycles | 334,028 | 234,519 | -29.79% |
| p95 total cycles | 334,090 | 234,581 | -29.79% |
| p99 total cycles | 334,106 | 234,597 | -29.78% |
| Maximum total cycles | 585,743 | 486,225 | -16.99% |
| Median draw cycles | 144,856 | 143,722 | -0.78% |
| Median Mode 4 blit cycles | 76,953 | 76,924 | unchanged |
| Median update/overhead residual | 112,219 | 13,873 | -87.64% |
| Missed ticks (warm-up + capture) | 1 | 0 | cold miss removed |
| EWRAM used | 187,316 B | 187,316 B | unchanged |
| Debug ROM size | 37,428 B | 36,064 B | -3.64% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The visible HUD includes the tick counter and fixed-point state checksum, so
pixel parity also covers accumulated computation state. The workload is
intentionally dominated by tiny helpers and should not be read as a universal
29.79% speedup for ordinary games.

Re-run the paired proof after building the headless player:

```sh
go test -tags noallocdiagnostic ./cmd/renderbench \
  -run TestNoAllocCallDiagnostic -count=1 -v
```
