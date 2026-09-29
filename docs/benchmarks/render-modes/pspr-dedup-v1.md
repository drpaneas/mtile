# OBJ pixel deduplication evidence

The `deduplicated-bank` fixture stores five painted logical tiles in a
65,536-cell sheet. Tiles 0, 1 and 65,535 share one exact pixel pattern;
tiles 2 and 256 share a second. Separate poses address every logical ID and
exercise authored flips, whole-pose flips, animation and movement.

The native test verifies that both generated renderers contain **128 bytes of
OBJ pixel data**, representing two distinct 64-byte cells. The previous storage
would contain 320 bytes. This saves **192 pixel bytes** while preserving the
logical IDs and their separate pose references; the logical-ID index remains.

Captured on 2026-09-24 with TinyGo 0.42.0 and the repository's headless mGBA
player: **120 matching game frames and all 240 LCD scans have zero differing
pixels**. Mode 0's maximum visible commit is 4,324 cycles. Median frame cost is
16,437 cycles in Mode 0 versus 183,027 in Mode 4.

The original fighter and 1,000-frame large-bank fixtures were also rerun after
deduplication: another 240 game frames and 480 LCD scans matched exactly. Their
original evidence files remain unchanged.

Reproduce with a fresh output directory:

```sh
MTILE_PSPR_OUT=/tmp/mtile-pspr-dedup \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestPSprStreamTargetDiagnostic/(deduplicated-bank|fighters|large-bank)$' \
  -count=1 -v
```

The adjacent `pspr-stream-deduplicated-bank-v1.json` preserves ROM hashes,
toolchain details, memory measurements, timing and parity results.
