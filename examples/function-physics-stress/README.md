# Function physics stress benchmark

A deterministic Mode 4 physics workload with twelve record-shaped bodies, 66
pair checks, fixed-point integration/collision response, and roughly 280
numeric helper calls per 30 Hz tick. It isolates the user-function ABI while
leaving numeric globals, locals, arithmetic, and stable fields enabled in both
builds.

## Paired Mode 4 result

The benchmark builds two debug ROMs from this exact project. The baseline sets
`DisableMonomorphicFunctions`; the optimized build uses conservative numeric
parameters and single return values. Both run in the pinned headless mGBA
player for 20 warm-up ticks followed by 600 captured ticks.

| Metric | Tagged function ABI | Numeric signatures | Change |
| --- | ---: | ---: | ---: |
| Median total cycles | 875,310 | 649,440 | -25.80% |
| p95 total cycles | 917,070 | 667,623 | -27.20% |
| p99 total cycles | 936,885 | 679,583 | -27.46% |
| Maximum total cycles | 1,244,493 | 1,019,964 | -18.04% |
| Median draw cycles | 221,678 | 223,587 | +0.86% |
| Median Mode 4 blit cycles | 76,924 | 76,932 | unchanged |
| Median update/overhead residual | 576,708 | 348,921 | -39.50% |
| Missed ticks (warm-up + capture) | 620 | 620 | intentionally over budget |
| EWRAM used | 187,544 B | 187,544 B | unchanged |
| Debug ROM size | 56,864 B | 52,908 B | -6.96% |
| Pixel differences across 600 frames | 0 | 0 | exact |

The visible HUD contains the collision counter and fixed-point state checksum,
so pixel parity covers both rendering and accumulated physics state. The
workload deliberately exceeds the 561,792-cycle tick budget in both forms; it
amplifies function-call cost and is not a recommended shipping body/call count.

Re-run the paired proof after building the headless player:

```sh
go test -tags functionphysicsdiagnostic ./cmd/renderbench \
  -run TestFunctionPhysicsDiagnostic -count=1 -v
```
