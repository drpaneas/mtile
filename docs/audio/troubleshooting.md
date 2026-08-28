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
hardware fields; preview must not substitute a software instrument.

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

## A recorded chord sounds different

Check the live allocation rail. POLY2 maps high/low to CH1/CH2. POLY3 maps
high/middle/low to CH1/CH2/CH3, after newest-note overflow selection. The raw
take stores those physical transitions. Try selecting the retained take and
pressing USE with quantize OFF to separate allocator behavior from grid
rounding.
