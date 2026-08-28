# Four-channel music

A song is an arrangement of named parts. A part contains up to one clip for
each fixed hardware channel.

```text
song -> ordered parts -> CH1/CH2/CH3/CH4 clips -> timed note events
```

Clips are typed. A CH1 clip can use only CH1 sounds, a CH2 clip only CH2
sounds, and so on. CH4 events trigger noise sounds and do not pretend to be
pitched piano notes.

Timing uses 96 PPQN and a 256 Hz hardware commit clock. The scheduler retains
the rational remainder across ticks and repeated parts, so long loops do not
drift. Each song has a base tempo and each part may override it from 20 through
400 BPM. Overrides apply only at part boundaries; an omitted/zero part tempo
inherits the song value.

Each event has tick, duration, note, sound ID, priority, and optional legato.
Notes cannot overlap inside one clip. Legato updates frequency without a new
trigger only when the existing compatible sound remains active; CH4 rejects
legato because noise has no frequency-only update operation.

The straight editor grid reaches 1/64 note (6 ticks), while all events retain
exact 96-PPQN ticks. This permits exact quarter/eighth/sixteenth triplets,
dotted durations, and straight-grid swing without rescaling existing assets.
A part records its bar count and time signature; its exact `lengthTicks`
remains the runtime authority. The editor supports 1–16 bars, common simple,
compound, and odd meters, plus custom numerators and power-of-two beat units.

Each project may contain multiple songs with stable IDs. A song sequence is
explicit. `loopStart` is either no loop or one sequence
index. End stops song-owned notes but preserves SFX/direct ownership.

There are no PCM sample lanes in v2.

## Authoring in Music Lab

The active part's four hardware clips and the complete song order are visible
at the same time. Selecting an order card edits that part in place; adding,
renaming, duplicating, moving, deleting, looping, and ending parts does not
open another page. A retained performance take can be selected and reapplied
at Off, straight 1/4 through 1/64, or quarter/eighth/sixteenth-triplet
quantization. Straight grids also support 50–66% swing. Raw timing remains the
source for every re-quantization.

The piano roll supports rectangular multi-note selection. Copy/paste and move
preserve exact relative timing, duration, sound, priority, and articulation;
channel-kind and native pitch limits remain enforced.

`+ NEW` creates an independent part. `+ REUSE` inserts the same part ID again,
so edits update all occurrences. Duplicate creates new part/clip IDs instead.
Reusable per-channel sounds are selected above the hardware inspector.
