# Four-channel Game Boy audio completion audit

## Status

**PSG-only software, emulator runtime, one-screen Music Lab, performance
recorder, and Launchkey integration are implemented. Physical-GBA and physical
Launchkey sign-off remain external release gates.**

Direct Sound/PCM is deferred and is not part of this audit.

## Latest automated verification — 2026-08-27

- Race-enabled tests pass for the performance allocator/session, MIDI control,
  editor model/layout, and PSG-only asset compiler.
- `go vet ./...` and `golangci-lint run ./...` pass with zero issues in both
  the desktop module and the nested cartridge runtime module.
- Ten-second native-v2 fuzz gates completed 1,752,381 compiler executions and
  1,357,377 runtime-controller executions without a panic or four-channel escape.
- The complete `cmd/mtile` test suite passes with the real TinyGo toolchain.
- The Ebitengine editor package builds and its test binary compiles. Its windowed
  package cannot be executed inside the headless Codex desktop process because
  Ebitengine initializes macOS display services during package startup; all
  display-independent editor behavior is covered under `internal/editor`.
- A fresh `audio-gb-editor` cartridge build succeeds at 57,620 bytes, with
  74,168 bytes of EWRAM free and 2,794 bytes of PSG assets. Its independent
  desktop render is non-silent at 16,384 Hz (peak 67/127, RMS 23.49).
- Fresh builds of all three v2 examples pass, and the six-ROM PSG hardware
  bundle passes hash/manifest validation; its 59 physical-GBA listening checks
  remain intentionally unsigned until run on the device.
- Live keyboard/MIDI preview now applies the saved SOUNDCNT terminal levels,
  25/50/100% PSG ratio, per-sound routing, envelopes, sweep, and hardware
  length cutoff; its reader remains allocation-free and race-tested.
- CH3 pitch now follows the real Wave RAM sample clock in both 32- and
  64-sample modes; compiler/runtime/static-call validation and desktop/live
  preview share dimension-aware regression coverage across the MIDI range.
- Disconnect/all-notes-off regression coverage now includes CH4 pad playback
  while POLY2/POLY3 is selected, including deterministic take finalization.

## Audit as of 2026-08-27

| Requirement | State |
| --- | --- |
| Product scope is exactly CH1–CH4 | Locked and enforced |
| Active plan/schema/API rewritten for PSG-only | Done |
| Direct Sound/PCM removed from release gates | Done |
| CH1–CH4 direct-register truth ROMs | 42/42 host, TinyGo, and mGBA cases pass; physical GBA pending |
| Central shared sound-register owner | Implemented; host tests pass |
| Typed CH1 pulse+sweep, CH2 pulse, CH3 wave, CH4 noise | Implemented; host tests pass |
| 96-PPQN rational four-channel scheduler | Implemented; host tests pass |
| Ownership, loops, note-off, invalid-data, queue tests | Implemented; host tests pass |
| Timer 0 256 Hz target interrupt | Implemented; TinyGo build passes |
| Strict PSG-only JSON v2 parser/compiler | Implemented; unknown/v1/PCM fields rejected |
| Lua v2 lowering and runtime attachment | Implemented; generated no-allocation build passes |
| Single Music Lab with MUSIC/SFX composer, inspector, arrangement | Implemented; headless layout/build checks pass |
| Multi-song, multi-bar meters, reusable/duplicate parts and instruments | Implemented; model/preview round trips pass |
| Common/custom meters and per-part tempo overrides | Implemented; inheritance, boundary timing, compiler, preview, and runtime tests pass |
| Standalone typed PSG SFX with stable `sfx(id)` | Implemented in the shared composer; model/runtime preview tests pass |
| Per-note retrigger/tie articulation | Implemented for CH1–CH3; CH4 legato rejected |
| PSG terminal L/R levels, 25/50/100% ratio, and per-sound routing | Implemented in ADV; preview and generated SOUNDCNT state agree |
| Inline rename, scoped delete, copy/paste, duration/resize, cursor/key highlight | Implemented; sustained-note model, playback, overlap, and end-clamp tests pass |
| Exact 96-PPQN straight/triplet/dotted/swing authoring | Implemented; snap, recording materialization, preview, and schema round trips pass |
| Channel-aware pitch range and rectangular multi-note editing | Implemented; disabled-key, copy/paste/move/delete, collision, and bounds tests pass |
| MONO/POLY2/POLY3/NOISE allocator | Implemented; restoration/overflow tests pass |
| ARM, REC, count-in, click, loop takes, overdub, quantize, swing | Implemented; straight/triplet raw-take and scheduler tests pass |
| Parameter automation materializes real sound variants | Implemented with explicit RETRIGGER/NEXT_NOTE timing and hardware-compatible targets |
| Low-latency desktop PSG preview | Implemented; 20 ms buffer, stereo/global-output parity, hardware-length cutoff, and allocation-free reader tests |
| Independent PSG WAV render/check | Implemented; real CH1–CH4 song renders non-silent PCM16 with duration/peak/RMS telemetry |
| Launchkey Mini 25 MK4 zero-config adapter | Implemented; protocol and fake-hotplug tests pass; physical controller pending |
| Native four-channel examples | Three buildable examples complete |
| CPU/coexistence measurements | Paced mGBA: 1.248% mean, 17.45% maximum slot share; 636.53-second soak: zero errors/frame skips, CH1–CH4 observed |
| Physical-GBA sign-off | Pending |

The physical GBA row changes to complete only when the generated `RESULTS.md`
passes both `scripts/audio/verify_hardware_signoff.sh` and
`scripts/audio/verify_physical_signoff.py`. The Launchkey row changes to fully
signed off only after connection, keys, pads, encoders, transport, LEDs/OLED,
hotplug, and audible latency are exercised on the named device.

## Milestones

| Milestone | State |
| --- | --- |
| 0 PSG-only contract | Complete |
| 1 CH1–CH4 truth ROMs | Emulator complete; physical pending |
| 2 Managed PSG hardware | Complete |
| 3 Scheduler, assets, APIs, and preview truth | Complete |
| 4 One-screen Music Lab | Software complete; manual desktop sign-off pending |
| 5 Performance engine and recorder | Complete |
| 6 Launchkey integration | Software complete; physical controller pending |
| 7 Release verification | Automated/emulator gates complete; physical gates pending |

## Completion rule

Audio v2 release sign-off requires every gate in [`plan.md`](plan.md), including
simultaneous CH1–CH4 playback under graphics load, physical-GBA listening, and
the physical Launchkey acceptance run. FIFO/PCM experiments neither block nor
satisfy this rule.
