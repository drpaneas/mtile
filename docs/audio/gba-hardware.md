# Game Boy sound hardware on the GBA

The first native Mtile audio release uses the four legacy Game Boy PSG
channels built into the Game Boy Advance.

## CH1 — pulse with sweep

- Four duty cycles: 12.5%, 25%, 50%, 75%.
- 11-bit frequency and explicit trigger.
- Optional hardware length counter.
- Initial volume 0–15 and hardware envelope pace/direction.
- Frequency sweep pace/direction/shift.
- Independent left/right routing.

CH1 is the natural lead/effect channel because it is the only swept pulse.

## CH2 — pulse

CH2 has the same duty, frequency, length, envelope, trigger, and routing
controls as CH1 but no frequency sweep. It suits harmony, bass, and a second
melody.

## CH3 — programmable wave

- One 32- or 64-sample waveform in 4-bit Wave RAM.
- Two hardware banks and selectable starting bank.
- Mute, 25%, 50%, 75%, or 100% output level.
- Frequency, length, trigger, and left/right routing.

CH3's frequency register controls the Wave RAM **sample clock**, not the
finished waveform cycle directly. A 64-sample waveform therefore uses a
different period from a 32-sample waveform for the same musical note. Mtile
accounts for the selected dimension in the compiler, cartridge runtime,
desktop preview, live MIDI preview, and static `gba_note()` validation.

The selected playback bank and CPU-visible bank are opposite. Mtile disables
CH3, selects the opposite bank, writes all Wave RAM words, then establishes the
final mode/bank and triggers. CH3 has no hardware volume envelope.

## CH4 — noise

- 15-bit or 7-bit LFSR.
- Divisor and clock shift.
- Length, initial volume, hardware envelope, trigger, and routing.

CH4 is used for drums and noise effects. It is not a pitched piano channel;
named kick/snare/hat presets are real combinations of these controls.

## Shared mixer

`SOUNDCNT_L` selects CH1–CH4 routes and left/right terminal levels 0–7.
`SOUNDCNT_H` selects the PSG output ratio. `SOUNDCNT_X` controls the master and
reports channel activity. Shared writes are masked through one owner.

On real GBA hardware, `SOUNDCNT_X.7` must be enabled before writing
`SOUND1CNT_*` through `SOUNDCNT_L`; while it is clear that PSG register window
is inaccessible and reset to zero. Mtile therefore claims the PSG master
first, commits the complete route/level shadow second, and only then writes a
channel's envelope/frequency/trigger. This ordering is covered by a host MMIO
trace regression because permissive emulators may otherwise hide the bug.

## CPU behavior

Once triggered, each channel generates its waveform, envelope, sweep, and
length behavior in hardware. The ARM7 CPU does not produce individual audio
samples. It only writes registers for scheduled events at 256 Hz.

## Not used in Audio v2

The GBA also has Direct Sound FIFO A/B for PCM. Audio v2 deliberately does not
use them, DMA1/2, PCM buffers, or a software mixer. Earlier reference ROMs
remain useful hardware research but are outside the product.

References:

- [GBATEK GBA sound channel 3](https://rust-console.github.io/gbatek-gbaonly/#gba-sound-channel-3-wave-output)
- [GBATEK GBA sound control](https://mgba-emu.github.io/gbatek/)
