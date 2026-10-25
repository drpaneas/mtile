# Mtile documentation

## Start here

You do not need to read this documentation from beginning to end. Start with
one working game, make one change, and look up features as you need them.
If your tools are not installed, complete [setup](getting-started.md#requirements)
first.

1. **[Play Coin Quest](first-game.md).** Create the
   starter, build it, and collect five coins. You have a working game and know
   how to run it.
2. **[Make one change](first-game.md#your-first-change).** Change `speed = 2`
   to `speed = 3`, rebuild, and feel the difference. You have completed the
   edit → build → play loop.
3. **[Try one pose, then a clip](../examples/moth-study/README.md).** Moth Study
   starts with one still picture, then adds playback and facing in separate
   lessons. Or [choose a different feature](small-games.md) for your next game.

Stop after any step with a working project. Come back for the next feature
when you want it; the four [Coin Quest lessons](first-game.md#learn-one-piece-at-a-time)
are there when you want to understand the code.

## Find your next task

| I want to… | Open… |
| --- | --- |
| Install tools or fix the preview | [Getting started](getting-started.md) |
| Learn movement, pickups, sound, and winning one step at a time | [Your first game](first-game.md) |
| Learn one new feature in a small working game | [Choose one small game](small-games.md) |
| Find a button or follow an artwork workflow | [Illustrated IDE guide](ide.md) |
| Draw a pose or play a repeating animation | [Moth Study](../examples/moth-study/README.md) |
| Start and finish a one-shot animation | [Moon Door](../examples/moon-door/README.md) |
| Add a map, following camera, or parallax | [Tidewalk](../examples/tidewalk/README.md) |
| Switch between rooms and choose the entry position | [Lantern Rooms](../examples/lantern-rooms/README.md) |
| Place animated decorations by painting them in the map | [Tile links](tile-links.md) |
| Make a sound or song | [Audio](audio/README.md) |

## Look up details when you need them

These are references, not prerequisites for your first game.

| Reference | Use it for |
| --- | --- |
| [Lua API walkthrough](lua-api.md) | Builtin signatures, examples, and usage notes |
| [Reference](reference.md) | CLI flags, project files, Lua, input, and builtins |
| [Renderer compatibility](render-modes.md) | Exact Mode 0, Mode 4, and AUTO restrictions |
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
