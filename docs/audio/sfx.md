# Four-channel sound effects

An SFX is a short typed clip targeting one hardware channel. It uses the same
sound definitions, timing, queue, and register operations as music.

In Music Lab, MUSIC/SFX switches the same composer between the current song
part and the standalone SFX library. An effect card displays its stable
`sfx(id)`, name, hardware channel, and event count. The shared piano roll,
CH4 drum grid, note duration, instrument selector, simple/advanced hardware
inspector, note audition, transport, mute/solo, rename, undo, and save behavior
all work without opening another editor.

SFX and timed direct notes share the current part's effective tempo: its
override when present, otherwise the song base tempo. When no song is running
they use the fixed 120 BPM default. Up to four SFX clips may run at once—one
per hardware channel—and a new equal/higher-priority SFX replaces only the
existing SFX on its own channel.

- CH1 effects can use pulse sweep.
- CH2 effects use pulse/envelope.
- CH3 effects use a custom Wave RAM tone.
- CH4 effects use noise and are the default for percussion.

`sfx(id, priority)` requests ownership of the clip's channel. It may replace a
song note only when its priority is at least the song event priority. Direct
`gba_note` calls may replace any owner. The next song part reclaims lanes it
uses. `sfx(-1)` stops only SFX-owned channels.

The queue is fixed and overflow is observable. Invalid IDs, channel-kind
mismatches, and out-of-range notes fail closed. There are no PICO waveforms,
filters, speed units, PCM samples, or FIFO bus effects.
