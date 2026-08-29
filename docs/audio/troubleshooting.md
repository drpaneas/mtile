# PSG troubleshooting

## No sound

Check controller initialization, `SOUNDCNT_X` master enable, terminal levels,
the channel's left/right route, valid sound/channel kind, and nonzero envelope
initial volume (or CH3 output level). Inspect rejected command/asset counters.

## Wrong pitch

Confirm the MIDI note is inside the channel range and uses the correct pulse
or CH3 period table. CH3's period also depends on whether Wave RAM contains 32
or 64 samples; changing that dimension without recompiling the sound is not a
pitch-neutral operation. CH4 is noise, not pitched. Legato must update
frequency without setting the trigger bit.

## CH3 waveform is wrong

Check 4-bit nibble packing, 32 versus 64 length, selected versus CPU-visible
bank, start bank, and output level. Wave RAM must be loaded while CH3 is
disabled through the safe opposite-bank sequence.

## Starting one channel stops another

Inspect whole-register writes to `SOUNDCNT_L/H/X`. Managed code must use the
central masked owner. Channel stop should clear only that channel's DAC/control
state and update master ownership only when no managed channel remains.

## Notes are late or drift

Check that Timer 0 runs at exactly 256 Hz, service is not being called from a
frame-rate loop instead, and the rational remainder survives part/loop
transitions. Inspect maximum service cycles and queue overflow.

## Editor and cartridge differ

Compare the compiled event trace and `Preview()` snapshot, then compare managed
register state with the direct-register truth ROM. Presets must save materialized
hardware fields and their bounded macro; preview must not substitute a software
instrument. Native PSG audition renders directly at 48 kHz with integrated
hardware edges and an AC-coupled high-pass stage. Sending a 16,384 Hz
point-sampled square/noise stream through the host resampler is a bug because it
creates audible alias tones that do not exist in GB Studio or on the hardware.
CH1 and CH2 use the exact hUGEDriver period table. Do not copy the same raw
tracker index directly to CH3 while calling it MIDI: the Wave channel clocks
every sample, so conventional MIDI pitch requires a one-octave table offset for
32 samples and a two-octave offset for 64 samples. Omitting that compensation
makes Wave playback physically one or two octaves too low.

If an old project sound was created before the corrected preset catalog, open
`PRESET LIBRARY` and select that preset again. Mtile creates/reuses the corrected
hardware definition; it does not silently rewrite a user-edited instrument.

## PCM/FIFO question

Audio v2 does not use Direct Sound, PCM samples, FIFO A/B, DMA1/2, or a software
mixer. Seeing those controls in the active editor or generated v2 path is a
product bug, not a configuration problem.

## Launchkey is not detected

The supported zero-config device is Novation Launchkey Mini 25 MK4. Connect its
MIDI and DAW ports before opening Music Lab. Mtile scans for hotplug, enables
DAW/feature control, and labels the channel rail `LAUNCHKEY CONNECTED`. A
disconnect immediately emits all-notes-off and safely closes an active take.
MIDI Learn and manual CC mapping are intentionally unnecessary.

If another application owns one of the ports, Mtile keeps any usable note
input active and reports the unavailable endpoint in the status bar. Close the
other MIDI application or reconnect the controller; no project configuration
needs to be changed.

## Launchkey notes sound correct but late

Mtile owns a low-latency host-audio context for editor monitoring. On macOS it
requests a 40 ms device queue and keeps a 20 ms live source buffer instead of
Oto's roughly 128 ms default AudioQueue. Those are two macOS device callbacks;
one-callback 8–10 ms configurations were rejected because
an editor frame or short GC pause could exhaust them and produce crackling.
Changing only a player's buffer does not reduce the device queue and can make
notes either late or chopped.

Mtile opens one Launchkey keyboard input and one DAW/control input. CoreMIDI
aliases of the same role are ignored so mirrored endpoints cannot retrigger
every note twice. Physical key-up uses a 5 ms monitor-only release ramp to
remove hard-cut clicks; the recorder still stores the exact MIDI note-off.

If latency remains unusually high, use the MacBook's built-in output or a
wired audio interface while testing. Bluetooth headphones add their own codec
and radio buffering after Mtile, outside the MIDI-to-PSG path. Cartridge PSG
timing is unaffected by any desktop output device.

## A recorded chord sounds different

Check the live allocation rail. POLY2 maps high/low to CH1/CH2. POLY3 maps
high/middle/low to CH1/CH2/CH3, after newest-note overflow selection. The raw
take stores those physical transitions. Try selecting the retained take and
pressing USE with quantize OFF to separate allocator behavior from grid
rounding.
