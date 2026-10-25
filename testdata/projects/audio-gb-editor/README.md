# Music Lab round trip

This small project is deliberately ordinary editor output. Open it, rename the
clip, move notes, save, and build it to verify the complete Music Lab → strict
JSON v2 → generated Go → TinyGo ROM path.

```sh
go run ./cmd/mtile edit testdata/projects/audio-gb-editor
go run ./cmd/mtile build testdata/projects/audio-gb-editor
go run ./cmd/mtile render-audio testdata/projects/audio-gb-editor
```

`music-preview.wav` is an independently playable capture of the exact software
PSG monitor signal used by Music Lab.
