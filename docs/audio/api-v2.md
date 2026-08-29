# Four-channel Game Boy audio API v2

## Beginner Lua API

```text
sfx(id [, priority])
music(id)
```

- `sfx(id)` starts a compiled one-channel effect clip.
- `sfx(-1)` stops gameplay-owned SFX without stopping song/direct ownership.
- `music(id)` starts a song at its first part.
- `music(-1)` stops song-owned notes without stopping gameplay/direct notes.
- Priority is 0–255.
- Effects use the current part's effective tempo (part override or song base),
  or 120 BPM when no song is running.
- At most one effect clip runs per hardware channel; different channels are
  independent.

Version-1 channel/offset/length and music-fade arguments are rejected by v2.

## Native Lua API

```text
gba_note(channel, note, sound_id [, duration_ticks])
gba_stop(channel)
```

- Channel is 1–4.
- The sound kind must match that channel.
- CH1–CH3 use a MIDI-style note accepted by the channel's frequency range;
  CH3 validation and period selection account for 32- versus 64-sample waves.
- Fixed CH4 sounds trigger their saved noise configuration. Table-driven
  presets use the note as the base for hUGEDriver-compatible relative
  polynomial rows.
- Duration zero sustains; otherwise it is 1–65535 ticks at 96 PPQN.
- Timed direct notes use the current part's effective tempo, or 120 BPM without
  a song.
- Calls enter the same fixed command queue as compiled music.

There is no `sample`, `gba_sample`, `gba_stop_bus`, FIFO, PCM, pan, or mixer
voice API in Audio v2.

## Go API

```text
Controller.Init(ProfilePSG)
Controller.ConfigureOutput(leftLevel, rightLevel, ratio)
Controller.Bind(Library)
Controller.Start()
Controller.Stop()
Controller.NoteOn(channel, note, soundID, durationTicks)
Controller.StopChannel(channel)
Controller.PlaySFX(id, priority)
Controller.StopSFX()
Controller.PlaySong(id)
Controller.StopSong()
Controller.Service() // host/editor manual clock only
Controller.Stats()
Controller.Preview()
```

`Start` claims Timer 0 and services the controller from the exact 256 Hz IRQ.
`Stop` releases Timer 0 and silences CH1–CH4. `Service` cannot double-advance a
running target controller.

Invalid literal IDs fail during compilation with source position. Invalid
dynamic values fail closed and increment bounded diagnostics.

## Runtime telemetry

`stat(80..92)` exposes bounded native-audio diagnostics to Lua:

| ID | Value |
| ---: | --- |
| 80 | scheduler service calls |
| 81 | applied hardware events |
| 82 | active PSG channels |
| 83 | channel ownership replacements |
| 84 | priority rejections |
| 85 | rejected commands |
| 86 | command-queue overflows |
| 87 | maximum service cycles |
| 88 | absolute 96-PPQN song tick |
| 89 | completed songs |
| 90 | minimum measured service cycles |
| 91 | mean measured service cycles |
| 92 | measured services |

Missing audio returns zero. Cumulative counters saturate at 32,767 for the
16:16 Lua number representation.

## Ownership

1. Direct native calls may replace any owner on their channel.
2. SFX may replace a song note only at equal or higher priority.
3. Song events do not replace SFX/direct ownership in the middle of a part.
4. A new part reclaims every channel assigned by that part.
5. Stopping one owner class preserves the others.

Expert raw register access remains available through `runtime/hw`, but mixing
managed and unmanaged PSG writes is outside the supported contract.
