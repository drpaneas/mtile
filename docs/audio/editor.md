# Music Lab editor

## One screen, one document

Audio Studio contains one Music Lab. There are no CREATE, SONG, GBA, Sample
Library, or Audio Profile pages. The small MUSIC/SFX switch changes what the
same piano roll is editing; hardware controls never move to another screen.
Composition, live performance, recording, and arrangement remain visible
together.

```text
song title + tempo                     ARM  REC  PLAY  STOP
----------------------------------------------------------------
Game Boy channels | live allocation + active part + sound controls
                  | piano roll or CH4 drum grid
                  | song order + retained takes + loop/end
```

The four permanent hardware channels keep one color everywhere:

| Channel | Friendly label | Color | Musical use |
| --- | --- | --- | --- |
| CH1 | Pulse + Sweep | yellow | lead / melody |
| CH2 | Pulse | green | harmony / bass |
| CH3 | Wave | blue | custom tone / bass |
| CH4 | Noise | red | drums / effects |

There is no sample library, FIFO/bus view, PCM track, generic voice count, or
PICO instrument list.

## Write a part

Select CH1, CH2, or CH3 and click the piano roll to place a note. Selecting CH4
turns the same editor into eight labeled hardware-noise rows: kick, snare,
closed/open hat, two toms, click, and FX. Right-click erases. Delete/Backspace
removes the selected note. The matching piano key and moving playback cursor
use the selected channel's color.

The authoring clock is 96 PPQN. Straight grid guides reach 1/64 (6 ticks), but
event positions and durations retain their exact tick values. `NOTE LENGTH`
includes straight, triplet (`T`), and dotted (`.`) choices from 1/64 through
four beats. `SNAP` independently selects Off, a straight division, or a
quarter/eighth/sixteenth triplet; `SWING` delays alternating straight-grid
positions from 50% through 66%. New projects default to a comfortable 1/32
note and 1/64 snap.

Click and drag horizontally on a new or selected note to set its duration.
The block grows to show the actual sustained duration and cannot overlap the
following note. Dragging across pitch rows keeps the pencil-style continuous
melody workflow. Shift-drag selects a rectangular group; Shift+Arrow moves it,
Cmd/Ctrl+C/V copies and pastes it with exact relative timing, Delete/Backspace
removes it, and Escape clears the selection. MIDI note-on/note-off timing is
stored as the note's real duration and uses the same rendering and playback
path.

Each pitched hardware channel exposes only notes that its real frequency
register can represent; invalid piano keys are disabled and octave navigation
skips empty ranges. Each part may contain 1–128 bars and uses an explicit time
signature. The friendly selector includes common simple, compound, and odd
meters from 2/4 through 12/8; Shift-click changes a custom numerator and
right-click changes its denominator among 1, 2, 4, 8, 16, and 32. The BAR
control pages through the same continuous clip; it does not create disconnected
patterns. The selected pitched note exposes `RETRIGGER` or `TIE / LEGATO`. A
tie changes only the hardware period when a compatible previous note is still
sounding, so its phase/envelope is not restarted. CH4 always retriggers.

`REC` is a continuous transport, independent of the song's playback loop. The
playhead and unfinished notes appear in the piano roll while they are being
performed. Recording continues across measure boundaries until `STOP`; the
active part then grows to the next whole bar and the single retained raw take
is committed for non-destructive quantization.

The name above the grid is the selected channel clip and can be edited in
place. The hardware inspector changes with the channel:

- CH1: duty, sweep, envelope, length, and route;
- CH2: duty, envelope, length, and route;
- CH3: Wave RAM drawer, bank, level, length, and route; and
- CH4: width, divisor, shift, envelope, length, and route.

`PRESET LIBRARY` opens a channel-specific dropdown based on GB Studio's
MIT-licensed factory instruments: 14 CH1 pulse/sweep sounds, 10 CH2 pulse
sounds, 14 CH3 Wave RAM tones, and 14 CH4 noise/percussion sounds. Pointing at
an entry highlights it; click it, or use the arrow keys and Enter, to select
and immediately audition it. The chosen preset becomes the reusable instrument
for new notes on that physical channel. Selecting the same unchanged preset
again reuses its stable sound ID instead of consuming another ROM slot.

GB Studio instruments can contain multi-step subpatterns. Music Lab preserves
the factory tables as bounded hardware macros: row zero is applied with the
note trigger, later rows advance at hUGEDriver's 59.7275 Hz cadence, CH1–CH3
rows update period without retriggering, and CH4 rows rewrite the exact NR43
divisor/shift pair relative to the note being played. Preview, saved assets,
generated cartridge tables, and the runtime controller all consume the same
macro rather than reducing a preset to its first row.

Every control writes a real hardware-valid field. CH3 Wave RAM may be drawn
directly. The editor never invents ADSR, filters, reverb, arbitrary oscillators,
or software voices.

The inspector starts in SIMPLE mode with only the musical controls most people
need. ADV reveals the complete legal register-level controls. Instruments are
reusable per channel, can be renamed or duplicated, and use copy-on-write when
one note is changed so an edit never silently alters unrelated notes.
ADV also reveals the project-wide hardware output controls in the live rail:
left terminal level, right terminal level, and the 25/50/100% PSG mix ratio.
These write SOUNDCNT_L/H values; they are not software volume effects.

## Arrange the song without changing pages

The order strip under the composer shows named parts from left to right. Each
part card contains its CH1–CH4 clips. Click a part header to select or rename
it; click a channel lane to edit or create that clip above.

- `+ NEW` adds an independent part.
- `+ REUSE` inserts another reference to the selected part; edits intentionally
  update every occurrence.
- Cmd/Ctrl+D duplicates the selected part.
- Shift+Left/Right reorders it.
- Shift+Delete/Backspace deletes it.
- Delete/Backspace on a selected arrangement lane removes only that clip.
- Cmd/Ctrl+C/V copies and pastes clips.
- LOOP/END chooses whether the song repeats.

The SONG controls in the header create, select, rename, and delete independent
songs in the same project. Stable song IDs remain the values passed to
`music(id)`.

The tempo value is labeled `SONG` while the selected part inherits the song
tempo and `PART` while it has an override. Click the value to toggle between
those scopes; part changes apply exactly at part boundaries. The minus/plus
buttons edit the currently displayed scope from 20 through 400 BPM.

The editor does not expose next pointers or require an abstract pattern before
the first note.

## Make a sound effect in the same editor

Select `SFX`, choose CH1–CH4, and draw a short clip in the same piano roll or
noise grid. The lower strip becomes the SFX library and shows each stable
runtime call, for example `sfx(7)`. `+` creates an effect, `X` deletes the
selected standalone effect, and the arrows select the previous/next effect.
Changing hardware channel creates a correctly typed effect for that channel.

Space and PLAY audition the effect with its real route and project PSG level.
Mute/solo remain monitor-only. A standalone SFX is never inserted into the
song order and a song clip is never accidentally deletable from the SFX list.

## Play and perform

Space toggles song play/stop. The transport buttons do the same; STOP also
finishes an active recording safely. Playback moves the piano-roll cursor and
follows the current part.

ARM enables immediate live preview without recording. Choose one performance
mode:

- MONO: the selected CH1, CH2, or CH3;
- POLY2: highest note on CH1 and lowest on CH2;
- POLY3: high/middle/low on CH1/CH2/CH3; or
- DRUMS: CH4 noise presets.

Computer keys and MIDI enter the same `PerformanceEvent` path. When capacity
is exceeded, the newest held notes win; an older held note returns when a newer
one is released. The allocation rail always shows the real result, such as
`G4 -> CH1 | E4 -> CH2 | C4 -> CH3`.

Live keyboard/MIDI monitoring uses the selected reusable instrument and the
same route, global L/R levels, PSG mix ratio, envelope, sweep, Wave RAM, noise
clock, and optional hardware length cutoff as saved cartridge playback.

## Record and keep takes

REC is independent of ARM. Recording stores raw timestamps plus the allocator's
exact physical channel starts, stops, reassignments, and parameter changes.
The authoring controls are:

- quantize Off, 1/4, 1/8, 1/16 (default), 1/32, 1/64, or quarter/eighth/
  sixteenth triplet;
- swing 50%, 54%, 58%, 62%, or 66% for straight quantization;
- overdub or replace;
- metronome on/off; and
- count-in Off, one bar, or two bars.

Loop recording retains Take 1..N. The take selector in the order strip chooses
a take; USE re-materializes it with the current quantize and overdub settings.
Raw timing remains unchanged, so repeated quantization never compounds error.
Takes are editor metadata and are stripped from cartridge data.

## Launchkey Mini 25 MK4

The supported controller is zero-config. Music Lab detects its MIDI and DAW
ports, enables DAW/feature control, configures relative encoders and pad mode,
and updates LEDs/OLED labels. Keys perform notes; eight pads play the CH4 drum
set; the remaining pads select modes and toggle click, count-in, loop, and
overdub. Transport controls play, stop, and record. Encoders change real PSG
parameters only.

In POLY3, linked duty targets CH1–CH3, envelope controls target CH1–CH2,
sweep targets CH1, and wave level targets CH3. POLY2 leaves encoder 8 reserved.
The recorder does not create automation events for incompatible channels.

Parameter timing is explicit and identical in preview and recorded playback:

| Semantics | Current controls | Result while a note is held |
| --- | --- | --- |
| `RETRIGGER` | duty, CH3 level, CH4 width/divisor/shift | restart that physical channel at the automation tick with the new hardware state |
| `NEXT_NOTE` | volume/envelope and CH1 sweep | leave the held note untouched; use the value on the next trigger |
| `IMMEDIATE` | reserved for a future safe live-register control | change state without restarting phase/envelope |

This avoids pretending that rewriting a Game Boy envelope or sweep register in
the middle of a note is a stable cross-hardware operation.

Velocity and aftertouch are ignored because the hardware channels do not have
those dimensions. Disconnect emits all-notes-off, finalizes the current take,
and restores controller mode on shutdown. MIDI data never becomes a cartridge
asset.

On macOS this uses the four CoreMIDI endpoints exposed by the controller. It
does not require a driver, Novation Components preset, MIDI Learn, or manual CC
mapping. Hot-plug scanning starts when Music Lab opens. A fully ready device is
shown by its clean product name; a missing or busy MIDI/DAW port is reported in
the status bar instead of failing silently.

## First-use acceptance

A new project must let a first-time user select CH1, click notes, press Space,
hear playback, add CH4 hits, name the part, loop it, save, and build without a
tutorial or unexplained audio-engine term. A musician may instead connect the
Launchkey, ARM, perform, REC, choose a take, and arrange it in the same window.

Opening Music Lab in a graphics-only older project initializes empty v2 PSG
assets. A project that contains legacy PICO/PCM audio is left untouched and
receives an explicit conversion-required message.

## Desktop sound preview

The GBA hardware remains the cartridge sound source, but the desktop editor is
not silent. Music Lab simulates CH1–CH4 in software using the same compiled
notes, periods, duty cycles, Wave RAM, envelopes, sweep, and CH4 LFSR controls.
Play and Space send this signal to the host audio device and move the playhead.
An empty arrangement reports that there is nothing to play instead of opening
an all-zero stream.

Click audition and armed keyboard/MIDI monitoring use the same pure-Go PSG
renderer. Its channel timers run from the AGB clock and its hardware edges are
band-limited to the 48 kHz monitor rate. Retriggering a pulse channel preserves
the hardware waveform phase, so rapid Launchkey notes do not introduce a new
DC jump or a click at every note-on.
Physical MIDI note-off receives a 5 ms host-monitor de-click ramp. This does
not lengthen the recorded note or change the PSG command emitted by a built
cartridge.

For an independently playable check, render exactly that pre-device signal:

```text
mtile render-audio -o music-preview.wav project-dir
```

The command fails on an actually silent song and reports duration, sample rate,
peak, and RMS for a successful render. Hardware conformance remains a separate
mGBA/physical-GBA gate; WAV preview is an authoring monitor, not a substitute
for the final hardware capture.
