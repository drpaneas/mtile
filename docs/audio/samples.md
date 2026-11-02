# Import music and recorded sounds

You can use the same **Direct Sound input formats as Butano**:

| What you have | Files | Lua call |
| --- | --- | --- |
| A recorded sound, voice or short effect | Uncompressed PCM `.wav` | `sample(id)` |
| A tracker song, with notes and sampled instruments | `.mod`, `.xm`, `.s3m`, `.it` | `pcm_music(id)` |
| A song made in mTile's four-channel Music Lab | Existing PSG JSON assets | `music(id)` |

A WAV is a recording: it stores the waveform over time. A tracker song is
closer to a score plus its instruments. It can repeat a small instrument many
times, which usually takes much less cartridge space than a full recording.
Butano uses Maxmod for these Direct Sound assets; mTile now uses the same
BlocksDS Maxmod player and the `mmutil` converter. See the
[Butano import guide](https://gvaliente.github.io/butano/import.html).

## Bring files into a project

Install **mmutil**, the Maxmod converter supplied by the Butano/devkitPro or
Wonderful/BlocksDS toolchain. It must be on your `PATH`. Alternatively set
`MMUTIL` or pass `-mmutil /path/to/mmutil`. The upstream converter and build
instructions are at [BlocksDS mmutil](https://github.com/blocksds/mmutil).
This tool is needed when importing or updating source audio; building an
already imported project needs only mTile and TinyGo.

```sh
mtile import-audio -project mygame theme.xm jump.wav
```

The command prints the IDs to use:

```text
pcm_music(0)  theme
sample(0)     jump
```

Use simple filenames such as `theme`, `jump` or `boss_theme`: a letter followed
by letters, numbers or underscores. File extensions are case-insensitive.
For WAV, export uncompressed 8-bit or 16-bit PCM; float, 24-bit and ADPCM
encodings are rejected by mmutil. Mono is a good choice for short effects.
Importing another file adds it to the existing bank; reimporting the same name
updates it and preserves its public ID. Song and effect IDs are independent.

The importer stores copies of source files and a compiled bank under
`assets/audio/direct/`. Commit that complete directory. A content-addressed
bank is published before its manifest, so a failed conversion leaves the
previous bank playable. Old bank/source files are not automatically deleted.
Keep the generated manifest intact; reimport the sources to change it.

## Play the result

```lua
function _init()
  pcm_music(0)       -- loop the imported tracker song
end

function _update()
  if btnp(5) then sample(0) end
end

function _draw()
  cls(1)
end
```

Build normally with `mtile build mygame`, or open it with `mtile edit mygame`
and **Build + Reload**. The cartridge preview plays the ROM's actual audio.
Music Lab continues to edit PSG music; imported module patterns and WAVs are
source assets, not editable PSG lanes.

| Operation | Meaning |
| --- | --- |
| `pcm_music(id)` | Start a tracker song from its beginning; loop by default. |
| `pcm_music(id, false)` | Play it once. Tracker-authored jumps/loops still follow Maxmod semantics. |
| `pcm_music(-1)` | Stop the imported song. |
| `sample(id, volume, priority)` | Play a WAV; volume is 0–1 (default 1), priority 0–255 (default 160). |
| `sample(-1)` | Stop all imported WAV effects. |

There are eight tracked WAV effect voices. When full, a new effect may replace
the lowest-priority active effect; lower-priority requests are discarded.
Songs and effects also share Maxmod's bounded 32-channel software mixer.
Dynamic invalid IDs and out-of-range values do nothing; known invalid literal
IDs fail with a source-positioned build diagnostic.

`music`, `sfx`, `gba_note` and `gba_stop` retain their existing PSG meanings.
You can use them alongside imported audio. Stopping an imported song or WAV
does not stop the PSG song, and stopping PSG does not stop the imported song.
The companion bank has its own versioned manifest; the existing PSG v2 schema
is preserved. This does not restore the superseded audio-v1 format.

## What it costs

For measured CPU cycles, both RAM banks, ROM, heap/GC controls and the
Butano comparison, read [What imported audio costs](imported-performance.md).
It includes audio-only examples, the 32-voice pacing fix and timing limits.

Maxmod reads instruments from ROM and mixes stereo output at approximately
15,768 Hz (its standard "16 kHz" rate), then DMA1/2 feed FIFO A/B. Mixing
needs CPU time and fixed RAM; PSG alone remains the inexpensive option.
The PCM sample clock uses Timer 1; PSG retains Timer 0; the existing frame
profiler retains Timers 2/3. Shared sound-register ownership preserves both.

The player is serviced at the LCD's 59.73 Hz while game updates remain at
approximately 30 FPS. A short VBlank handler resets DMA; mixing runs at scanline
0. A long service can extend into VBlank. Remembered LCD events and HALT
preserve the main loop's pacing across such interrupts; see the
[32-voice follow-up](../benchmarks/audio-pacing-2026-11-02/README.md).
Missing the audio service deadline can
also produce glitches in an overloaded game. Profile the
complete game with its busiest song and simultaneous effects.

The existing `stat(1)`/`stat(2)` work counters exclude deliberate pacing waits,
so they are not a total-CPU meter for background audio mixed during those
waits. Check actual update cadence and missed frames as well. Go cartridges
can read `Context.AudioStats().PCM.MaxMixCycles` for the maximum measured
single-refresh mixing cost; it uses the existing frame counter without
claiming another hardware timer.

The compiled bank is limited to 24 MiB, leaving room within the GBA's 32 MiB
cartridge limit for code and other assets. Full recordings consume much more
ROM than short instruments. There is no MP3/OGG decoder, runtime filesystem,
module-to-PSG conversion or claim of physical-hardware listening sign-off.
Unused imported assets do not link the Maxmod player into the cartridge.
