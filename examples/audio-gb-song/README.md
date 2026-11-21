# Native Game Boy song

A two-section Audio v2 song using CH1, CH2, CH3, and CH4 together. The intro
plays once and the Adventure section loops. Every sound is an exact Game Boy
hardware configuration; the cartridge has no software sample mixer.

From the folder containing the [downloaded examples](../README.md#run-an-example), run:

```sh
mtile edit examples/audio-gb-song
mtile build examples/audio-gb-song
mtile render-audio examples/audio-gb-song
```

The checked-in `music-preview.wav` is the same PSG simulation that feeds the
Music Lab Play button. It contains all four channels and is useful for checking
host audio independently of the editor window.

In the cartridge, **A plays/restarts** the song and **B stops** it. In the
editor preview, click the display first; Z is A and X is B.
