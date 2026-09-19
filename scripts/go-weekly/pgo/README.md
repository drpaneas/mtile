# Reproduce or refresh cmd/mtile's PGO profile

The retained profile is scoped to `cmd/mtile/default.pgo`. It trains only the Go
build-preparation prefix: source bundling, globals analysis, validated asset
loading/flattening, transpilation and linked-asset validation. It measures neither
TinyGo nor native game performance, editor rendering, or whole CLI build latency.

Use a fresh snapshot/evidence directory. This recipe needs the original local
`examples/celeste` project, which is ignored by Git. Its full source/assets copy
for the accepted run is in `/tmp/mtile-go-weekly-loop/pgo/inputs/celeste`; the
repository retains only its SHA manifest. A checkout without this local fixture
cannot reproduce the same training corpus. Restore the exact fixture, or treat a
different corpus as a new experiment with freshly declared gates.

From the repository root, with the recorded Go toolchain:

```sh
python3 scripts/go-weekly/pgo/freeze.py /tmp/mtile-pgo-refresh docs/benchmarks/go-weekly/pgo-refresh
python3 scripts/go-weekly/pgo/experiment.py /tmp/mtile-pgo-refresh docs/benchmarks/go-weekly/pgo-refresh
python3 scripts/go-weekly/pgo/actual_build.py /tmp/mtile-pgo-refresh docs/benchmarks/go-weekly/pgo-refresh
```

The frozen training projects are Celeste AUTO, Multi-file and Audio GB Song.
Held-out projects are Platformer, Pong and Mapdemo; controls are Hello and
Noalloc Call Stress. Both implementations must improve the held-out aggregate
median by at least 3%, with neither control regressing more than 2%. `experiment.py`
records ten alternating pairs; the actual `Service.Build` validation
records six additional pairs using the same profile. There is exactly one
30-second training profile. Do not tune the profile or choose new inputs after
seeing held-out results. Training and measurement inherit the same normal
`GOMAXPROCS`; the original run used 12. Warm filesystem/compiler caches and
normal Go GC are part of this host experiment.

`internal/build/pgo_benchmark_test.go` is opt-in. It mirrors the default build
prefix and validates generated source and build metadata against the real
method before TinyGo. Keep the helper synchronized if Build changes. The
additional actual-method fixture is stored as `.go.in` so ordinary test
discovery does not run it. Both scripts compile `-pgo=off` and explicit-profile
test executables from the same frozen source. They preserve binaries, logs,
output manifests, samples and hashes and refuse to overwrite prior runs.

After reviewing passing gates and maintenance cost, copy the one profile to
`cmd/mtile/default.pgo` and verify the actual command:

```sh
cp /tmp/mtile-pgo-refresh/training.pprof cmd/mtile/default.pgo
python3 scripts/go-weekly/pgo/validate_cli.py /tmp/mtile-pgo-refresh docs/benchmarks/go-weekly/pgo-refresh
go test ./internal/build ./cmd/mtile
```

The CLI check builds one executable with `-pgo=off` and one with the default
profile, then requires identical generated Go and ROM hashes for Hello, Pong
and Mapdemo. It requires the normal TinyGo/native toolchain but makes no timing
claim for that external compilation. A rejected experiment must leave the
previous default profile intact. Do not copy this profile into the editor.

To build the command without PGO:

```sh
go build -pgo=off -o build/mtile ./cmd/mtile
```
