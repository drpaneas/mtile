# Legacy PICO-style music assets and API

> Historical version-1 format. Native GBA audio v2 does not reinterpret this
> schema; see `../../music.md` for the replacement model.

## Status

The version-1 pattern schema, strict build validation, fixed-memory runtime,
real `music()` lowering, sample-clocked transitions, looping, and SFX channel
interaction are implemented. The path uses the provisional 16,384 Hz mono
stream and still needs audible and real-hardware sign-off. The music editor,
target-runtime host preview, preview-only mute/solo, and sample-continuous
`music()` fades are implemented.

## Project file

Music lives in `assets/audio/music.json`:

```json
{
  "version": 1,
  "name": "Adventure Starts",
  "patterns": [
    {
      "id": 0,
      "name": "intro",
      "channels": [0, 1, 2, -1],
      "next": 1
    },
    {
      "id": 1,
      "name": "loop",
      "channels": [3, 1, 2, -1],
      "next": 1
    }
  ]
}
```

`name` is optional editor-facing song metadata. It does not replace stable
pattern IDs or change `music()` calls.

Each stable pattern ID is both a `music()` entry point and one node in the
song arrangement. `next` selects the pattern reached at the exact end of the
current pattern. A backward link creates a loop; `next: -1` stops. This graph
keeps intros, loops, alternate entry points, and one-shot songs in one compact
format without a separate song table.

Version 1 requires:

- Up to 64 patterns with unique stable IDs from 0 through 63.
- Unique non-empty names with at most 64 non-control characters.
- Exactly four channel slots per pattern.
- Each slot is an existing SFX ID or `-1` for no music assignment.
- At least one assigned SFX per pattern.
- An explicit `next` containing an existing pattern ID or `-1`.

Unknown fields and versions, duplicate IDs/names, missing SFX or transition
references, wrong channel counts, and fully silent patterns fail before
TinyGo runs. A missing file means an empty version-1 music library, preserving
existing projects.

## Pattern duration and transitions

A PICO-imported pattern follows the left-most non-looping assigned SFX. If
every assigned SFX loops, it uses the longest complete assigned SFX duration:

```text
duration = first_non_looping(SFX speed * SFX step count)
        or max(looping SFX speed * full authored step count)
```

One PICO-8 speed unit is 183 exported samples at 22,050 Hz. At 16,384 Hz the
equivalent target count is a rational `499712/3675` samples, so cumulative
boundaries alternate between 136 and 137 samples without drift. Transition
timing therefore stays sample-accurate and independent of the 30 Hz game
loop. Shorter channels may finish before the pattern ends. An SFX-local loop
can continue within the pattern, but it does not make the music pattern
infinite; the compiled full SFX duration remains its clock.

At a transition, finite non-negative assignments start their SFX from step zero
on the matching logical channel, including an unchanged finite ID that was
still active. An unchanged looping assignment retains its independent clock
across rows. A `-1` slot stops a previous music-owned SFX on that channel but
does not stop a manually triggered SFX.

## Lua API

```lua
music([pattern_id [, fade_ms]])
```

- `music(n)` starts or restarts stable pattern ID `n` immediately on the
  producer timeline.
- `music(-1)` stops all music-owned channels.
- `music()` is the same immediate stop operation.
- Invalid pattern IDs fail closed and increment bounded telemetry.
- `music(n, fade_ms)` starts pattern `n` immediately at silence and raises
  music gain linearly to full scale.
- `music(-1, fade_ms)` lets the current arrangement continue while its gain
  falls linearly, then releases only music-owned channels.
- Fade lengths are integer milliseconds from 0 through 32,767. Positive
  lengths round up to a complete four-sample FIFO word at 16,384 Hz; for
  example, 1 ms becomes 20 samples. Negative and out-of-range values fail
  closed.

A literal missing pattern ID is rejected during transpilation when the project
links audio, with its Lua source position and `assets/audio/music.json` in the
diagnostic. `music()` and `music(-1)` remain valid stop calls. Variable and
expression IDs continue through the bounded runtime validation path.

Fade gain is independent from authored per-note volume and fade effects. Its
first sample uses the starting endpoint; after the compiled number of samples
has been emitted, gain is exactly the target endpoint. A pattern transition
during a long fade preserves the gain clock. The fade argument follows the
[official PICO-8 `music()` surface](https://www.lexaloffle.com/dl/docs/pico-8_manual.html#MUSIC),
with Mtile's target-word rounding documented explicitly above. The separate
PICO-8 `channel_mask` argument is not part of Mtile's version-1 call surface.

## Interaction with `sfx()`

Music and manual SFX share the same four logical channels and four mixer
voices.

- `sfx(id, channel)` immediately takes that logical channel from music on the
  updated producer timeline.
- Music reclaims assigned channels at the next pattern transition.
- A `-1` music slot leaves a manually triggered SFX alone.
- `music(-1)` stops only channels still owned by music; interrupted/manual
  SFX continue.
- Automatic `sfx(id)` chooses the first inactive logical channel, then the
  oldest. In a three-channel song it normally uses the fourth channel; when
  all four are occupied it may interrupt a music channel until transition.

These rules are deterministic and covered at both direct runtime and queued
stream-rewind boundaries.

## Fixed-memory scheduling

Music definitions remain in ROM. The runtime adds only bounded pattern state,
ownership bits, counters, and one music snapshot to each of the existing four
SFX checkpoints. `music()` and `sfx()` share the fixed eight-command queue.
After stream startup, a command restores the page-after-next checkpoint and
regenerates that page, preserving the current and immediate-next safety pages.
Normal call latency therefore remains roughly 15.6..31.25 ms with a full
queue.

The current target map reports a 19,376-byte `SFXPlayer`, including four mixer,
SFX, music, filter, and gain-clock checkpoints plus fixed filter histories.
The stream remains 1,936 bytes EWRAM and the mixer is 1,328 bytes IWRAM. Build
output reports exact per-project ROM and remaining EWRAM. Mixing, pattern
transitions, and fade advancement allocate nothing.

`SFXStats` exposes `MusicActive`, `MusicPattern`, `MusicStarts`, `MusicStops`,
`MusicTransitions`, `InvalidMusicCalls`, `MusicFadeSamplesRemaining`, and
`MusicFadingOut` alongside the shared command and rewind counters.

On the current TinyGo/mGBA ARM7TDMI benchmark, four music-owned channels take
33,808 cycles per 256 samples at unity gain and 44,276 cycles (16.9% of the
16,384 Hz page budget) during a one-second global fade. The continuous
music-fade graphics matrix completes all seven unforced workloads with zero
underruns and command fallbacks. Empty, sparse-map, and complete GBA-class
frames fit one 30 Hz work budget; clear/HUD and the three heavier graphics
stress cases remain over budget. Real-hardware listening remains open.

## Editor workflow

Open a project in the bundled editor, select **Audio Studio**, then **Song**.
Pressing `3` opens Audio Studio directly:

```sh
go run ./cmd/mtile edit examples/audio-music/
```

Song opens one integrated **Music Lab**. The left side contains friendly
instruments grouped as Lead, Bass, and Beat. The arrangement runs left to right
with four colored tracks: **Melody**, **Harmony**, **Bass**, and **Drums**. The
lower half is the selected clip's piano roll and clickable keyboard.

For an empty song, pick an instrument and click **Start Song**. Section 1 and
its first clip are created without leaving the screen. Click or drag in the
piano roll, click the keyboard, or use `Z` through `M` to enter notes. Clicking
an empty arrangement block creates a clip from the selected instrument;
clicking a colored block edits that clip in place. **Sound Details** opens the
advanced Sounds view only when loop bounds, note effects, filters, or stable
IDs are needed.

`+ Section` inserts after the selected section; Copy duplicates all four
tracks. **Next**, **Loop**, and **End** define what happens afterward. Visible
left/right buttons scroll arrangements up to the 64-section limit. Each track
has preview-only Mute and Solo controls, and `music(id)` remains visible beside
the selected section without making IDs part of composition.

The top bar edits optional song-title metadata. Tempo is presented as BPM for
new songs and changes all clips referenced by the song transactionally. An
imported song whose clips intentionally use different speeds displays
**Mixed** until the composer chooses one shared tempo; unrelated standalone
SFX are never retimed.

Space starts or restarts preview and Escape stops it. Mute and solo are host
transport state only and are never saved to `music.json`. The editor
compiles the in-memory assets and renders through the same `runtime/audio`
mixer, SFX player, music sequencer, and 16,384 Hz sample clock as a generated
cartridge. Finite arrangements stop naturally; looping previews are capped at
eight seconds.

Fade duration belongs to a `music()` call rather than to a pattern asset, so
the editor's normal Play button remains immediate. `RenderMusicPreviewFade`
is available to host tools and tests that need call-site fade rendering.

Then build the project:

```sh
go run ./cmd/mtile build examples/audio-music/
```

Direct JSON editing remains supported. See [`examples/audio-music`](../../../../examples/audio-music)
for an intro, a two-node loop, three simultaneous music channels, explicit
interruption, automatic SFX assignment, restart, and stop behavior.
