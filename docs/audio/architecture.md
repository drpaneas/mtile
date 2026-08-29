# Audio v2 architecture

## Decision

Audio v2 uses only the four Game Boy PSG channels present in the GBA. Direct
Sound, PCM samples, software oscillators, and software mixing are deferred.

```text
fixed command queue
       |
96-PPQN scheduler -- Timer 0 / 256 Hz
       |
 CH1   CH2   CH3   CH4
       |
 GBA hardware PSG mixer
```

## Components

- `runtime/hw/sound_control.go` is the only managed owner of shared sound
  controls and master state.
- `runtime/hw/psg.go` exposes typed CH1–CH4 trigger, legato update, and stop
  operations plus register-equivalent snapshots.
- `runtime/audio/native_controller.go` owns the fixed command queue, rational
  clock, four channel states, song/part cursors, ownership, and telemetry.
- Per-frame target telemetry uses caller-owned `PreviewInto` storage while
  interrupts are masked; it never transports the large snapshot through a
  TinyGo aggregate return.
- Generated assets are immutable slices; target state is fixed-size.
- Editor click audition and live MIDI monitoring share one pure-Go,
  cycle-timed CH1-CH4 renderer. It consumes the same sound definitions as
  target playback and uses band-limited hardware edges rather than host
  oscillators.

## Desktop authoring boundary

The Music Lab has a separate low-latency authoring path:

```text
computer keyboard / Launchkey MIDI
              |
       PerformanceEvent
              |
 MONO/POLY2/POLY3/NOISE allocator
              |
 exact CH1-CH4 actions ----> low-latency host preview stream
              |
raw timestamped take ----> quantizer ----> typed clips/sounds
```

The macOS authoring path uses a 20 ms live source buffer and requests a 40 ms
Oto/CoreAudio device queue. Two source callbacks protect playback from an
ordinary editor frame or short Go GC pause without returning to Oto's old
half-second source buffer. MIDI note-off also gets a 5 ms monitor-only de-click
ramp. These desktop choices do not alter PSG periods, recorded event timing,
or generated cartridge code.

The allocator, MIDI protocol, raw takes, metronome, and host preview stream are
desktop-only. Recorded data crosses the compiler boundary only after it has
become ordinary physical CH1–CH4 events. The cartridge contains no MIDI,
polyphonic voice allocator, take metadata, or desktop renderer.

## Resource ownership

| Resource | Audio v2 use |
| --- | --- |
| CH1–CH4 registers | Managed PSG controller |
| `SOUNDCNT_L` | terminal levels and CH1–CH4 routes |
| PSG ratio in `SOUNDCNT_H` | managed masked field |
| `SOUNDCNT_X` | master enable/status |
| `SOUNDBIAS` | safe default only |
| Timer 0 | 256 Hz event commit clock |
| Timer 1–3 | free for game/runtime |
| DMA1/2 and FIFO A/B | unused by production Audio v2 |

The direct-register truth ROMs may bypass this owner. Generated games may not.

## Interrupt policy

The Timer 0 handler is bounded by four channel states and a 32-entry command
queue. It does not allocate, synthesize samples, loop over an audio buffer, or
wait on DMA. Normal game code writes commands; only the handler commits due
hardware changes.

## Initialization and shutdown

1. Disable the managed timer.
2. Silence CH1–CH4.
3. Set safe bias, PSG ratio, terminal levels, and channel routes.
4. Clear scheduler/ownership/telemetry state.
5. Load validated static tables.
6. Start Timer 0 only when audio becomes active.

Stop performs the reverse while preserving unrelated timers and DMA. Restart
must be indistinguishable from a fresh start.

## Deferred architecture

Existing FIFO and software-mixer code is retained for historical experiments
and v1 projects. It is not linked or initialized by a v2 PSG-only cartridge.
Any future Direct Sound product requires a new explicit plan and schema
version; it does not grow invisibly inside this controller.
