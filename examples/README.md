# Example projects

From the repository root, replace `hello` with a project below:

```sh
go run ./cmd/mtile edit examples/hello
go run ./cmd/mtile build examples/hello
```

The resulting ROM is written inside the project directory. See
[getting started](../docs/getting-started.md) for the required toolchain.

## Start here

For a guided first game, follow [Coin Quest](../docs/first-game.md).
`mtile new -template coin-quest mygame` creates an editable game, original art
and sound, and four lesson checkpoints. The template is also included in the CLI
binary, so it works without the repository's examples directory.

| Project | Focus |
| --- | --- |
| [hello](hello) | D-pad input, a moving sprite, and a mapped background |
| [color](color) | Minimal text/drawing project produced by `mtile new` |
| [multi-file](multi-file) | Source organization and explicit `require()` |
| [background-layers](background-layers) | Four independent planes and scrolling parallax |
| [coin-block](coin-block) | An animated map cell and a state transition |
| [audio-gb-melody](audio-gb-melody) | A compact four-channel hardware melody |
| [audio-gb-song](audio-gb-song) | A multi-section song using all four channels |
| [audio-gb-editor](audio-gb-editor) | Music Lab save/build/run workflow |

## Explore further

[mapdemo](mapdemo) exercises a large streaming map.
[pong](pong) is a larger game example. Their READMEs describe controls and
project-specific assumptions; check renderer diagnostics and measured costs
when adapting them.

Projects such as [testio](testio), [physics-stress](physics-stress), and
[noalloc-call-stress](noalloc-call-stress) are diagnostics or stress workloads.
They are useful for contributors, but are not introductory game templates.

## Research projects

Research ports and locally ignored projects are not required to build Mtile
or run its introductory examples. Their presence in a developer checkout does
not make them part of the supported public example set.
