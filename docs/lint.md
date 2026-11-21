# MTile Lint

[Effective mTile](performance.md) · [Command reference](reference.md#commands)

MTile Lint reads your Lua and points out opportunities from Effective mTile.
It works like a reviewer who circles repeated work and asks a useful question.
You still decide whether the suggested change describes your game.

```sh
mtile lint mygame
```

Replace `mygame` with your project folder. You can also pass a Lua entry
file, for example `mtile lint mygame/game.lua`. With no
argument it checks the current directory. Put flags **before** the input path.

## In the IDE

Open **Develop → Lint** to see the same findings while editing. The native
editor checks the complete explicit `require()` graph, including unsaved
buffers, in its background source analysis. Click a source row or press Enter
to jump to it; MTL008 includes separate links for the first and replacing
constructions. **Docs** opens the explanation, and **Show context notes**
reveals the contextual allocation inventory. Advice remains separate from
build errors in Problems. See the [IDE guide](ide.md#effective-mtile-checks)
for interaction and snapshot behaviour.

## Read an item as a decision

Consider this small program:

```lua
readings = {0, 0, 0, 0}
function _update()
  readings = {1, 2, 3, 4}
end
```

The text report shows the code, the exact source point and a reason:

```text
advice[MTL001]: A new table can be allocated on a frame-reachable path.
  --> game.lua:3:14
  |
2 | function _update()
3 |   readings = {1, 2, 3, 4}
  |              ^ fresh table constructor
4 | end
  |
  = note: named-call witness: _update
```

The `help` lines explain an alternative and its tradeoff; the `docs` line is
a clickable URL into the relevant Effective mTile chapter. Text paths include
the project directory relative to where you ran the command (or an absolute
path when outside that directory tree), so an editor can
open `examples/celeste/src/presentation.lua` directly. JSON paths remain
project-relative. The byte column stays unchanged in both formats; the text
caret accounts for tabs and Unicode display width. Long source lines show a
window around the source point, with `...` where text was clipped.

Here you might create the four slots once and refill them.
But if something retains yesterday's table as a snapshot, reuse would change
that snapshot too. A report identifies the construction; it does not prove
that replacing it preserves ownership, pixels, timing, or gameplay.

The practical loop is **read the advice → check the game rule → change one
thing → verify the same replay → measure the complete frame**. Keep a useful
change; keep the original code when its flexibility or identity is needed.

## Advice and context notes

**Advice** identifies a source pattern worth considering. **Notes** preserve
allocation evidence whose context does not justify presenting it as a repeated
frame-work problem. The default report counts hidden notes explicitly. To see
their source frames and call paths:

```sh
mtile lint -include-notes mygame
mtile lint -include-notes -format json mygame
```

For MTL001, a construction reached through a syntactic conditional, short
circuit or potentially empty loop is a note. A helper reached by both a
guarded path and an unguarded path retains advice; the analysis is not based
on whichever caller happens to be visited first. An exact adjacent
`local p = deli(pool)` followed by `if p == nil then p = {...}` is recognized
as an existing reuse attempt with an allocation fallback, not missing pooling.

**Conditional does not mean rare.** A branch can run on almost every tick.
The classification exposes uncertainty; it does not measure frequency or
waive a frame budget. Conversely, a path without an enclosing syntactic guard
does not prove that it executes every frame: earlier returns, indirect calls
and game state still matter. The reported call path is one static witness,
not a replay trace or a list of every possible caller. No function-name
whitelist is used: calling `initialize_cache()` directly every tick still
produces advice.

MTL008 uses stronger evidence: a fresh result is replaced before an observed
read or escape in a supported trace. It remains **advice** when that trace is
inside an event such as spawning. Low execution frequency does not make a
demonstrably discarded construction necessary.

By default, documentation URLs point to repository Markdown on `main`. To use
the rendered local guide preview running at port 4191:

```sh
mtile lint -include-notes -docs-url http://127.0.0.1:4191 mygame
```

The base URL may include a hosting subdirectory. It changes report links only;
the linter makes no network request. The plain text report needs no ANSI colour
support and remains readable in saved logs.

## What the first version checks

Except for MTL002, rules inspect functions reachable by direct, named calls
from `_update()` or `_draw()`, including helpers and recursive call graphs.
They inspect possible branches, not how often a branch actually executes.
A door-opening branch can therefore produce legitimate advice you choose to
decline. Startup-only and unused helpers do not receive frame-work advice.

| ID | Detected source shape | The decision to make |
| --- | --- | --- |
| **MTL001** | A table constructor `{...}` in the resolved frame call graph; conditional paths and recognized reuse fallbacks become context notes | [Reuse scratch slots](performance.md#reuse-storage-when-the-same-slots-will-do) when older identities and values are not needed. New objects can be correct. |
| **MTL002** | An assignment that aliases a single, unshadowed, top-level global dense literal of 1–256 numbers | [Use direct indices](performance.md#make-a-fixed-list-easy-to-recognise) when the alias is unnecessary. Aliases prevent this particular storage proof; removing one does not prove every other eligibility condition. This check includes startup and unused named helpers because the compiler proof is program-wide. |
| **MTL003** | `pal()`, `pal(0)` or `pal(1)` on a frame path | [Update the mappings that change](performance.md#change-the-colours-that-actually-change) on the same layer when that is equivalent. Draw and display mappings have different roles. A full fade or reset that restores other state should remain. |
| **MTL004** | `mset()` with three or four literal integer arguments on a frame path | [Author permanent cells in assets](performance.md#keep-permanent-world-data-in-the-assets). Keep edits for doors, destruction and resets; the linter cannot know that a cell is permanent. |
| **MTL005** | A unit-step loop from 1 through the length of a fixed ASCII label whose sole statement is `print(sub("LABEL", i, i), ...)` | [Print a fixed label whole](performance.md#keep-a-fixed-label-whole) if font metrics, positions and colours match. Per-letter effects are valid exceptions. |
| **MTL006** | Two nonempty literal integer unit-step loops whose only operation is `pset(x, y, literal_colour)` using the two loop indices | [Consider `rectfill`](performance.md#ask-for-the-picture-you-mean) with the same inclusive bounds and colour. Confirm the selected renderer's final pixels. |
| **MTL007** | One statically numbered slot (1–256) of an unrebound, unshadowed top-level global table receives literal numbers and literal strings/booleans/nil; at least one such write is on a frame path | [Separate numeric state from existence](performance.md#keep-one-number-about-one-thing) when that matches the design. Mixed configuration is still useful, and zero is not a universal missing-value marker. |
| **MTL008** | A fresh, unescaped table result is replaced before observation in a bounded trace, including supported helper calls | [Avoid discarded construction](#mtl008-discarded-construction). Preserve other state changes and any caller that needs the first result. |

The linter uses the actual mTile parser, so comments and strings containing
`pal()` or `{}` are not mistaken for calls or allocations. It follows the
build's explicit `require()` graph, checks each included module once and maps
findings back to the original file, line and byte column. A directory uses
`project.json`'s `entry`, defaulting to `game.lua`. Passing a file uses that file
as the entry with its parent as the source root; use a project directory for
the project's configured entry and module layout.

It does not scan unrelated files, load artwork/audio, execute Lua, invoke
TinyGo, build a ROM, modify source, or automatically apply fixes. A full build
still checks semantic and renderer compatibility; lint only parses and checks
the source patterns above. No new Lua API or renderer eligibility is introduced.

## MTL008: discarded construction

Imagine building a model, replacing it with another model, and only then
showing anyone the result. The first model consumed time and materials, even
if this happened only when entering a room. That is a stronger reason to
investigate than merely noticing that construction occurred.

Before, rebuilding the same result twice:

```lua
-- lint-example: discarded-before
local points = {}
total = 0
function build_points()
  points = {}
  for i = 1, 3 do
    add(points, {x = i, y = 0})
  end
end
function _update()
  build_points()
  build_points()
  total = points[1].x + points[2].x + points[3].x
end
```

After, constructing the result that is actually used:

```lua
-- lint-example: discarded-after
local points = {}
total = 0
function build_points()
  points = {}
  for i = 1, 3 do
    add(points, {x = i, y = 0})
  end
end
function _update()
  build_points()
  total = points[1].x + points[2].x + points[3].x
end
```

Both produce the same three points and `total = 6`. The first result in the
before program has no observer outside its construction. The second call
replaces the whole list before any game rule reads it. Appending elements
while building that list is part of construction, rather than a consumer
depending on its final value.

**Keep both results** if code between the calls reads the first or retains
its identity. For example, assigning `snapshot = points` between calls keeps
the first list alive; it is not discarded. A helper can also change other
state, consume randomness, play sound, or call an unknown observer. Removing
an entire helper would then need a different justification. MTL008 reports
the discarded table result and its related source positions; it does not
automatically delete helpers or replace separate objects with a shared one.

The rule follows lexical bindings and object paths. Names such as
`build_points`, `actor.ribbon`, or `player.hair` have no special meaning to it.
It handles plain local/global bindings, static fields and literal indices.
For a nested field proof, its containing tables must be known fresh and
unescaped in the current trace. Unknown existing parent identities do not
qualify. Reads, aliases and escapes stop the relevant proof; unknown effects,
dynamic indexing, multiple assignments and uncertain control flow are
conservative barriers.

Direct zero-argument helpers with simple bodies can be followed across files.
Supported nonempty literal unit-step loops can be traced for up to 64
iterations; changing an induction variable prevents that proof. Recursion,
parameterized calls, early-return helpers and other unsupported shapes end
cross-call reasoning. Each function trace also has a 2,048-statement budget.
These limits bound the analysis; they are not instructions to change the
game's counts. A missing finding is not proof that the work is necessary.

The text report includes related source frames for the first call, the
replacement call and the replacing write when distinct. JSON exposes the
same locations through `related`, with `path`, `line`, `column`, `label`, and
`snippet`. A discarded-result finding appears by default and participates
in `-strict`, even when MTL001 records the underlying allocation as a
conditional context note. No cycle saving is inferred from the finding.

## Use it in CI or with an agent

The default exit status is zero after a successful analysis, including when
advice is present. Opt into a failing gate when your project has agreed which
rules to enforce:

```sh
mtile lint -strict mygame
mtile lint -strict -format json mygame > lint.json
```

`-strict` returns status 1 when enabled rules report advice. Context notes do
not fail the gate, including with `-include-notes`. Input, resolution,
syntax, flag and output failures also return status 1. JSON stays on standard
output; command errors go to standard error. `"diagnostics": []` means no
visible items; check `hidden_notes` to see whether contextual items were omitted.

The JSON object contains `files`, `disabled_rules`, `hidden_notes`, and `diagnostics`. Each
diagnostic has `rule`, `path`, `line`, `column`, `message`, `suggestion`, and
`guide`. It also includes `severity`, `label`, `snippet` (original line numbers
and source text), and a clickable `documentation` URL. Allocation diagnostics
include a `call_path`, optional `conditional_at` location and explanatory
`notes`. Paths are relative to the source root; `guide` retains its repository
Markdown path for existing integrations. The same rule IDs appear in terminal
and JSON output. Source excerpts come from the resolved source snapshot rather
than a later reread of potentially changed files.

If intentional palette resets are part of your project's design, you can
exclude that rule explicitly:

```sh
mtile lint -strict -disable MTL003 mygame
mtile lint -format json -disable MTL003,MTL004 mygame
```

Unknown rule IDs fail rather than silently disabling a misspelled check.
Disabled rules are listed in both report formats. This first version disables
a rule for the **whole analysis**, not one line; that can hide other useful
items under that rule. Record the reason in your project's CI instructions.
There are no inline suppression directives or automatic source rewrites.

A useful agent instruction is:

> Run `mtile lint -format json` on the project. For each item, explain the
> repeated work, the intended game behavior and the tradeoff. Propose a rewrite
> only when it preserves state ownership, results and final pixels. Check the
> same replay and complete 30 FPS frame budget before accepting it. Do not
> change gameplay or disable checks simply to get an empty report.

## What an empty report does not tell you

An empty report means no enabled advice was found; contextual notes may still
be counted separately. It is not an
optimization score, a frame-budget measurement, a complete type check, or a
promise that every compiler fast path was selected.

The analysis follows plain named top-level functions, including local helpers;
it does not resolve method/field calls or indirect function values. It skips
ambiguous function/global/builtin assumptions conservatively: a same-named
local or parameter anywhere, rebinding, or duplicate declarations can prevent
the relevant check even when the compiler could resolve that use precisely.
MTL002 and MTL007 intentionally cover only simple global table declarations,
not every local table, escaped alias, dynamic index or computed value.

It does not yet diagnose parameter-to-literal loop bounds, wrapped map-read
spelling, per-cell map drawing, manual IWRAM placement, or all string creation.
Those choices need stronger data-flow/asset evidence or profiling. In
particular, it will not recommend increasing a loop count to reach an
optimization threshold, changing a finite world to wrap, or moving all code
into fast RAM. Keep following the [measurement guide](performance.md#measure-a-decision)
for actual target performance.

## Celeste case study: 27 sites are not 27 mistakes

The October 31, 2026 report of the local `examples/celeste` snapshot identified
27 real table constructors in six resolved Lua files. The source positions
were correct. Presenting all of them as equivalent optimization advice was
too broad: call-graph reachability alone did not establish steady-state churn,
missing reuse, or safe ownership changes.

All 27 source sites were reviewed:

| Source and lines | Function | What the construction does |
| --- | --- | --- |
| `game.lua:79,91` | `reset_player` | A new player record and its initial hair field during reset/spawn. This is state reconstruction, not normal movement code. |
| `src/entities.lua:43` | `entities_reset` | Rebuilds the active entity list for a room or respawn. |
| `src/entities.lua:61` | `entity_add` | Creates a distinct entity, stores it in the active list and returns it. One shared record would merge live entities. |
| `src/presentation.lua:42,44` | `create_hair` | Builds one hair list and separate segment records during player initialization. The segments are subsequently updated in place. |
| `src/presentation.lua:138` | `clear_smoke` | Replaces the active list after returning its objects to the existing inactive pool at respawn. |
| `src/presentation.lua:145` | `effect_smoke` | Allocates only after `deli(smoke_pool)` returns nil. Its other branch overwrites and reuses all particle fields; expired particles return to the pool. |
| `src/presentation.lua:211,213` | `create_death_particles` | Creates a separate burst on player death, with distinct live particle records. |
| `src/presentation.lua:284,285,286,287,289,294,297` | `initialize_effects` | Prepares effect containers, clouds and ambient particles on a new game. Their regular update paths mutate existing records. |
| `src/presentation.lua:377,378,379,380,381,386,389,392,395,398` | `initialize_render_cache` | Prepares persistent render slots on a new game. Preparing these once is already the reuse strategy described by Effective mTile. |

That is **17 new-game setup sites, nine reset/entity/effect sites and one
recognized reuse fallback**. Those are source-site counts, not allocations
per tick. A constructor inside a loop can execute many times; a constructor
behind a restart button may not execute during a normal replay at all.
Entity and particle events can also burst or repeat on consecutive frames.

Before the discarded-result rule was added, this snapshot produced **0 advice
items and 27 contextual notes counted separately**. MTL008 now adds **two
advice items** for the two discarded constructions described below; the 27
allocation notes remain available with `-include-notes`. This is not a claim
that Celeste is allocation-free, that transitions are cheap, or that no
further optimization is possible. No Celeste gameplay code was rewritten for
this review, and no new cycle measurements were made.

The manual review did find a concrete construction to investigate:
`start_spawn()` calls `reset_player()`, which creates hair, then changes the
spawn coordinates and calls `create_hair()` again before using that first
hair list. The first list and its five segment records are replaced. Also,
the initial `hair = {}` field in the player constructor is replaced by
`create_hair()`. Consolidating that initialization may reduce transition
allocations, but the other `reset_player()` callers still need valid hair.
That calls for a scoped game change with state/pixel checks and transition
profiling, not a blanket instruction to reuse every entity or remove a pool
fallback. MTL008 now detects both the overwritten empty hair field at
`game.lua:91` and the constructed hair result at `src/presentation.lua:42`.
For the latter it shows the first call at `game.lua:94` and the replacing
call at `game.lua:108`, together with the field write inside the helper.
MTL001 continues to describe allocation context; MTL008 supplies the separate
discarded-result evidence. The implementation contains no Celeste-specific
function names, fields or source positions.
