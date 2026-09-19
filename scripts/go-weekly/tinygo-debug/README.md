# TinyGo mGBA panic experiment

Run from the repository root on macOS with TinyGo 0.42.0, the repository's Go
toolchain, and an already-built `build/player/mtile-player`:

```sh
python3 scripts/go-weekly/tinygo-debug/run.py \
  --out /tmp/mtile-tinygo-debug-results
```

The runner uses macOS `cp -cR` to copy the installed TinyGo resources into a
fresh temporary directory, patches only that copy, and bypasses Homebrew's
`bin/tinygo` launcher so its hard-coded `TINYGOROOT` cannot override the copy.
The same real compiler executable builds every variant. It uses the existing
Go/TinyGo caches; it never installs a toolchain or changes mTile's default build.
Temporary ROMs and generated workspaces remain available for inspection.

`--work` selects a fresh work directory; `--tinygo` selects the installed
launcher; `--samples` sets a positive number of logical preview frames (300 by
default, after twenty warmup updates). The runner fails if its expected source
shape or any experiment assertion changes.

The matrix checks:

- Both explicit `panic` and volatile-index bounds panic, with mTile's target
  and startup. Unpatched `mgbadebug` must fail with `zeroSizeAllocPtr`; patched
  builds must deliver the real panic through native-player stderr. These
  fixtures do not import mTile or call `rt.DebugLog`.
- Zero compiler-reported allocations in both diagnostic programs.
- Testio Mode 0 and Mode 4 release ROM hashes, generated source, memory totals,
  and first four native-player frames, before and after patching the resources
  without enabling `mgbadebug`.
- Ordinary mTile previews versus previews additionally enabling `mgbadebug`:
  synchronized RGBA hashes, watches, heap/GC state, missed frames, memory,
  and emulated GBA frame-work cycles.
- Hashes of the installed runtime and target files remain unchanged.

Each command saves a log and an exit status in `commands.json`. The final
`results.json` includes every preview sample; `metadata.json` records toolchain,
player and product-source hashes. The generated `upstream.patch` is the complete
TinyGo change. A failed assertion exits nonzero; per-command logs remain even if
the final results file has not been written.

See the [measured result](../../../docs/benchmarks/go-weekly/tinygo-debug/README.md).
