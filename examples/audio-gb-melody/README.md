# Game Boy hardware melody

A compact Audio v2 cartridge that loops one section using all four physical
Game Boy sound channels in the GBA: CH1 pulse+sweep, CH2 pulse, CH3 Wave RAM,
and CH4 noise. No Direct Sound, FIFO, DMA audio, PCM samples, or software mixer
is linked.

Open it in Music Lab with:

```sh
go run ./cmd/mtile edit examples/audio-gb-melody
```
