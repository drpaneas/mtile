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
| Median total cycles | 985,948 | 740,131 | -24.93% |
| p95 total cycles | 1,099,691 | 764,342 | -30.49% |
| p99 total cycles | 1,196,595 | 784,075 | -34.47% |
| Maximum total cycles | 1,415,553 | 1,119,955 | -20.88% |
| Median draw cycles | 229,881 | 230,350 | +0.20% |
| Median Mode 4 blit cycles | 76,961 | 76,961 | unchanged |
| Median update/overhead residual | 679,106 | 432,820 | -36.27% |
| Missed ticks (warm-up + capture) | 620 | 620 | intentionally over budget |
| EWRAM used | 183,960 B | 183,960 B | unchanged |
| Debug ROM size | 63,976 B | 60,828 B | -4.92% |
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
