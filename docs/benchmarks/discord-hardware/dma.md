# DMA, alignment, BIOS copy, and constant materialization trials

Worktree: `/private/tmp/mtile-discord-experiments-ofgo5qwq/dma`.
Baseline: `/private/tmp/mtile-discord-experiments-ofgo5qwq/baseline`.
All target results use native mGBA's ARM7TDMI execution, not physical hardware.
No public Lua API was added or changed.

## Keep: bound DMA stalls

The original 9,600-word EWRAM-to-VRAM transfer costs 76,929 cycles in the fixed-size fixture. Timer0 overflows every 65,536 cycles. At starting phase 57,344, the independent Timer1 cascade records two overflows while the CPU's Timer0 handler receives one interrupt. This occurs with both an empty handler and a synthetic 16,384-cycle handler body.

Chunking the transfer at 4,096 words makes all 16 phase/body combinations deliver every overflow. The corrected fixed-size copy costs 77,025 cycles (+96, +0.125%), and all words match. Generic variable-count calls cost +26 cycles for small transfers and +138 cycles for 9,600 words. Both public hardware-copy helpers use the bounded path. Existing negative/zero/oversized guards remain unchanged.

A production-shaped follow-up ran `runtime/cmd/audiopsgcoexist` for 1,800 emulator frames per variant. Baseline: 7,533 service calls and 295 applied events. Candidate: 7,689 calls and 304 events. Both report PASS, no frame skips, no queue overflows, no rejected commands, all four PSG channels observed, and the same 9,590-cycle maximum IRQ. The earlier coexist self-test does not detect scheduler slowdown; the independent overflow-count fixture supplies that regression proof.

Decision: keep. Slight copy overhead and greater opportunity for audio IRQ work are outweighed by preserved audio-clock delivery. Mode 4 copies a hidden page before presentation, so IRQ opportunities during the copy do not themselves tear the visible page. Large visible Mode 0 commits still require a complete budget audit.

## Keep: unaligned Fill32 byte fallback

The original DMA32 fallback masks destination address bits. An eight-word fill at offsets 1, 2, and 3 produced 33, 34, and 35 byte mismatches, including bytes outside the requested slice. The aligned case passed.

The candidate preserves the existing aligned ARM fast path and uses byte stores for an unaligned RAM destination. All four alignments now pass. The unaligned eight-word case costs 182 rather than 153 cycles (+29), an appropriate cost for correct writes. Aligned results are 202 versus 198 cycles, a small code-layout difference rather than a claimed optimization. Rejected counts still leave guards untouched.

Decision: keep. Current framebuffer callers are aligned, so their intended fast path remains.

## Drop: extra DMA startup NOPs

Inserted two explicit ARM NOPs immediately after enabling DMA. Baseline and candidate copy guards pass across counts -1, 0, 65536, 9601, 1, 2, 7, 8, 31, 32, 4096, 4097, and 9600, for both byte- and word-slice APIs. Two NOPs add 24 cycles to a three-chunk full-frame copy: 77,025 to 77,049. No additional correctness benefit was observed. This emulator result does not certify all physical startup timing; retain current polling and guards, do not add speculative overhead.

## Drop: CpuFastSet replacement

The compatible aligned 9,600-word case reproduces exact destination bytes but costs 85,405 modeled cycles versus 76,929 for the original DMA and 77,025 for bounded DMA. It also has a stricter alignment/block contract. mGBA's HLE BIOS is used; this is not a physical BIOS timing measurement. Bounded DMA handles the tested IRQ delivery issue with much less cost.

## Keep existing LLVM output: constant materialization

A separate ARM microtrial loads `0x040000de` through a literal pool or `mov #0xde; orr #0x04000000`. Both complete routines occupy 12 bytes including return/literal data. With the runtime's WAITCNT `0x4317`, 16,000 executions per variant measure:

| Location | Literal load | Two immediates |
| --- | ---: | ---: |
| ROM | 19 cycles | 14 cycles |
| IWRAM | 6 cycles | 5 cycles |

Value, r1-r12, stack pointer, condition flags, and ARM return state pass. The baseline TinyGo output already constructs this DMA register address using MOV/ORR; consequently no production edit is justified. This one eligible ARM constant does not establish a blanket replacement rule for arbitrary constants or Thumb code.

## Repeatable commands and evidence

From the isolated `dma` repository:

```sh
python3 scripts/dma-check/check.py --runtime /private/tmp/mtile-discord-experiments-ofgo5qwq/baseline/runtime --output-dir /private/tmp/mtile-discord-experiments-ofgo5qwq/dma-results/repro-baseline
python3 scripts/dma-check/check.py --output-dir /private/tmp/mtile-discord-experiments-ofgo5qwq/dma-results/final-check
cd runtime
go test ./...
tinygo build -gc=none -opt=2 -target=target-gba.json -o /private/tmp/mtile-discord-experiments-ofgo5qwq/dma-results/coexist-candidate.elf ./cmd/audiopsgcoexist
```

The baseline regression command intentionally returns 1 with five failures (three unaligned destinations, two missed-overflow cases). The candidate returns zero. The runtime suite passes. `repro-baseline/report.json`, `final-check/report.json`, and `repro-delay-approved/report.json` record the native probes; directories retain ELF/ROM/disassembly/runtime sources. The finalized report records compiler versions and source/library hashes. `coexist-{baseline,candidate}-report.json` contains actual PSG workload telemetry; matching ELF/ROM/raw logs and `coexist.c` remain adjacent. `constants/` retains the assembly, native C checker, binary/ROM, and report. `changes.patch` contains the suggested production fixes and reusable regression checker.

Files ready for integration:

- `runtime/hw/fill_tinygo.go`
- `runtime/hw/mmio_tinygo.go`
- `runtime/hw/fill_host_test.go`
- `scripts/dma-check/README.md`
- `scripts/dma-check/check.py`
- `scripts/dma-check/check.c`
- `scripts/dma-check/fixture.go`

The experimental BIOS wrapper is inserted only into the checker's temporary runtime. There is no production crt0 change.
