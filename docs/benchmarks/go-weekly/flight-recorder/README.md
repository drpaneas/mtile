# Editor flight recorder, 2026-09-19

**Drop the product integration.** The candidate successfully captured a valid
execution trace containing events before a stalled Update, but Code rendering
exceeded the frozen 5% overhead gate: **+9.06% median paired overhead**. No tuning
or second performance attempt followed the failure. The production flag,
wrapper and candidate tests were removed; existing files were restored exactly.
The patch and fixtures remain available for future investigation.

## Candidate and correctness

The proposed `-trace-stall <file>` option started Go 1.27.1's flight recorder
before entering the editor loop. It targeted five seconds / 8 MiB of recent
history and captured once after an Update completed in at least 100 ms. With
the option absent, startup returned the original `*Game`; ordinary Update had
no added branch or clock read. Only Update was wrapped; permanent hangs and
Draw-only stalls did not trigger capture.

An asynchronous worker wrote directly to a newly created 0600 file with
`O_EXCL`, then stopped the recorder. Shutdown joined the writer before normal
editor cleanup and persistence. Startup rejected an existing destination, and
capture checked exclusivity again. Write/close failures were logged and the
partial file was removed when its identity still matched the created file;
an observed replacement was preserved. This identity check is not a universal
atomic unlink guarantee. No-stall shutdown created no file.

Focused tests passed for threshold boundaries, preserved Update errors,
exclusive creation after startup, partial-write and close failures, replacement
preservation, concurrent capture/shutdown, and absolute CLI path forwarding.
The writer/shutdown ordering test uses `testing/synctest` to prove shutdown
waits while the writer is blocked. Race tests passed at `-cpu=1,4`.

The [6,964-byte real trace](pre-stall.trace) decoded through EOF with the
standard library's `cmd/trace -d=parsed` decoder. Its ordered events are:

| Event | Trace time, ns |
| --- | ---: |
| Pre-stall sentinel in an earlier Update | 110831435967040 |
| Controlled stall region begins | 110831435968064 |
| Controlled stall region ends | 110831557991040 |
| Completed-Update capture trigger | 110831557994496 |

The region lasted **122.022976 ms**. This demonstrates retained pre-stall data,
rather than starting tracing after the slow call. See [trace proof](trace-proof.json),
[complete parsed events](parse-trace.log), and [validation commands](validation.json).
The controlled stall uses the real production wrapper and flight recorder with
a small test game that sleeps; it is not a recorded user incident.

## Frozen performance gate and result

The [candidate and gates](frozen-candidate.json) were frozen before timing.
The host used macOS/arm64, Go 1.27.1 and Ebitengine 2.10.2.
Each Sprite, Map and Code scene used the same copied `background-layers`
project, 1200×800 target and existing native Ebitengine render harness. Each
sample measured **Update + forced Draw + GPU ReadPixels** on the render thread;
dispatch, setup, hashes and recorder start/stop were excluded. Recorder-enabled
and disabled states each received at least one second of active warmup.
Samples accumulated at least 300 ms of measured work and at least 32 frames.

Six pairs per scene alternated off/on and on/off. The gate was at most +5%
median of the six paired percentage changes, calculated from each sample's
mean frame time. Any auto-triggered recorder would invalidate its sample.
No sample triggered, failed or changed pixels; every measured frame matched
the scene's reference RGBA hash.

| Scene | Measured frames | Median paired overhead | Observed pair range | Result |
| --- | ---: | ---: | ---: | --- |
| Sprites | 581 | −3.50% | −14.20% to +11.91% | Pass |
| Map | 384 | +1.90% | −2.83% to +6.89% | Pass |
| Code | 817 | **+9.06%** | −3.42% to +25.97% | **Fail; stopped** |

Code's six paired changes were +25.97%, +5.80%, +4.67%, +20.13%, +12.32% and
−3.42%. Four of six exceeded the gate. These are noisy host measurements, not
deterministic CPU costs or a speedup claim for Sprites. Descriptive 95%
percentile bootstrap intervals over the six pairs are −11.04%..+6.27% for
Sprites, −1.66%..+5.77% for Map, and +0.63%..+23.05% for Code. The broad intervals
limit precision; they do not change the predeclared rejection rule.

[Per-pair mean frame times](paired-means.csv), [all frame durations](performance.json),
[summary](summary.json), and [test output](performance.log) are preserved.
The complete measurement run took approximately 95 seconds. No subsequent
benchmark was used to rescue the failed candidate.

## Memory observations

Memory was measured separately after six seconds of active rendering in each
state, using post-GC `HeapAlloc`, `HeapInuse` and current process RSS. After
stopping the recorder, one further second of activity preceded another GC and
measurement. The gate was at most 32 MiB additional steady live `HeapAlloc`.

| Scene/state | HeapAlloc MiB | HeapInuse MiB | RSS MiB |
| --- | ---: | ---: | ---: |
| Sprites disabled | 267.81 | 270.93 | 771.88 |
| Sprites enabled | 271.72 | 275.31 | 771.61 |
| Sprites stopped | 264.18 | 267.40 | 768.09 |
| Map disabled | 1,059.26 | 1,062.74 | 1,860.12 |
| Map enabled | 1,062.13 | 1,065.81 | 1,993.69 |
| Map stopped | 1,055.59 | 1,058.84 | 1,993.08 |

The measured live-heap increases were **3.92 MiB** and **2.87 MiB**, both within
the gate. Code's memory phase was not run after its timing failure. RSS includes
graphics resources, caches and allocator retention in the shared process;
Map's RSS increase persisted after stopping tracing even as live heap fell.
These readings do not isolate every byte attributable to the recorder.

Go's `MaxBytes` and `MinAge` are retention targets: they do not guarantee a hard
memory/file-size cap or a full five seconds of history. Asynchronous capture
also forces a global trace-buffer flush, so it is not free of process-wide work.
The measured heap result must not be presented as a universal memory bound.

## Restoration and reproduction

[Restoration hashes](restoration.json) confirm that both existing entrypoints
match their [pre-experiment snapshot](pre7.json). The four new Go files were
removed from buildable source. Restored `go test ./cmd/editor ./cmd/mtile
-count=1` passed; see [restored checks](restored-tests.log). The earlier renderer
benchmark harness and accepted editor changes were preserved.

The complete rejected change is [candidate.patch](candidate.patch), with source
and tests also archived under [candidate](candidate/cmd/editor/flight_recorder.go.in)
as `.go.in` files. The two original entrypoints are under `baseline/`.
In an isolated checkout matching this experiment, apply that patch, then run:

```sh
go test -race ./cmd/editor ./cmd/mtile \
  -run 'TestFlightRecorder|TestRunEditForwardsAbsoluteStallTracePath' -cpu=1,4 -count=1

# Use a fresh absolute output directory; captures never overwrite files.
MTILE_FLIGHT_REPORT=/tmp/flight-recording go test ./cmd/editor \
  -run '^TestFlightRecorderRealPreStallCapture$' -count=1 -v
go build -o /tmp/trace-tool cmd/trace
/tmp/trace-tool -d=parsed /tmp/flight-recording/pre-stall.trace

MTILE_FLIGHT_BENCH=/tmp/flight-measurement go test ./cmd/editor \
  -tags=mtile_editor_bench -run '^TestFlightRecorderPerformance$' \
  -count=1 -timeout=10m -v
```

Create `/tmp/flight-recording` first. The benchmark creates its output directory.
The ordinary product does **not** expose `-trace-stall` after this rejection.
