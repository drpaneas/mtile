# Four-channel Game Boy audio schema v2

## Contract

Version 2 is strict and PSG-only. Parsers reject unknown fields, unknown
versions, duplicate IDs, incompatible channel assignments, out-of-range
hardware values, PCM/sample fields, and version-1 PICO files.

## Limits

| Item | Limit |
| --- | ---: |
| Sounds | 256 |
| Clips | 256 |
| Parts | 128 |
| Songs | 64 |
| Compiled events | 8,192 |
| Name | 48 UTF-8 bytes |
| Authoring clock | 96 PPQN |
| Tempo | 20–400 BPM |
| MIDI-style note | 0–127, additionally checked against channel range |

Tempo changes occur only at part boundaries. A retained rational accumulator
maps 96-PPQN ticks to the fixed 256 Hz hardware commit clock without drift.

## `assets/audio/sounds.json`

```json
{
  "version": 2,
  "sounds": [
    {
      "id": 0,
      "name": "bright-lead",
      "kind": "ch1",
      "route": "both",
      "pulse": {
        "duty": 50,
        "length": 0,
        "useLength": false,
        "envelope": {"initial": 15, "direction": "down", "pace": 2},
        "sweep": {"pace": 2, "direction": "up", "shift": 1}
      }
    },
    {
      "id": 1,
      "name": "soft-wave",
      "kind": "ch3",
      "route": "both",
      "wave": {
        "samples": [0, 2, 4, 7, 10, 13, 15, 13, 10, 7, 4, 2, 0, 0, 0, 0,
                    0, 0, 0, 0, 2, 4, 7, 10, 13, 15, 13, 10, 7, 4, 2, 0],
        "level": 100,
        "startBank": 0,
        "length": 0,
        "useLength": false
      }
    }
  ]
}
```

Each sound has exactly one payload matching `kind`:

| Kind | Required controls |
| --- | --- |
| `ch1` | duty, length, hardware envelope, hardware sweep |
| `ch2` | duty, length, hardware envelope; sweep forbidden |
| `ch3` | 32/64 4-bit samples, start bank, hardware output level, length |
| `ch4` | 7/15-bit width, divisor, shift, length, hardware envelope |

Routes are `none`, `left`, `right`, or `both`. Duty is 12.5, 25, 50, or 75.
Envelope initial volume is 0–15, direction is `up`/`down`, and pace is 0–7.
Sweep pace and shift are 0–7. CH3 samples are 0–15 and level is
0/25/50/75/100. CH4 divisor is 0–7 and shift is 0–13.

Friendly presets are materialized into these fields before save.

## `assets/audio/music.json`

```json
{
  "version": 2,
  "ppqn": 96,
  "clips": [
    {
      "id": 0,
      "name": "intro-lead",
      "kind": "ch1",
      "lengthTicks": 384,
      "events": [
        {"tick": 0, "durationTicks": 96, "note": 60, "soundId": 0, "priority": 100},
        {"tick": 96, "durationTicks": 96, "note": 64, "soundId": 0, "priority": 100, "legato": true}
      ]
    }
  ],
  "parts": [
    {
      "id": 0,
      "name": "intro",
      "lengthTicks": 384,
      "bars": 1,
      "beatsPerBar": 4,
      "beatUnit": 4,
      "tempo": 140,
      "lanes": {"ch1": 0, "ch2": null, "ch3": null, "ch4": null}
    }
  ],
  "songs": [
    {"id": 0, "name": "new-song", "tempo": 120, "sequence": [0], "loopStart": 0}
  ],
  "takes": [
    {
      "id": 0,
      "name": "Take 1",
      "partId": 0,
      "durationNanos": 500000000,
      "events": [
        {"offsetNanos": 0, "kind": "start", "channel": 1, "note": 60},
        {"offsetNanos": 500000000, "kind": "stop", "channel": 1, "note": 60}
      ]
    }
  ]
}
```

A clip targets exactly one hardware channel kind. Events are sorted and may
not overlap inside a clip. Duration is at least one tick and schedules an
explicit stop. `legato` is optional and valid only for CH1–CH3. A part assigns
at most one compatible clip to each of CH1–CH4.
`loopStart` is null for an ending song or an index into `sequence`.

`bars`, `beatsPerBar`, and `beatUnit` preserve editor meter semantics and must
multiply to exactly `lengthTicks`; old v2 files that omit all three retain the
length-only interpretation. An omitted or zero part `tempo` inherits the song;
an explicit override is 20–400 BPM and takes effect at that part boundary. A
clip with `"sfx": true` is a standalone SFX and cannot be assigned to a song
part. Its clip ID is the stable runtime `sfx(id)`. The explicit marker prevents
an unassigned/deleted music clip from silently becoming gameplay API. Multiple
songs share the same sound/clip/part libraries but retain independent
sequences, base tempo, part overrides, and loop.

`takes` is optional editor-only source data. It retains raw nanosecond timing,
exact physical channel actions, and hardware-parameter changes so a performance
can be quantized again without cumulative rounding. It is validated on load
but stripped from generated cartridge tables.

## `assets/audio/settings.json`

```json
{
  "version": 2,
  "profile": "gameboy-psg",
  "leftLevel": 7,
  "rightLevel": 7,
  "psgRatio": 100
}
```

`gameboy-psg` is the only accepted profile. Terminal levels are 0–7. PSG ratio
is 25, 50, or 100 percent. There are no PCM rate, voice count, FIFO, bus,
sample, pan, or Direct Sound settings.

## Legacy and deferred fields

Entering Music Lab or building fails with a superseded-format diagnostic if a
project has v1 `sfx.json`, v1 music/settings, `samples/manifest.json`, or
experimental v2 PCM/FIFO fields. The sprite/map editor remains accessible.
Conversion is explicit and never mutates source assets.
