# Legacy audio v1 completion audit

> Historical audit for the superseded PICO-compatible audio system. It is not
> the completion status of native GBA audio v2.

## Outcome

The version-1 implementation is complete through host, generated-code, TinyGo,
allocation, exact mGBA benchmark, graphics-coexistence, deterministic capture,
and strengthened ten-minute emulator-soak gates. External PICO-8 conformance
passes all 870/870 instrument, pitch, note-effect, filter, and speed cases.
The supplied corpus additionally exports all 229 carts and 289 distinct music
roots and compares 153 paired album references, including the reconstructed
interactive Slipways arrangement.
The initiative is **not yet fully signed off** until Definition of Done item
13's physical-GBA observation passes.

The exact external procedure and ROM hashes are in
[`hardware-signoff.md`](hardware-signoff.md). No emulator result is counted as
physical evidence.

## Definition-of-done evidence

| # | Requirement | Status | Evidence |
| ---: | --- | --- | --- |
| 1 | Real useful `sfx()` and `music()` | complete | Runtime/transpiler tests, [SFX guide](sfx.md), [music guide](music.md), and the SFX/music examples. |
| 2 | PCM/native voices coexist with PICO audio | complete | Mixed `audio-samples` example, twelve-slot transient/PCM allocator, and all PCM coexistence matrices. |
| 3 | Stereo/pan if hardware approves | deferred by measured v1 decision | Low-level stereo/pan/capture exists; four-voice stereo fails the graphics gate and project exposure is disabled. Mono does not pay stereo state cost. |
| 4 | SFX, music, samples, settings editable | complete | Audio Studio authors all four documents; Music Lab integrates instruments, arrangement, piano roll, keyboard, tempo, and playback, while Sounds, Import WAV, and Diagnostics progressively expose specialist controls. |
| 5 | Preview matches target generation | complete through deterministic target evidence | Editor previews use `runtime/audio`; golden/property tests compare byte/word paths; mGBA captures verify generated cartridges. Physical listening remains item 13. |
| 6 | Fixed-memory and allocation-free runtime | complete | Fixed queues/pages/voice pools and TinyGo `-gc=none -print-allocs=.` gates for generated examples and target ROMs. |
| 7 | Underruns, steals, CPU, ROM, EWRAM observable | complete | `stat(80..89)`, `Context.AudioStats()`, build output, editor dashboard, target map sizes, and benchmark telemetry. |
| 8 | Survives documented graphics workloads | complete for documented support boundary | Every unforced standard/PCM matrix case has zero underruns; capacity limits are reported rather than hidden. |
| 9 | Hardware ownership documented and tested | complete in code/mGBA; physical observation pending | [Architecture ADRs](architecture.md), FIFO/timer/DMA probes, Mode 4 DMA3 matrices, stop/restart tests, and hardware handoff ROM. |
| 10 | Examples cover SFX, music, samples, mixed playback | complete | `examples/audio-sfx`, `examples/audio-music`, and mixed synth+PCM `examples/audio-samples`. |
| 11 | CI covers host/assets/generated/TinyGo/allocations | complete | Cross-platform tests/vet/lint, fuzz smoke, all audio examples, generated packages, matrices, both hardware ROMs, and stereo experiments. |
| 12 | Complete PICO-8 external conformance gate | complete | All 870/870 isolated cases pass; all 229 supplied carts and 289 graph roots export; 153 album references are compared and classified. See [conformance](conformance.md). |
| 13 | Physical GBA pass for every production profile | pending external run | Exact probe, four-synth, and two-synth+PCM ROMs/checklist are prepared in `hardware-signoff.md`. |

## Final measured software baseline

- Output: signed 8-bit mono FIFO A at exactly 16,384 Hz.
- Standard workload: four sustained mixer voices; generated cartridges reserve
  eight slots so four 48-sample release tails can overlap incompatible
  replacements. Compatible cells and channel retriggers preserve oscillator
  phase instead of producing a speed-locked pulse. The
  sustained four-voice baseline is 31,689 cycles;
  3/7 graphics workloads fit one 30 Hz work budget.
- Sample workload: twelve-slot generated allocator; the measured sustained
  four-synth-plus-PCM continuity case passes
  and 2/7 workloads fit; the documented two-synth-plus-PCM boundary fits 5/7.
- Worst eight-command note-effect producer: 155,654 cycles, 59.4% of one
  256-sample page and 50.0% of the 304-sample guard.
- Retained state when linked: 1,936 bytes stream EWRAM, 19,376 bytes
  player/checkpoint/filter-history EWRAM, and 1,328 bytes mixer IWRAM.
- Standard ten-minute mGBA soak: 18,000 ticks, 48,634 boundaries, 120 rewinds,
  60 completed fade cycles, zero pending/fallback/drop/underrun/switch failure.
- Two-synth+PCM ten-minute mGBA soak: 18,000 ticks, 38,646 boundaries, 120
  rewinds, 60 completed fade cycles, one retained PCM voice, and the same zero
  failure counters.

## Reproducible verification

```sh
go test ./...
go vet ./...
cd runtime
go test ./...
go vet ./...
make smoke-gba audiobench-gba audiocoexist-gba \
  audiopcmcoexist-gba audiopcmthreecoexist-gba audiopcmtwocoexist-gba \
  audiohardware-gba audiohardwarestandard-gba \
  audiostereobench-gba audiostereostream-gba audiostereocoexist-gba
cd ..
scripts/audio/extract_audiobench.sh
scripts/audio/extract_audiocoexist.sh
scripts/audio/extract_audiopcmcoexist.sh
scripts/audio/extract_audiopcmthreecoexist.sh
scripts/audio/extract_audiopcmtwocoexist.sh
scripts/audio/soak_audiohardware_mgba.sh
scripts/audio/soak_audiohardwarestandard_mgba.sh
```

The licensed external PICO-8 gate runs separately from CI:

```sh
scripts/audio/run_pico_instruments.sh /tmp/mtile-pico-instruments
scripts/audio/run_pico_effects.sh /tmp/mtile-pico-effects
scripts/audio/run_pico_filters.sh /tmp/mtile-pico-filters
scripts/audio/run_pico_speeds.sh /tmp/mtile-pico-speeds
scripts/audio/check_pico_conformance.py \
  --instruments /tmp/mtile-pico-instruments \
  --effects /tmp/mtile-pico-effects \
  --filters /tmp/mtile-pico-filters \
  --speeds /tmp/mtile-pico-speeds
python3 scripts/audio/inventory_pico8_corpus.py \
  --converted-dir /tmp/mtile-pico8-carts Pico-8 \
  /tmp/mtile-pico8-inventory.json
python3 scripts/audio/pair_pico8_corpus.py \
  /tmp/mtile-pico8-inventory.json /tmp/mtile-pico8-pairs.json
python3 scripts/audio/run_pico8_corpus.py \
  /tmp/mtile-pico8-pairs.json /tmp/mtile-pico8-run
python3 scripts/audio/audit_pico8_corpus.py \
  /tmp/mtile-pico8-pairs.json /tmp/mtile-pico8-audit
```
