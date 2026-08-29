# PSG performance

## Budget model

Audio v2 performs no per-sample work. CH1–CH4 generate sound in hardware. The
CPU runs one 256 Hz Timer 0 service that:

- drains a bounded command queue;
- advances a rational song clock;
- checks four note-off states; and
- writes only due channel/shared registers.

The desktop live-preview stream is not part of this target budget. It exists
only while authoring and translates the same physical channel actions that the
recorder stores. On macOS its source player keeps 20 ms of PCM while the host
device queue is explicitly requested at 40 ms; the previous platform-default
queue was roughly 128 ms. None of these host buffers enter generated ROM data.

## Parameter timing contract

Every controller parameter has one named timing semantic shared by live
preview and tracker materialization:

- `RETRIGGER`: duty, CH3 level, and CH4 polynomial controls restart the active
  physical channel at that exact tick.
- `NEXT_NOTE`: pulse/noise envelope and CH1 sweep edits update the sound bank
  without disturbing a held note; the next trigger uses them.
- `IMMEDIATE`: reserved for a future control with a documented safe live PSG
  register write. No current performance knob claims this behavior.

Raw takes retain all parameter timestamps in every case. Quantization may move
the tracker tick, but it never silently changes the parameter's semantic.
Straight quantization can apply 50–66% swing; triplet grids remain exact and
do not accept swing. The piano-roll snap is a separate authoring control, so
changing how new notes align never rewrites a retained performance take.

## Required measurements

Measure on generated target code:

| Case | Required |
| --- | --- |
| idle timer, no events | service baseline |
| one due CH1 event | pulse+sweep write cost |
| CH3 waveform replacement | worst Wave RAM update |
| four simultaneous starts | normal maximum event burst |
| 32 queued commands | hard queue-drain bound |
| four note-offs plus part boundary | ownership/transition burst |

Record minimum/mean/p95/maximum cycles, queue overflow, applied/rejected event
counts, and game frame cost with empty, sprite-heavy, and dense-map scenes.

## Measured coexistence result

The production `runtime/cmd/audiopsgcoexist` cartridge runs CH1–CH4 together
with the graphics stress matrix. The 2026-08-26 mGBA 0.10.3 run reported:

| Metric | Result |
| --- | ---: |
| 256 Hz service cycles, minimum | 117 |
| 256 Hz service cycles, mean | 818 |
| 256 Hz service cycles, maximum | 11,439 |
| Maximum 65,536-cycle service-slot share | 17.45% |
| Mean ARM7 CPU share | 1.248% |
| Applied events | 149 |
| Scheduler service calls | 2,432 |
| Graphics frame skips | 0 |
| Hardware channels observed | `0x0f` (CH1–CH4) |
| ROM size | 32,632 bytes |
| Target heap allocations | 0 |
| PSG controller state/queue in EWRAM | 688 bytes |
| Caller-owned coherent preview in EWRAM | 252 bytes |
| Four-channel fixture tables in IWRAM | 700 bytes |
| Timer service call graph in IWRAM | 6,612 bytes |

The maximum remains below the explicit 16,384-cycle per-service budget. The
mean share is `818 / 65,536`, independent of the 30 Hz game frame because the
sound commit clock is 256 Hz.

The same ROM also passed an unthrottled long-run gate for 162,952 scheduler
services (636.53 seconds of emulated playback): 10,181 events, all four
hardware channels observed, zero frame skips, zero queue overflows, zero
rejected commands, and a clean fail latch. Target snapshots were written via
caller-owned `PreviewInto` memory throughout this soak.

The table sizes are workload-dependent; the 700-byte figure contains four
sounds, sixteen note events, four clips, one part, and one song. The controller
size is fixed. There is no PCM ring, sample buffer, FIFO staging page, or audio
DMA descriptor in either memory region.

## Acceptance

- No target allocation after initialization.
- No missed 256 Hz commits in supported scenes.
- No audio DMA, PCM buffer, or FIFO memory cost.
- CH3 worst-case Wave RAM update remains bounded and does not regress frames.
- Audio stop releases Timer 0 and leaves unrelated timers/DMA unchanged.
- Release docs publish measured maximum cycles and ROM/EWRAM/IWRAM bytes.
- Live keyboard/MIDI callbacks remain independent of the editor frame loop and
  the host audio reader performs zero allocations while filling a buffer.
