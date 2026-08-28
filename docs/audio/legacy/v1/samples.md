# Legacy audio v1 PCM samples and API

> Historical single-mix-bus sample workflow. Native GBA audio v2 treats PCM
> voices as software sources routed to Direct Sound A/B buses.

## Status

Milestone 4's version-1 asset, runtime, transpiler, editor, and example paths
are implemented on the provisional 16,384 Hz mono stream. WAV sources compile
to signed 8-bit target PCM, generated cartridges select the twelve-slot
profile, and `sample()` shares the existing checkpointed command path with
SFX/music. The four-synth-plus-PCM cartridge profile passes the complete mGBA
continuity and starvation-recovery matrix. Its optimized mixed-fade path fits
the empty and sparse-map scenes; the documented two-synth-plus-PCM safety
boundary fits five common scenes, including complete PICO- and GBA-class
ticks. The 100-sprite and dense-map stresses plus real-hardware listening
remain outside that candidate. Stereo source preview is implemented without
changing the target format or enabling project-level stereo.

## Project files

Sample metadata lives in `assets/audio/samples/manifest.json`; WAV sources stay
beside it:

```json
{
  "version": 1,
  "samples": [
    {
      "id": 0,
      "name": "impact",
      "source": "impact.wav",
      "targetRate": 16384,
      "gain": 256,
      "priority": 96,
      "normalize": true,
      "trimSilence": true
    },
    {
      "id": 1,
      "name": "engine-loop",
      "source": "engine.wav",
      "targetRate": 16384,
      "gain": 112,
      "priority": 48,
      "loop": {"start": 0, "end": 4096}
    }
  ]
}
```

Version 1 accepts:

- Up to 64 samples with unique stable IDs 0..63 and unique names.
- Clean relative source paths confined to `assets/audio/samples/`.
- RIFF/WAVE integer PCM format 1, mono or stereo, 8/16/24-bit, at source
  rates from 1 through 192,000 Hz.
- The single measured target rate, 16,384 Hz mono signed 8-bit PCM.
- Default gain 1..256 and priority 0..255.
- Optional normalization and leading/trailing silence trimming.
- Optional half-open loop points `[start,end)` measured in source WAV frames.

The converter averages left and right with integer arithmetic before trimming,
normalization, linear resampling, loop-boundary remapping, and signed 8-bit
quantization. That order is the version-1 stereo conversion policy and remains
deterministic across builds. A converted asset is limited to 262,144 frames;
total unique PCM is limited to 1 MiB. Identical converted samples share one ROM
range.

Unknown fields/versions, malformed WAV chunks, duplicate IDs/names, escaping
paths, invalid loops, and unsupported rates fail before TinyGo starts. Gain is
runtime metadata rather than being baked into PCM, so call-site volume
overrides do not apply the default twice.

## Lua API

```lua
sample(id [, volume [, priority]])
```

- `sample(id)` starts a native PCM voice using manifest volume and priority.
- `volume` overrides the default with 0..256.
- `priority` overrides the default with 0..255.
- `sample(-1)` stops all active PCM voices but leaves synth SFX/music alone.
- Multiple calls may overlap. A looping definition repeats its compiled
  half-open loop range until stopped or stolen.
- Invalid IDs/arguments and starts blocked by voice policy fail closed and
  increment fixed runtime telemetry.

When audio is linked, a literal missing sample ID fails transpilation before
TinyGo and names `assets/audio/samples/manifest.json` in the source-positioned
diagnostic. `sample(-1)` remains valid. Variable and expression IDs are checked
at runtime so gameplay-selected samples still work normally.

Projects containing samples use twelve shared mixer slots. Four PICO channels
preserve phase across compatible cells and can each overlap one short release
tail when a source must be replaced, leaving four slots for native or manual
overlap. This is an allocation limit, not a CPU support promise: four
sustained synth voices plus one PCM stream fit 2/7
measured scenes, three plus PCM fit 4/7 because the PICO tick reaches 100.8%,
and the documented safety boundary is two synth voices plus one PCM stream at
5/7. A start uses an inactive voice first. If all twelve are active, it may
replace the oldest voice among the lowest priorities no greater than the new
request; protected voices cannot be stolen. The pool is shared rather than
partitioned, so a sufficiently high-priority sample can deliberately replace
a synth voice and vice versa.

After stream startup, sample start/stop calls use the same bounded eight-command
queue and page rewind as `sfx()` and `music()`. Mixer snapshots copy active
voices only and contain PCM position and loop state, so regenerated pages
remain deterministic and no new target allocation is required. `SFXStats`
includes active PCM voices,
starts, stops, failed starts, and invalid calls.

## Editor workflow

Open a project and select **Audio Studio > More… > Import WAV…**:

```sh
go run ./cmd/mtile edit examples/audio-samples/
```

Drop one or more `.wav` files anywhere on the editor window. Imports are copied
into `assets/audio/samples/`, decoded and converted before metadata is committed,
then assigned the first free stable ID. A failed import removes its partial
copy. Deleting a sample removes only metadata and preserves the source WAV, so
undo remains non-destructive.

The inspector edits name, stable ID, default volume/priority, normalization,
silence trimming, and source-frame loop bounds. All edits validate the complete
manifest transactionally and participate in Ctrl/Cmd+Z history. The panel shows
source bytes, total target PCM ROM bytes, and the fixed target rate.

For a stereo WAV, the upper waveform shows separate left/right lanes and Play
Source preserves both channels while adapting them independently to the host
output clock. Mono sources retain one lane and are duplicated only in the final
desktop stereo container. The lower waveform and Play Target use the exact
version-1 mono ROM bytes, authored gain, loop semantics, and `runtime/audio`
mixer arithmetic. This makes the stereo-to-mono result audible before a build.

Version 1 does not store per-sample pan or stereo target PCM. Those are future
schema/runtime decisions and remain unavailable while project-level stereo is
experimental; importing a stereo WAV cannot silently opt a project into the
dual-FIFO backend.

## Build output and example

Builds with audio print the selected profile and packed audio cost, for example:

```text
built .../game.gba (56472 bytes, 65444 bytes EWRAM free)
audio: 16,384 Hz mono, 12 voices, master 256/256, 7,998-byte assets
```

See [`examples/audio-samples`](../../../../examples/audio-samples) for one-shot,
looping, default/overridden volume and priority, PCM-only stop behavior, and
mixed playback under two looping music-owned synth voices.
