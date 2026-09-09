# Mtile documentation

## Make a game

| Guide | What it covers |
| --- | --- |
| [Getting started](getting-started.md) | Install, build a ROM, and use the preview |
| [Your first game](first-game.md) | Coin Quest: a playable template and four working lessons |
| [Examples](../examples/README.md) | Small projects to read, run, and modify |
| [IDE](ide.md) | Sprite, map, audio, and source editing |
| [Reference](reference.md) | CLI flags, project files, Lua, input, and builtins |
| [Graphics model](graphics-model.md) | Map planes, sprites, camera, and draw order |
| [Renderer compatibility](render-modes.md) | Exact Mode 0, Mode 4, and AUTO restrictions |
| [Audio](audio/README.md) | Four-channel hardware sounds and music |
| [Performance](performance.md) | Cycle budgets, memory limits, and measured costs |

## Work on Mtile

Build or edit the [static learning website](../website/README.md).

Start with [Contributing](../CONTRIBUTING.md) for the code map and checks.
The [map format](map-assets.md), [audio schema](audio/schema-v2.md),
[renderer design](render-mode-plan.md), and [audio architecture](audio/architecture.md)
describe the contracts behind the implementation.

[Release packaging](../scripts/release/README.md),
[launch checklist](launch-checklist.md), and
[third-party notices](third-party/README.md) cover distribution.

## Benchmarks and design history

The [performance guide](performance.md) links measured results and their raw
evidence, including the [compiler/runtime optimization pass](celeste-mode4-performance-2026-09-08.md)
and [dirty-region experiments](celeste-dirty-regions-2026-09-08.md).
[Renderer benchmarks](benchmarks/render-modes/README.md) document the accepted
compatibility/performance gates.

Date-stamped reports, files under `design/`, and plans record experiments or
proposals; they do not define supported features. In particular, private
renderer experiments do not widen the public
[compatibility contract](render-modes.md).
Audio v1 is unsupported; its software mixer and tooling have been removed.
Use the guides above for current game development.
