# Cartridge saving: parity and timing

> Historical evidence for the previous feature version. Current behavior and
> reproducible workloads are documented in [pause-only v2](pause-only-v2.md)
> and [save slots v2](saving-slots-v2.md). The original source/workload must be
> checked out to reproduce the old report below.

The four save builtins are renderer-independent. The fixture
`cmd/renderbench/testdata/workloads/saving.lua`, using the Save Game example's
assets, exercises all 64 dynamic field indices, signed/fractional payloads,
invalid arguments, parenthesized literal keys, commit/readback, staging
replacement, reload, and wrong-key failure without losing staged data.
It deliberately runs a full write/read/check sequence every eighth tick;
normal games should save only on explicit player actions.

The exact same ARM workload ran in mGBA with both linked renderers: eight
warmup frames, then 120 synchronized captured frames. All pixels matched.
A separate diagnostic checked the fixture's `errors` watch on every sample
in both renderers: all field/status checks passed. Both checks produced the
same paired ROM hashes.

| Measurement | Mode 0 | Mode 4 |
| --- | ---: | ---: |
| Median tick cycles | 8,043 | 31,831 |
| P95 tick cycles, including save ticks | 442,104 | 465,898 |
| Missed frames | 0 | 0 |
| ROM bytes | 34,540 | 35,080 |

The 30 Hz tick budget is 561,792 cycles. The shared menu loop is included.
A 64-byte nibble CRC table preserves the exact IEEE checksum and record
format while keeping the stress fixture within that budget. Independent
standard-library CRC vectors and recovery tests check compatibility.
The fixture establishes exact functionality and records
cost, not a guarantee that saving fits alongside arbitrary game logic.
Calls are synchronous and may briefly pause a busy game. Saving does not
alter the rest of a scene's AUTO admission rules.

Raw reports: [paired renderer data](saving-v1.json) and
[independent save-field oracle](saving-oracle-v1.json). The complete example
also passed an [ARM playthrough](saving-example-v1.json) in Mode 4, Mode 0,
and AUTO: pausing the game, saving once while A is held, clearing the wait
after five seconds, selecting Load, resuming, exiting, and restoring after a
machine reset, with identical captured pixels at every checkpoint. A
machine reset retains emulated SRAM and is not a physical power-cycle test.
The paired reports' environment
metadata records the compiler, source fingerprint, helper hash, and ROM
hashes. **This is emulator evidence; physical GBA/EZ-Flash verification is
pending.** Use the [dedicated hardware procedure](../../saving.md#ez-flash-omega-procedure).

From the repository root, with TinyGo and the native player available:

```sh
go run ./cmd/renderbench -project examples/save-game \
  -source cmd/renderbench/testdata/workloads/saving.lua \
  -out /tmp/mtile-saving-bench -warmup 8 -samples 120 -gate parity
MTILE_SAVE_OUT=/tmp/mtile-save-oracle \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestSave.*TargetDiagnostic$' -count=1 -v
```

Ordinary tests require both recorded reports, passing parity, zero oracle
errors, matching ROM hashes, and P95 costs within the tick budget. Runtime
tests additionally exercise every byte-store interruption and each byte's
corruption independently; compiler/build tests verify generated execution,
AUTO selection, SRAM identifier alignment, and IWRAM reader placement.
