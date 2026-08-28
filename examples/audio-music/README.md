# audio-music

> **Legacy audio v1 fixture.** This project remains runnable while native-GBA
> audio v2 is developed, but its four logical software tracks are not the v2
> GBA channel model. See the
> [archived v1 documentation](../../docs/audio/legacy/v1/).

The first fixed-memory PICO-style music example. `music(0)` starts an intro,
then the explicit pattern links form a two-pattern loop:

```text
intro (0) -> phrase-a (1) -> phrase-b (2) -+
                  ^                         |
                  +-------------------------+
```

Every pattern assigns an SFX ID or `-1` to each of four logical channels. Its
duration is the longest complete assigned SFX; SFX-local loops do not block a
music transition. Build it from the repository root:

```sh
go run ./cmd/mtile build examples/audio-music/
```

Or author and preview it in the native Music mode:

```sh
go run ./cmd/mtile edit examples/audio-music/
```

Select the **Music** tab or press `4`. Space controls transport; M and S mute
or solo the selected channel for preview without changing the saved pattern.

Controls in mGBA:

- A: restart at pattern 0 with a 500 ms fade-in
- B: fade music-owned channels out over 500 ms
- Left: manually interrupt logical channel 1; music reclaims it at the next pattern
- Right: play an automatically assigned SFX, normally using silent channel 3

The v1 source is `assets/audio/music.json`. Pattern IDs are stable `music()`
entry points, `next` selects the transition, a backward link loops, and
`next: -1` stops. The editor follows these links in its signal view and uses
the target music sequencer for preview. `music(n, fade_ms)` fades a new entry
in, while `music(-1, fade_ms)` fades music-owned channels out without changing
manually triggered SFX.
