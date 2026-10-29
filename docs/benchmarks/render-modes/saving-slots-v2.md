# Cartridge save slots: ARM parity and cost

The workload in `cmd/renderbench/testdata/workloads/saving.lua` stages all
64 exact numeric fields, saves and reloads them, and checks errors independently
of the image. It covers default slot-zero calls, literal slot 31, dynamic
slots 1/2, wrong keys, invalid slot types/ranges/fractions and `save_pending()`.
An accumulated ARM-side mask proves slots 0, 1, 2 and 31 actually ran.

Both renderers captured 120 synchronized samples with **zero differing pixels,
zero missed frames and zero save-oracle errors**. The independent oracle's
ROM hashes exactly match the paired benchmark.

| Measurement | Mode 0 | Mode 4 |
| --- | ---: | ---: |
| Median cycles | 8,349 | 34,360 |
| P95 cycles | 443,456 | 468,988 |
| Debug ROM bytes | 49,496 | 49,904 |
| Free EWRAM bytes | 247,520 | 253,156 |

The two-frame budget is 561,792 cycles. Slots share the same 864-byte staging
and scratch buffer; each owns two records in its own 1 KiB SRAM region.
Runtime tests additionally cover all 32 slots, raw fields, legacy offsets,
byte interruption, neighboring-slot isolation, retries and invalid inputs.

The [three-file playthrough](saving-example-v2.json) creates A, B and C,
saves distinct coin counts (0, 1 and 3), loads after unsaved movement, and
verifies each file after a machine reset. Holding A saves only once; Save
and Quit returns to the file picker after the reminder expires. Start selects
files, finishes names and returns from the game menu without pausing; only
gameplay opts into pause. Holding menu Start through that transition does not
open the pause overlay. The example's
dynamic strings require Mode 4; AUTO selects identical Mode 4 ROM bytes.
The numeric API workload above proves Mode 0 compatibility separately.

Evidence: [benchmark](saving-slots-v2.json), [ARM oracle](saving-slots-oracle-v2.json),
[example](saving-example-v2.json), and [system pause](pause-only-v2.md).
These are emulator checks. Real GBA/EZ-Flash power-cycle verification is pending.

```sh
go run ./cmd/renderbench -project examples/save-game \
  -source cmd/renderbench/testdata/workloads/saving.lua \
  -out /tmp/mtile-save-slots-bench -warmup 8 -samples 120 -gate parity
MTILE_SAVE_OUT=/tmp/mtile-save-slots \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run 'TestSave(Target|ExampleTarget)Diagnostic$' -count=1 -v
```
