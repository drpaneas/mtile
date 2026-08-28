# Audio settings

Audio v2 has one profile: `gameboy-psg`.

```json
{
  "version": 2,
  "profile": "gameboy-psg",
  "leftLevel": 7,
  "rightLevel": 7,
  "psgRatio": 100
}
```

- `leftLevel` and `rightLevel`: 0–7 hardware terminal levels.
- `psgRatio`: 25, 50, or 100 percent.

The Music Lab keeps these out of the beginner surface. Select `ADV` in the
hardware inspector to edit `L`, `R`, and `MIX` directly while hearing the same
settings in desktop preview. Per-sound left/right/both routing remains beside
the channel's other advanced controls.

Channel routing belongs to each sound. The scheduler rate is fixed at 256 Hz
and is not a user setting. There are no output-rate, PCM voice, sample, stereo
bus, FIFO, DMA, or software mixer options.

The editor shows estimated compact event/Wave RAM ROM cost and measured maximum
scheduler cycles. It does not ask a beginner to choose an audio engine profile.
