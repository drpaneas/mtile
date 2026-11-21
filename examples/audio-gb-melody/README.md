# Game Boy hardware melody

A compact Audio v2 cartridge that loops one section using all four physical
Game Boy sound channels in the GBA: CH1 pulse+sweep, CH2 pulse, CH3 Wave RAM,
and CH4 noise. No Direct Sound, FIFO, DMA audio, PCM samples, or software mixer
is linked.

From the folder containing the [downloaded examples](../README.md#run-an-example), run:

```sh
mtile edit examples/audio-gb-melody
```

In the cartridge, **A plays/restarts** the song and **B stops** it. In the
editor preview, click the display first; Z is A and X is B.
