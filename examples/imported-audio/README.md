# Imported audio

An original synthetic MOD melody and WAV bell demonstrate the optional
Maxmod Direct Sound path alongside the existing PSG controller. The display
is deliberately a plain background so this example works in both renderers.

- A plays the imported WAV bell.
- B plays a CH1 PSG ping over the imported music.
- L stops the imported song; R restarts it.

From the folder containing the [downloaded examples](../README.md#run-an-example), run:

```sh
mtile edit examples/imported-audio
```

Use Build + Reload, then click the preview to send game controls. Importing
new files requires mmutil; building this already imported example does not.
To replace the included files while retaining their IDs:

```sh
mtile import-audio -project examples/imported-audio theme.mod bell.wav
```

See [the import guide](../../docs/audio/samples.md) for the supported
WAV/MOD/XM/S3M/IT formats, IDs, source assets, ROM/CPU tradeoffs and limits.
The included source sounds and melody are original procedural fixtures,
licensed under the repository MIT license. No third-party music is included.
