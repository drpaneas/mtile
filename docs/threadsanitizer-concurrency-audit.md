# ThreadSanitizer and concurrency audit — 2026-09-19

Decision: **keep the preview ownership fix, regression tests, and focused host
race-detector CI check**. This exercise improves editor reliability. It does not
change the cartridge runtime or claim a rendering speedup.

[The article](https://theconsensus.dev/p/2026/09/06/data-races-and-the-limits-of-threadsanitizer-in-c-and-go.html)
explains how synchronization mistakes can survive tests, how a program can
have a logical race without a data race, and how bounded tracking and artificial
synchronization can hide data races from ThreadSanitizer. Its useful application
here is to combine instrumented execution with explicit ownership reasoning and
tests of the required behavior. A clean detector run is evidence, not proof.

The [Go race-detector documentation](https://go.dev/doc/articles/race_detector)
likewise limits detection to races on executed paths. The
[Go memory model](https://go.dev/ref/mem) defines synchronization guarantees;
those guarantees do not establish which preview owns an input event.

## Reproduced bug and retained fix

The editor closes a stop channel when retiring an emulator worker. Closing it
does not join that worker: its ticker can also be ready, allowing another call
to `stepEmulator`. Previously that call could read and clear the replacement
preview's reset flag and pending button latch, and overwrite its last-sent keys.
Every access used `runMu`, so this was a lifecycle error, not a data race that
ThreadSanitizer incorrectly failed to report.

The new regression sets up a stopped or replaced preview and invokes the
retired worker's next tick. Against the original implementation, both cases
fail deterministically: the old worker consumes the reset/input and publishes
one audio frame and one video frame. The same assertions also fail under
`-race`, without a `WARNING: DATA RACE` report.

The fix checks `playRunID` while holding `runMu`, then snapshots, consumes and
acknowledges input in that same critical section. It deliberately checks the
active preview ID rather than the latest build ID: an existing preview must
continue running while its replacement is being built. A positive regression
covers that behavior. A step already in flight retains the retired worker's
own audio stream and frame channel; it does not consume replacement input.

Production cost: one identity comparison, no new fields, allocations, channels
or locks. The active path uses one lock/unlock pair instead of two. This is
not a measured performance claim; correctness is the reason to retain it.

## Coverage and tradeoffs

| Change or approach | Decision | Benefit and cost |
| --- | --- | --- |
| Preview identity check and atomic input handoff | Keep | Prevents a reproduced lost-input/reset bug with a small local change. |
| Deterministic stopped/replaced/build-in-progress tests | Keep | Check lifecycle behavior independently of scheduling luck or detector output. |
| Concurrent PCM contract test | Keep | Checks intact, increasing sample frames through ring wrap, allowed drops, underruns and final drain. Uses one producer, one reader and a bounded workload. |
| Focused Linux `-race` CI step at `-cpu=1,4` | Keep | Exercises editor, emulator bridge, live performance and MIDI packages. Adds instrumented build/test cost only. |
| Blanket locking or replacement of the live-audio ring | Drop | No demonstrated defect justified changing its existing ownership and atomic publication. |
| Production race instrumentation or a C/Go detector rewrite | Drop | Adds cost without addressing the reproduced lifecycle error. |

The focused four-package command passed locally in **5.16 seconds** with a
warm build cache on macOS/arm64. This is not an estimate of cold CI time. CI runs
the step only on Linux, using its existing graphics dependencies and Xvfb.
The deterministic regressions also run in ordinary macOS/Linux tests.

Manual review covered the mutex-protected emulator session/log/PCM bridge,
the producer-serialized live-audio queue and reader-owned oscillator state,
editor build/preview handoffs, MIDI callbacks, and the native helper boundary.
No application-level `sync.Pool` use or custom race-detector annotations were
found. Dependency and standard-library pools were not rewritten. Large-goroutine
stress alone and increasing detector history were not used as correctness proof.

The helper calls mGBA's frame execution and audio capture synchronously; the
current frontend does not start a threaded mGBA core. Go's `-race` check does
not instrument that separate C process or validate native MIDI libraries.
GBA interrupt preemption also needs target-specific reasoning and tests.
Queue and snapshot critical sections were reviewed, but this exercise is not
a target interrupt-safety sign-off. A possible lost rejection-counter increment
was noted for invalid direct Go controller calls concurrent with IRQ service;
the generated Lua wrappers reject those inputs before those methods. That
telemetry-only case was not reproduced on target and no runtime change is claimed.

## Evidence and reproduction

[Evidence directory](benchmarks/threadsanitizer/) contains the expected baseline
failures, passing candidate tests, required repository checks, source/toolchain
fingerprints and the exact production patch. Existing workspace changes were
preserved. No public Lua API or renderer eligibility changed.

Passing validation:

```sh
go test ./cmd/editor -count=1
go test -race ./cmd/editor -count=20 -cpu=1,4 \
  -run '^(TestRetiredEmulatorCannotConsumeActivePreviewInput|TestActivePreviewKeepsRunningWhileReplacementBuilds)$'
go test ./internal/emulator -count=20 \
  -run '^TestPCMStreamConcurrentFramesStayWholeAndOrdered$'
go test -race ./internal/emulator -count=20 \
  -run '^TestPCMStreamConcurrentFramesStayWholeAndOrdered$'
go test -race -count=1 -cpu=1,4 \
  ./cmd/editor ./internal/emulator ./internal/performance ./internal/midicontrol
go test ./internal/transpiler ./internal/build ./cmd/mtile
(cd runtime && go test ./...)
```

Editor tests require an active graphical macOS session or `xvfb-run -a` on
headless Linux. To reproduce the baseline failure without changing the working
file, copy `cmd/editor/play_editor.go` to a temporary location, reverse
`benchmarks/threadsanitizer/preview-generation.patch` on the copy, and use Go's
`-overlay` option to map the original absolute filename to that baseline copy.
Run the retired-worker regression with and without `-race`; both should fail
their behavioral assertions. The recorded race-enabled baseline used this
overlay method.
