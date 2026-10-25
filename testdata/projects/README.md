# Engineering cartridge fixtures

These editable projects support CI, compiler benchmarks, and renderer parity
checks. They are not introductory games. Run build commands from the repository
root, for example `go run ./cmd/mtile build testdata/projects/testio`.

| Fixture | Purpose |
| --- | --- |
| color | Implicit-print-cursor smoke test |
| platformer | Historical PGO corpus input; only a sprite crop, not a platformer |
| testio | Static-map renderer parity and performance baseline |
| audio-gb-editor | Music Lab save/load/build round trip |
| physics-stress | Numeric lowering benchmark |
| function-physics-stress | Function signature specialization benchmark |
| multi-return-physics-stress | Multi-return specialization benchmark |
| noalloc-call-stress | Non-allocating call/root-wrapper benchmark |
| parallax-rectangles | Grouped-scroll and Mode 0 transfer workload |

Sources and assets were preserved during relocation. Existing raw benchmark
JSON records retain their original paths and measurements; current source
commands and test paths use this directory. PGO uses a separately frozen corpus
selected by `MTILE_PGO_INPUT_ROOT`, whose flat project names remain unchanged.
