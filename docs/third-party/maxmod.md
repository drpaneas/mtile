# Maxmod

mTile uses the BlocksDS fork of Maxmod for optional imported Direct Sound
music and effects. The ISC-licensed source is vendored from Butano revision
`04374b034c76b82bfdc3aa6c38d652e5e7778a68` under `runtime/maxmod/`.

The [integration notes](../../runtime/maxmod/MTILE.md) describe the Timer 1,
shared sound-register, static-buffer and LLVM-assembler adaptations. The
[upstream ISC license](../../runtime/maxmod/COPYING) is retained in source
and included in release bundles as `licenses/MAXMOD-ISC.txt`.

Upstream: [BlocksDS Maxmod](https://github.com/blocksds/maxmod).
The external asset converter is [mmutil](https://github.com/blocksds/mmutil).
No third-party song or sound effect is included in the imported-audio example.
