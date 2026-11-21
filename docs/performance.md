# Effective mTile

<p class="effective-opening">A faster game starts with a better description of the work. Learn to give mTile the same game in a form it can carry out more cheaply.</p>

You do not need assembly, register names or hardware experience. Imagine a
small workshop preparing a new picture thirty times a second. You decide
what the picture means. When you build the cartridge, a **compiler** translates
your Lua instructions into code the GBA can run. Clear rules help it prepare a
cheaper plan before play begins, avoiding the same questions on every tick.

<figure class="effective-loop" aria-label="Prepare once at startup. Each game tick reads controls, updates the world, draws a picture and publishes it. Repeat the tick at the 30 FPS target.">
  <div class="effective-once"><strong>Prepare once</strong><span>Load art · create persistent state</span><code>_init()</code></div>
  <ol><li><strong>Read controls</strong><span>What does the player want?</span></li><li><strong>Update the world</strong><code>_update()</code></li><li><strong>Draw its picture</strong><code>_draw()</code></li><li><strong>Publish</strong><span>Make the finished picture visible</span></li></ol>
  <figcaption>The order is a workflow, not a duration scale. Preparation should not become an unnecessary job on every tick.</figcaption>
</figure>

The aim is **the same movement, collisions, colours and drawing order with less
work**. A shorter program is not automatically cheaper. A faster program that
changes the game is a different program.

This chapter starts with pictures, then shows working Lua pairs. Each technical
note explains the actual compiler rule and the price of using it. The older
measurement reference is preserved [at the end](#historical-reference).

## Think in jobs, then in code

A **variable** is a name for a value, like `player_x = 40`. A **table** groups
values: `positions = {10, 20, 30, 40}`. A **loop** repeats instructions;
`for i = 1, 4 do ... end` visits four slots. A **function** gives a job a name
so it can be called again. Lua list slots start at **1**; map coordinates and
many drawing coordinates start at **0**.

Read `=` as “store this value” and `==` as “are these equal?” Quoted text such
as `"none"` is a string; `true` and `false` are yes/no values. Square brackets
choose a list slot: `positions[1]` reads its first value. `if ... then ... else`
chooses what to do, and `end` closes a block. A `--` line is a comment for the
reader. `%` keeps a remainder: a counter's `% 2` alternates between 1 and 0.
You will see each of these in a complete example below.

mTile loads the authored assets. It calls `_init()` for your startup work,
`_update()` to advance the world, and
`_draw()` to describe its next picture. A **tick** means one update of the game.

The GBA gives the usual 30 FPS game loop about **561,792 CPU cycles per tick**.
A cycle is a small unit of processor time. Saving cycles leaves room for another
enemy or a busier scene; it does not make the player move faster. The actual
LCD cadence makes this target approximately 29.86 updates a second. This guide
keeps that same two-display-scan target throughout.

Start with the decision that matches your problem:

- [Temporary data keeps being created](#reuse-storage-when-the-same-slots-will-do): prepare storage, then refill it.
- [One value keeps changing its kind](#keep-one-number-about-one-thing): separate the number from its meaning.
- [A list has a permanent shape](#make-a-fixed-list-easy-to-recognise): let the compiler see that shape.
- [Work repeats a fixed number of times](#state-a-fixed-rule-only-when-it-is-fixed): expose the real bound.
- [You paint a shape or a tile grid by hand](#ask-for-the-picture-you-mean): use a whole operation.
- [A map, label or colour keeps being rebuilt](#keep-permanent-world-data-in-the-assets): distinguish permanent content from changes.
- [You need to know whether any of this helped](#measure-a-decision): compare the same game, including its expensive moments.

<details class="effective-note" markdown="1">
<summary>Which compiler version does this describe?</summary>

The advanced fast-path rules below describe the verified optimization set
through **`b99eb51`**, measured on October 31, 2026. Older builds can execute
the same Lua through more general paths. For example, the older `934434e`
checkout does not contain every automatic placement, wrapped-map or palette
optimization described here. These are implementation conditions, not new Lua
language rules. Check your build's selected renderer, memory use and compiler
report; do not infer a feature from a benchmark number.

</details>

## Reuse storage when the same slots will do

Imagine four reusable cups for four sensor readings. Pouring new readings into
them is different from throwing away the cups and making four new ones.

<figure class="effective-compare" aria-label="Before: create a scratch table, fill it, discard it and create another next tick. After: create one table, refill the same four slots on every tick.">
  <div><strong>Replace the container</strong><p>Make a list → fill four slots → use it<br>Next tick: make another list</p><span class="effective-trade">Less state to manage; repeated allocation</span></div>
  <div><strong>Refill the container</strong><p>Make a list once<br>Every tick: fill its four existing slots</p><span class="effective-trade">Persistent storage; fewer temporary objects</span></div>
  <figcaption>The second drawing shows the same container at different times. Reuse does not mean keeping the previous readings.</figcaption>
</figure>

Here the scratch readings must be replaced every tick. Each program produces
the same four numbers and `total`.

Before, replacing the scratch table:

```lua
-- effective-example: reuse-before
tick = 0
readings = {0, 0, 0, 0}
total = 0
function _update()
  tick = tick + 1
  readings = {tick, tick + 1, tick + 2, tick + 3}
  total = readings[1] + readings[2] + readings[3] + readings[4]
end
```

After, reusing slots that are all overwritten before they are read:

```lua
-- effective-example: reuse-after
tick = 0
readings = {0, 0, 0, 0}
total = 0
function _update()
  tick = tick + 1
  for i = 1, 4 do
    readings[i] = tick + i - 1
  end
  total = readings[1] + readings[2] + readings[3] + readings[4]
end
```

**Choose reuse** for a permanent set of sensors, particle slots or temporary
results. **Choose separate objects** when each object needs its own identity or
must outlive this calculation. If another part of the game keeps the old table,
refilling it changes what that part sees. Reuse is then a change of ownership,
not a harmless spelling change.

A fixed pool also reserves space for inactive slots. Do not reserve hundreds
of objects merely to avoid making three occasional ones. And do not clear all
of RAM: overwrite the fields the next user actually needs.

<details class="effective-note" markdown="1">
<summary>Under the hood: why creating less can prevent a pause</summary>

General Lua tables and newly constructed strings can use the Lua heap. A
**garbage collector** finds objects that are still reachable and reclaims the
rest. An unreachable object is not necessarily erased immediately. Repeated
allocation creates pressure; a larger live set can also make collection more
expensive. Eligible fixed numeric lists can avoid general table machinery
entirely. Reuse helps even when that stronger proof does not apply, but it does
not promise that the whole game becomes allocation-free. A `local` declared
inside `_update()` can be recreated every tick; a chunk-level `local` outside
functions persists. Scope and lifetime are separate from the word `local`.

See the [illustrated garbage-collection guide](garbage-collection.md).

</details>

## Keep one number about one thing

An enemy's position and whether that enemy exists are two facts. Putting a
number in a box on one tick and the word `"none"` in it on the next makes every
reader ask which meaning the box currently has.

Before, deliberately using an indexed general table for mixed values:

```lua
-- effective-example: meaning-before
tick = 0
target = {"none"}
total = 0
function _update()
  tick = tick + 1
  if tick % 2 == 0 then
    target[1] = 12
  else
    target[1] = "none"
  end
  total = 0
  if target[1] ~= "none" then
    total = target[1] + 1
  end
end
```

After, giving existence its own flag:

```lua
-- effective-example: meaning-after
tick = 0
target_x = 0
has_target = false
total = 0
function _update()
  tick = tick + 1
  has_target = tick % 2 == 0
  if has_target then
    target_x = 12
  end
  total = 0
  if has_target then
    total = target_x + 1
  end
end
```

The requested result is unchanged: no target gives `0`; a target at `12` gives
`13`. The after version keeps an old coordinate while the flag is false, and
never treats it as active. The flag is necessary: zero could be a real position.

**Choose separate facts** for frequently used numbers and state flags. A
flexible, indexed table is still useful for genuinely mixed configuration or
occasional data. Do not expand every dictionary into dozens of variables on
principle.

<details class="effective-note" markdown="1">
<summary>Under the hood: stable numeric types are a contract</summary>

mTile can specialize proven numbers into native fixed-point values and use
numeric function parameters/results when all relevant call paths agree.
In its monomorphic paths, changing an established numeric binding or static
record field into a different Lua type is unsupported. The general indexed
storage above is intentional; it does not show permission to write
`target_x = "none"` after establishing `target_x` as a number.

Fixed-point numbers have finite range and precision. Keep fractional movement
when the game needs it; replacing it with integer movement changes gameplay.
Do not replace ordinary division with bit shifts without proving the rounding,
sign and overflow behavior. The compiler already optimizes eligible constant
arithmetic exactly. See [language rules](reference.md) for the numeric model.

</details>

## Make a fixed list easy to recognise

A row of numbered trays can hold one kind of item in a known number of slots.
A general container must also support more flexible contents. Both are useful;
the row is cheaper when the game really has that fixed rule.

<figure class="effective-storage" id="effective-storage-lab" aria-labelledby="storage-title storage-description">
  <strong id="storage-title">Same numbers, smaller slots</strong>
  <p id="storage-description">A byte is a small unit of storage. On GBA, one general value slot occupies 12 bytes; one proven numeric element occupies 4. These bars count element storage only, excluding table headers, keys and other game memory. This is a storage model, not an emulator or a speed prediction.</p>
  <label for="storage-count">Number of elements <output id="storage-count-value" for="storage-count">16</output></label>
  <input id="storage-count" type="range" min="1" max="32" value="16" step="1" aria-describedby="storage-description">
  <div class="effective-storage-row"><div><strong>General value slots</strong><span id="storage-general">192 bytes</span></div><div class="effective-track"><span id="storage-general-bar" style="width:50%"></span></div></div>
  <div class="effective-storage-row"><div><strong>Proven numeric elements</strong><span id="storage-numeric">64 bytes</span></div><div class="effective-track"><span id="storage-numeric-bar" style="width:16.6667%"></span></div></div>
  <div class="effective-scale"><span>0 bytes</span><span>384 bytes · one shared scale</span></div>
  <p id="storage-feedback" role="status" aria-live="polite">16 elements save 128 bytes of element storage when the numeric representation is eligible.</p>
  <noscript><p>The initial 16-element example remains valid without JavaScript. The slider needs JavaScript to recalculate the bars.</p></noscript>
  <figcaption>More compact storage leaves room for other data. It does not make the GBA's memory larger or prove that a particular loop is three times faster.</figcaption>
</figure>

Before, giving the whole list a second name, called an **alias**:

```lua
-- effective-example: list-before
positions = {1, 2, 3, 4}
total = 0
function _update()
  local points = positions
  for i = 1, 4 do
    points[i] = points[i] + 1
  end
  total = positions[1] + positions[2] + positions[3] + positions[4]
end
```

After, keeping direct accesses the compiler can follow:

```lua
-- effective-example: list-after
positions = {1, 2, 3, 4}
total = 0
function _update()
  for i = 1, 4 do
    positions[i] = positions[i] + 1
  end
  total = positions[1] + positions[2] + positions[3] + positions[4]
end
```

Both modify the same four positions. In this case the alias is unnecessary.
An ordinary temporary number such as `local speed = 2` is not the problem.
**Choose direct, fixed numeric storage** when the collection has one stable
identity, known slots and numeric contents. **Keep a general table** when the
game needs growth, mixed values, aliases or observable identity. Those features
are useful; they simply need a more general representation.

<details class="effective-note" markdown="1">
<summary>Under the hood: exactly what the current proof needs</summary>

The global numeric-list proof accepts a single top-level dense literal of
1–256 numbers, numeric writes and proven in-range integer indices, without
rebinding, shadowing, escaping aliases or identity-observing uses. It must be
initialized before it can be read. Private-local specialization has its own
narrower contract; adding `local` everywhere is not a speed switch.

Depending on the optimization set, the compiler can keep eligible numeric
storage in the game state, use a compact heap array, or retain a general table.
For some supported non-allocating, bounded loops it validates the aligned
storage once and exposes the existing words to native code. No detached second
copy is created. A call, allocation or unproven index can prevent that loop
proof even when the program remains valid. These are separate decisions:
**representation**, **safe loop access**, and **where the code/data fit**.

</details>

## State a fixed rule only when it is fixed

A workshop can prepare exactly four trays in advance. It needs a more general
plan if today's order can contain any number. The difference matters only
when the number really is part of the game's design.

Before, carrying a permanently fixed count through a parameter:

```lua
-- effective-example: fixed-before
total = 0
function probe(probe_limit)
  total = 0
  for i = 0, probe_limit - 1 do
    total = total + mget(i % 4, 0)
  end
end
function _update()
  probe(64)
end
```

After, stating this game's rule inside the helper:

```lua
-- effective-example: fixed-after
total = 0
function probe()
  total = 0
  for i = 0, 63 do
    total = total + mget(i % 4, 0)
  end
end
function _update()
  probe()
end
```

Both do exactly 64 probes. **Choose the fixed spelling** when every real call
must do that work. **Keep the parameter** when rooms, enemy counts or gameplay
really vary it. Never do extra probes to reach an optimization threshold.

<figure class="effective-proof" aria-label="The compiler checks facts, chooses a fast path when proven, and otherwise keeps a general path. A failed proof is not a different game rule.">
  <div><strong>Facts it can establish</strong><p>Fixed range · stable storage · supported effects</p></div>
  <div><strong>Proof succeeds</strong><p>Prepare a tighter loop and check its memory budget</p></div>
  <div><strong>Proof is unavailable</strong><p>Keep the general path and the original behavior</p></div>
  <figcaption>The compiler needs evidence, not hints that change the game's rules. Both paths must implement the same intended result.</figcaption>
</figure>

Small helper functions are still good design. Keep numeric inputs numeric and
return the intended kind on every path. Avoid allocating a table just to pass
a few temporary numbers to a frequently called helper. Do not flatten every
function into one enormous loop: readability, code size and inlining matter too.

<details class="effective-note" markdown="1">
<summary>Under the hood: bounded loops and automatic fast-memory placement</summary>

The verified frame-loop pass considers frame-reachable, non-allocating
functions with a nonempty literal integer loop spanning at least 16 iterations,
including nested blocks, with a non-wrapping induction. Other fixed-array
placement rules remain separate. Calls inside a borrowed-array or map-read
loop can have stricter eligibility than the surrounding function's placement.

The builder inspects the actual linked machine body. A source loop may already
have collapsed into a small straight-line calculation; forcing a new function
boundary can then hurt. The builder preserves ordinary inlining for those
flattened candidates and retries ordinary placement on a real capacity failure.
A source loop is a candidate, not proof that fast-memory placement will help.

</details>

## Ask for the picture you mean

If you want a filled rectangle, saying “draw a filled rectangle” gives the
engine more information than handing it one hundred separate pixel requests.
It still has to produce the same one hundred pixels.

Before, drawing a 10 × 10 solid square pixel by pixel:

```lua
-- effective-example: shape-before
function _draw()
  cls(1)
  for y = 10, 19 do
    for x = 20, 29 do
      pset(x, y, 7)
    end
  end
end
```

After, requesting the whole shape:

```lua
-- effective-example: shape-after
function _draw()
  cls(1)
  rectfill(20, 10, 29, 19, 7)
end
```

Both paint 100 pixels in Mode 4. The final coordinates are inclusive.
**Choose the whole shape** for a uniform fill. **Keep per-pixel work** when the
pixels actually have different colours, effects or gameplay work between them.
One API call is not one CPU cycle, and this example does not promise a 100× gain.

### A tile grid is also a whole operation

For an ordinary unscaled map of 8 × 8 tiles, the same idea applies:

Before, reading and drawing each map cell in Lua:

```lua
-- effective-example: map-draw-before
function _draw()
  cls(1)
  for y = 0, 2 do
    for x = 0, 3 do
      spr(mget(x, y), x * 8, y * 8)
    end
  end
end
```

After, asking for that same rectangle of tiles:

```lua
-- effective-example: map-draw-after
function _draw()
  cls(1)
  map(0, 0, 0, 0, 0, 4, 3)
end
```

The seven arguments select source layer 0, source cell (0, 0), screen position
(0, 0), four columns and three rows. Cell counts and screen pixels are different
units.

This pair assumes blank transparent tile 0, ordinary source transparency and
no special per-cell changes, transforms or ordering. Both use the same authored
4 × 3 map. If the loop deliberately changes a cell's appearance between draws,
a single `map()` may describe a different picture; keep the custom work.

<figure class="effective-compare" aria-label="Mode 4 paints the hidden bitmap in software before publication. Mode 0 can retain tiles and ask hardware to compose supported backgrounds and objects. Both must produce the same picture.">
  <div><strong>Paint the hidden picture</strong><p>Software writes its pixels<br>Publish the completed bitmap</p><span class="effective-trade">Mode 4 · flexible pixel effects</span></div>
  <div><strong>Arrange reusable tiles</strong><p>Store tile art and its arrangement<br>Hardware composes the picture</p><span class="effective-trade">Mode 0 · proven tile/BG/OBJ shapes</span></div>
  <figcaption>These are different ways to make the same picture, not “good” and “bad” renderers. Eligibility depends on exact semantics and resource limits.</figcaption>
</figure>

Leave `renderer: "auto"` as a useful starting point. Read why a build selected
a renderer. A full-screen pixel effect or unsupported ordering can legitimately
need Mode 4. Do not remove the effect simply to make a Mode 0 diagnostic vanish.
`rectfill()` remains a Mode 4 operation; using it does not make that game
eligible for hardware tiles. See [renderer compatibility](render-modes.md).

For a normal full redraw, `cls()` at the beginning describes replacing the
picture. In Mode 4 this can avoid preserving the previous page. For an
intentional trail or a small persistent patch, clearing everything changes the
effect. “Always clear” and “never clear” are both poor universal rules.

## Keep permanent world data in the assets

A wall that never changes is a good thing to paint in the map editor. A door
that opens during play is a good thing to change when its state changes.
Those are different kinds of work.

Before, repeatedly imposing a permanent tile from Lua:

```lua
-- effective-example: asset-before
function _update()
  mset(0, 0, 2)
end
function _draw()
  cls(1)
  map(0, 0, 0, 0, 0, 4, 3)
end
```

After, **authoring tile 2 at map cell (0, 0) in the asset first**:

```lua
-- effective-example: asset-after
function _update()
end
function _draw()
  cls(1)
  map(0, 0, 0, 0, 0, 4, 3)
end
```

The first visible tick has the same wall. This is an asset-and-code change,
not permission to delete an `mset()` call and hope the wall remains. It is safe
only if no other game rule depends on the earlier empty cell or changes it.
Keep `mset()` for doors, destruction and genuine edits; perform those changes
at the relevant state transition rather than rebuilding the world by habit.

<details class="effective-note" markdown="1">
<summary>Under the hood: immutable data enables a different route</summary>

A compiler that proves the authored map cannot be written can avoid some
streaming lookup/cache work and read unchanged data directly from ROM where
its accepted shape permits it. A writer can prevent that whole-program proof;
even an initialization-only edit is still a writer. Moving `mset()` from
`_update()` to `_init()` reduces repetition but does not automatically establish
an immutable cartridge map. Camera-window reuse and dirty-span publication are
engine work; keep using the appropriate `map()` call rather than writing a new
Lua cache by default. Large jumps or invalidation can still require a rebuild.

Authored art also has a cost. An unchanged spritesheet can use compact or
precomputed storage; runtime `sset()` needs writable data. Duplicating many
animation frames trades computation against cartridge and tile memory. Use
the pose/animation asset pipeline and measure the complete scene rather than
assuming duplicated art is always better than a supported hardware flip.

</details>

## Expose wrapped reads when the world really wraps

“After the last column, return to the first” is a game rule. Remainder (`%`)
is a way to express it. In a width-four wrapping world, `5 % 4` returns `1`.
In a finite world, reading beyond its edge should instead keep the game's
out-of-map behavior. Those worlds must not be confused for an optimization.

Before, putting already-wrapped coordinates into temporary locals:

```lua
-- effective-example: wrap-before
total = 0
function _update()
  total = 0
  for i = 0, 255 do
    local x = i % 4
    local y = (i * 3) % 3
    total = total + mget(x, y)
  end
end
```

After, calculating those same wrapped coordinates directly in the read:

```lua
-- effective-example: wrap-after
total = 0
function _update()
  total = 0
  for i = 0, 255 do
    total = total + mget(i % 4, (i * 3) % 3)
  end
end
```

Both read the same cells in the same order. **Choose this spelling** when the
locals just hold the calculations above and the game already wraps. **Keep
locals** when they avoid shared work or express coordinates used elsewhere.
A function that also moves an enemy must still run only once; keep its result
in a local when several operations need it.

<figure class="effective-proof" aria-label="Repeated read: check coordinates for every cell. Proven loop: check actual plane dimensions once, read proven in-range coordinates, and retain the checked loop as fallback.">
  <div><strong>The repeated question</strong><p>Does this coordinate fit?<br>Ask for every cell</p></div>
  <div><strong>A proof outside the loop</strong><p>Check this plane's actual dimensions<br>Read coordinates proven to fit</p></div>
  <div><strong>If the proof cannot apply</strong><p>Run the original checked loop<br>Keep its bounds and error behavior</p></div>
  <figcaption>The transformation removes repeated questions, not safety. It still uses the original map backing.</figcaption>
</figure>

<details class="effective-note" markdown="1">
<summary>Under the hood: the wrapped-map rule is deliberately narrow</summary>

The verified pass recognizes positive literal integer moduli directly in both
coordinates, in supported non-allocating literal loops spanning at least 128
iterations. The guard checks that a resident borrowed plane exists and its
actual dimensions are large enough. Negative/fractional dividends retain
Lua's positive-divisor remainder behavior before conversion to coordinates.
Unknown calls, unproven reads, known-layer writes, named divisors, unsupported
shapes and streaming retain their previous paths. A smaller actual image
fails the guard and uses the checked body. The compiler currently does not
carry these coordinate ranges through arbitrary locals.

This is a cost heuristic. Do not enlarge a 64-probe game to 128 probes to
qualify. The extra work can easily cost more than the checks it removes.
Extra generated code consumes memory; a real capacity failure retries checked
reads before sacrificing established placement.

</details>

## Change the colours that actually change

A palette is a colour lookup table: pixels store small colour indices, and the
palette gives those indices their visible colours. A colour flash can therefore
change an index's displayed colour without repainting every pixel using it.

Here the final `1` in the three-argument `pal()` call selects display mapping;
`pal(1)` resets that mapping layer.

Before, restoring the whole display mapping before changing one entry:

```lua
-- effective-example: palette-before
phase = 0
function _update()
  phase = (phase + 1) % 2
  pal(1)
  pal(2, 4 + phase, 1)
end
function _draw()
  cls(2)
end
```

After, changing that entry directly:

```lua
-- effective-example: palette-after
phase = 0
function _update()
  phase = (phase + 1) % 2
  pal(2, 4 + phase, 1)
end
function _draw()
  cls(2)
end
```

This small game changes **only display entry 2**; all other mappings remain
identity mappings. Each tick replaces entry 2's previous mapping. Under those
conditions the full reset adds no game rule. If other code changes other entries
and the reset is meant to undo it, removing the reset is wrong. Design who owns
the palette before simplifying its updates.

<figure class="effective-palettes" aria-label="A full palette publication resolves 256 entries. A one-entry edit can publish its aligned pair of two entries; unrelated entries retain their existing hardware colours.">
  <div><strong>Publish all 256 entries</strong><div class="effective-palette-grid effective-full" aria-hidden="true"><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span></div><p>Needed for initialization, resets and full changes</p></div>
  <div><strong>Publish one aligned pair</strong><div class="effective-palette-grid" aria-hidden="true"><span></span><span></span><span class="effective-changed">2</span><span class="effective-changed">3</span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span></div><p>Eligible sparse edit · indices 2 and 3 travel together</p></div>
  <figcaption>Each small square is one entry, on the same scale. mTile publishes two adjacent colours together with one four-byte write. Several edits coalesce into a range, which may include unchanged gaps.</figcaption>
</figure>

**Choose sparse updates** for a few independent colour changes. **Keep the full
change** for a fade or remap that really changes the whole palette. Splitting a
full-screen fade over several ticks to win a benchmark changes the effect.

<details class="effective-note" markdown="1">
<summary>Under the hood: why the compiler does not track every palette</summary>

Range bookkeeping itself costs time. The verified compiler selects it only
when a conservative whole-program proof bounds display edits to 1–32 per
frame. It follows helpers, sums possible branches conservatively and handles
supported literal loop counts. Larger/unknown counts, recursion, generic
iterators, frame-time resets and draw remaps retain the full-table runtime.
This is a build-time choice, not an extra runtime mode test on every colour.
Both Mode 0 palette banks and pose palette aliases retain their semantics.
Initialization-time composed mappings can still force a full publication;
small edit count alone is not a promise of a speedup in every program.

In a measured 128-edit control, an always-on prototype worsened 20,030 cycles
to 28,131. The final policy retained 20,030. That is why the advice is not
“always update individual colours”: use the operation the visual effect needs.

</details>

## Keep a fixed label whole

A sign saying SCORE is one reusable piece of content. Sending one letter at a
time makes the engine reconstruct what you already know is a fixed label.

Before, slicing and positioning each character separately:

```lua
-- effective-example: text-before
function _draw()
  cls(1)
  for i = 1, 5 do
    print(sub("SCORE", i, i), 8 + (i - 1) * 6, 8, 7)
  end
end
```

After, giving it the whole fixed label:

```lua
-- effective-example: text-after
function _draw()
  cls(1)
  print("SCORE", 8, 8, 7)
end
```

These pictures match for the **default six-pixel-advance font**. A custom font
can have different metrics; do not copy that manual spacing into every project.
**Choose a literal label** when the content is fixed. **Keep dynamic text or
separate glyphs** for a typewriter effect, per-letter movement or changing
content. A changing score should still change; do not replace it with a constant.

<details class="effective-note" markdown="1">
<summary>Under the hood: prepare what is known at build time</summary>

A literal text run with supported constant colour can expose reusable glyph
slots. Stable positions can allow a prepared layout as well. A substring loop
introduces separate calls and may create temporary strings; a general dynamic
text path can need conversion, validation or layout on each update. The exact
font, palette, object budget and renderer conditions still apply. Hardware
sprite entries and scanline limits remain real even if text construction is
prepared in ROM. Do not infer “free text” or a fixed cost for every font from
one label benchmark. See [fonts](fonts.md) for the actual metrics and limits.

</details>

## Use the fast workbench sparingly

The GBA has a small fast workbench and a larger store room. Moving a frequently
used tool closer can help; filling the workbench with every tool leaves no
space to work. You do not need to manage addresses to understand the choice.

<figure class="effective-memory-map" aria-label="IWRAM is a 32 KiB fast shared workbench. EWRAM is a 256 KiB larger shared store. VRAM is 96 KiB of display memory. Cartridge ROM holds code and authored assets. The diagram is not a relative-capacity or speed scale.">
  <div><strong>Fast shared workbench</strong><span>IWRAM · 32 KiB</span><p>Selected code, engine state and stacks share this space.</p></div>
  <div><strong>Larger shared store</strong><span>EWRAM · 256 KiB</span><p>Mutable data, runtime state and any linked Lua heap share it.</p></div>
  <div><strong>Picture workspace</strong><span>VRAM · 96 KiB</span><p>The selected renderer uses it for display data.</p></div>
  <div><strong>Cartridge shelves</strong><span>ROM</span><p>Code and authored assets. Its contents are prepared when building.</p></div>
  <figcaption>Named roles, not equal capacities or a universal speed ranking. Copying everything into fast memory is neither possible nor automatically faster.</figcaption>
</figure>

The newer compiler already tries eligible automatic placements and checks the
real linked budget. Manual placement is a later decision for **measured hot
helpers**. The following helper has a genuinely changing bound; it is not the
fixed-loop example above.

The Lua is intentionally identical in the two configurations:

```lua
-- effective-example: placement-before
tick = 0
total = 0
function probe(probe_limit)
  total = 0
  for i = 0, probe_limit - 1 do
    total = total + mget(i % 4, 0)
  end
end
function _update()
  tick = tick + 1
  probe(4 + tick % 4)
end
```

Before, ordinary project selection:

```json
{"iwramFunctions": []}
```

After, trying this helper explicitly:

```lua
-- effective-example: placement-after
tick = 0
total = 0
function probe(probe_limit)
  total = 0
  for i = 0, probe_limit - 1 do
    total = total + mget(i % 4, 0)
  end
end
function _update()
  tick = tick + 1
  probe(4 + tick % 4)
end
```

```json
{"iwramFunctions": ["probe"]}
```

Merge the field into your existing project file. This demonstrates the setting,
**not a measured speedup for this tiny example**. Keep it only if the complete
game improves within the shared memory budget. Removing it restores ordinary
selection. Lifecycle callbacks such as `_update` cannot be selected manually.

<details class="effective-note" markdown="1">
<summary>Under the hood: why moving code can also hurt</summary>

Inlining means placing a helper's instructions inside its caller instead of
making a separate call. Manual selection keeps a separate function boundary
and moves the selected body. Its callees and all game data do not automatically follow. The boundary
can prevent a compiler simplification, and selected code uses scarce IWRAM.
Automatic policy has conservative capacity fallbacks; explicit selections can
still overflow and require you to remove a name. The number of selected
functions is not a measure of optimization quality. The linker report measures
the actual program, including engine use and reserved stacks.

</details>

## Measure a decision

First ask what must remain true: the same collisions, positions, colours,
ordering, sound and saving behavior. Then change one choice and replay the same
scene. Smooth movement in a quiet room is not evidence about a crowded battle.

<figure class="effective-measure" aria-label="Preserve the game's rules, record a baseline, change one decision, compare the same replay and keep the change only when its measured tradeoff helps.">
  <ol><li><strong>State the rule</strong><span>What must remain the same?</span></li><li><strong>Record a baseline</strong><span>Same route, inputs and build</span></li><li><strong>Change one choice</strong><span>Know what you are testing</span></li><li><strong>Compare and decide</strong><span>Pictures · state · time · memory</span></li></ol>
  <figcaption>Measure complete game work at the same 30 FPS target. Changing a renderer or dropping effects without checking parity answers a different question.</figcaption>
</figure>

A **median** describes the middle measured update. A **p95** is the cost at or
below which 95% of sampled updates fall; it helps reveal expensive moments.
It is not the worst update. Check missed ticks and memory as well as ordinary
cost. Cross a map chunk boundary, open a door, spawn the busiest scene and reach
a collection point. Leave some headroom for work your short replay did not see.

The finished compiler investigation gives an example of an honest result:

<figure class="effective-results" aria-label="Measured sixteen display remaps at the same 30 FPS target: previous mTile 11389 cycles, final mTile 4670, strongest tested Butano 13476. All bars use a 15000-cycle scale.">
  <strong>Sixteen display remaps · complete Mode 0 update</strong>
  <div class="effective-storage-row"><div><span>Previous mTile</span><span>11,389 cycles</span></div><div class="effective-track"><span style="width:75.9267%"></span></div></div>
  <div class="effective-storage-row"><div><span>Final mTile</span><span>4,670 cycles</span></div><div class="effective-track"><span style="width:31.1333%"></span></div></div>
  <div class="effective-storage-row"><div><span>Strongest tested Butano</span><span>13,476 cycles</span></div><div class="effective-track"><span style="width:89.84%"></span></div></div>
  <div class="effective-scale"><span>0 cycles</span><span>15,000 cycles · shared detail scale</span></div>
  <figcaption>Measured snapshot, not a prediction for your game. Lower is less CPU work; all three already meet the same 30 FPS target.</figcaption>
</figure>

The unchanged 1,024-read map workload improved from 46,981 to **42,890 cycles**,
but the strongest tested Butano control remained faster at **34,157.5**. The
new palette path won its tested comparison; the larger map case still had a gap.
An optimization can be useful without proving that an engine wins every game.

These snapshots come from the completed `b99eb51` investigation: two independent
600-update captures after 60 warm-ups, pinned mGBA, and matching pixels, raw
LCD scans and scalar state. The C++ caller was tested in Thumb/ROM, ARM/ROM and
ARM/IWRAM at pinned Butano `04374b034c76b82bfdc3aa6c38d652e5e7778a68`.
They are emulator measurements; physical GBA confirmation remains outstanding.
The [teaching snapshot record](benchmarks/effective-mtile-guide-2026-10-31/README.md)
retains these summaries and their source provenance.
The warm-up count is not an FPS target. Your small examples above are tested
for behavior, not assigned these benchmark timings.

For a first check, `stat(1)` gives the previous tick's work fraction and
`stat(2)` the maximum since boot. Editor debug builds provide more phase
telemetry. Use [the survey commands](#frame-stability-survey) and the complete
cadence, state and image checks when making a performance claim. Some renderer
phase counters omit later presentation work: the Mode 4 palette primary
stopwatch in that investigation did. A low partial counter is not a complete
frame measurement. Host computer CPU percentage is not GBA cycle cost.

## Let MTile Lint point to a decision

You can ask the toolchain to look for some of the patterns taught here:

```sh
mtile lint mygame
```

Replace `mygame` with your project folder. **MTile Lint** reports a file and line, explains the repeated work and points back to this
guide. Its eight rules cover table creation, numeric-list aliases,
palette resets, literal map edits, split labels, pixel rectangles and mixed
slot meanings, and fresh constructions replaced before their results are
observed. It also follows direct named helpers called by the frame
callbacks, so moving work into a function does not hide it.

<figure class="effective-proof" aria-label="The linter identifies a possible cost. The developer checks the game rule and tradeoff. A replay and complete frame measurement determine whether to keep the change.">
  <div><strong>The tool finds a pattern</strong><p>This code may repeat work.</p></div>
  <div><strong>You check its purpose</strong><p>Is the alternative still my game?<br>What flexibility or storage does it cost?</p></div>
  <div><strong>The target supplies evidence</strong><p>Compare the same replay and complete frame.<br>Keep the change when it helps.</p></div>
  <figcaption>A clean lint report is not a speed measurement. Advice is a question to evaluate, not an instruction to remove game behavior.</figcaption>
</figure>

For example, a new table in `_update()` is easy to find automatically. Whether
that table can be reused depends on who retains its identity and old values.
Likewise, a constant `mset()` might be a permanent wall or an intentional door
reset. The report explains both choices; it does not rewrite either one.

Use `mtile lint -strict -format json mygame` when a CI job or agent needs a
machine-readable gate. Conditional table constructions and recognized pool
fallbacks are context notes, available with `-include-notes`; they do not fail
the advice gate. Conditional does not mean rare: use the source excerpt and
call path to decide what to profile. See the [Celeste review](lint.md#celeste-case-study-27-sites-are-not-27-mistakes)
for why 27 real constructors were not 27 proven optimization mistakes.
The stronger [MTL008 proof](lint.md#mtl008-discarded-construction) does report
discarded construction as advice, including in a spawn/reset path, with both
the first and replacing source locations.
See [MTile Lint](lint.md) for exact rule shapes, intentional
exceptions and limits. Several decisions still require profiling or asset
knowledge; an empty report does not certify that a game meets its 30 FPS budget.

## Give an agent a game rule and an acceptance test

Instead of “make it optimal,” describe the behavior it must preserve and the
evidence you will accept. For example:

> Build my game for the 30 FPS target. Preserve its rules, fractional movement,
> collision results, visible pixels, drawing order and sound. Use the current
> compiler's automatic paths where their proofs fit. Keep real changing counts
> dynamic; do not add probes, remove effects or change the world to qualify.
>
> Explain each proposed rewrite with the original and revised Lua, what stays
> the same, why the compiler can do less work, and what memory or flexibility
> it costs. Check the selected renderer and its diagnostics. Reuse storage only
> where ownership and identity permit it. Use whole drawing operations where
> they describe exactly the same picture.
>
> Compare the same replay before and after, including the busiest scene and
> boundary/collection events. Report complete cycles, median, p95, missed ticks
> and memory. Distinguish measured results from estimates and compiler rules
> from game-design choices. Keep a change only when its tradeoff helps this game.

You should be able to answer three questions about the result: **Is it still
my game? What repeated work disappeared? What did I give up to remove it?**
That is enough understanding to make a useful decision without reading assembly.

## Historical reference

The following is the earlier performance guide, preserved as an engineering
reference. Its per-call costs, budgets, recommendations and limits belong to
the stated renderer, toolchain and workload snapshots. They are not universal
current costs; use the decision guidance above and remeasure your build.

<details class="effective-history" markdown="1">
<summary>Open the earlier timing tables and implementation notes</summary>


This is the "Effective Mtile" for frame budgets: what the hardware gives
you, what each operation costs in measured ARM7TDMI cycles, and which
patterns help a game stay within its approximately 30 Hz budget. These are
recorded benchmark snapshots, not guarantees for arbitrary games or the current
checkout. The per-call table uses `runtime/cmd/benchspike`, mGBA, TM0/TM1
timers, and a 128x128 sheet with half-transparent art unless a row says
otherwise. Later sections link their own workloads, toolchains, and evidence.
Re-measure the relevant workload after runtime changes:

Unless a section explicitly says otherwise, graphics numbers in this guide
describe the Mode 4 software framebuffer backend. The compile-time Mode 0
backend accepts the statically proven tile/BG/OBJ subset, and AUTO selects it
only when both its semantic and benchmark-backed cost proofs succeed. The exact API contract and expansion gates are in the
[render-mode compatibility guide](render-modes.md) and
[engineering plan](render-mode-plan.md).

The tables below retain their stated benchmark snapshots. The September 5
[Celeste port measurements](celeste-port-performance-2026-09-05.md) add a
bounded Mode 4 raster cache for one small static map plane. Its exact-pixel
fast path substantially changes eligible map costs; older per-cell map timings
remain relevant to fallback calls, not to the new cached path.

The September 8 [division measurements](division-performance-2026-09-08.md)
cover exact literal arithmetic, faster IWRAM division helpers, and removal of
map-validation divisions, including complete-ROM arithmetic checks and a
pixel-identical Celeste route comparison.

The same day's [original Celeste Mode 4 measurements](celeste-mode4-performance-2026-09-08.md)
cover reusable compiler and software renderer optimizations, including bounded
multi-plane caching, native tile copies and exact target alignment checks.

The September 9 [TinyGo 0.42 tuning pass](toolchain-042-performance.md)
measures faster dynamic numeric table lookups under LLVM 22, with exact-pixel
renderer controls and a Celeste replay. The table workload improves; Celeste's
displayed cadence is unchanged. The report records the small string-key cost.

The [dirty-region experiments](celeste-dirty-regions-2026-09-08.md) compare
partial clearing and uploads against that optimized renderer. The tested
implementations preserve pixels but regress performance, so the existing
renderer remains enabled.

<h3 id="frame-stability-survey">Frame-stability survey</h3>

The survey builds each project as a debug ROM, plays a fixed input script in
the headless mGBA helper, and reports per-tick cycles, missed frames, GC
pauses, and IWRAM use.

<details markdown="1">
<summary>For contributors: reproduce the measurements from source</summary>

The survey uses the repository's `cmd/perfsurvey` tool. With the
[source build tools](getting-started.md#build-the-desktop-tools) installed,
run these commands from the repository root.

```sh
go run ./cmd/perfsurvey                          # every examples/ project
go run ./cmd/perfsurvey -input right -spikes 400000 examples/mapdemo
# pixel parity: hash every displayed frame with tick-keyed input, then compare
# the hash files from two runtimes
go run ./cmd/perfsurvey -renderer mode4 -tick-input -hashes /tmp/frames examples/pong
```

</details>

The October 25, 2026 survey (TinyGo 0.42.0, 3,600 frames per project, first
two seconds excluded) found that every example stays well inside the
561,792-cycle tick. Its worst ticks came from three runtime paths, each now
fixed without changing results:

| Change | Workload | Before | After |
| --- | --- | ---: | ---: |
| Row-copy streamed map chunk loads instead of per-cell index math | `mapdemo`, scrolling right for 60 s | 3 missed frames, worst tick 605,617 | 0 missed, worst tick 331,796 |
| Stop the GC sweep at the highest handle ever allocated, and run it from IWRAM | `pong` collection (2,048 handles, 540 live bytes) | 161,197 | 71,266 |
| Run GC tracing and copying from IWRAM and read only each slot's kind and ref | 26 KB live heap, 164 objects | 449,086 | 198,977 |

Mode 4 then drew into a 38,400-byte EWRAM framebuffer and copied it to the
hidden VRAM page every tick. Both costs ran at EWRAM's bus limit, so Mode 4 now
draws into the hidden VRAM page directly. Pixel writes merge into halfwords,
because VRAM ignores 8-bit stores. After a flip, the first write of a tick copies
the visible page across (VRAM to VRAM) to keep the framebuffer persistent; a
full `cls()` replaces the page and skips that copy, and a tick that draws
nothing does not flip. The EWRAM framebuffer is gone. Median Mode 4 tick cost:

| Workload | EWRAM framebuffer | Direct VRAM | Change |
| --- | ---: | ---: | ---: |
| empty `_draw` | 77,360 | 190 | -99.8% |
| `cls()` only | 139,425 | 23,876 | -82.9% |
| `print` only (no `cls`) | 83,592 | 45,653 | -45.4% |
| `hello` | 207,221 | 94,811 | -54.2% |
| `pong` | 186,430 | 71,072 | -61.9% |
| `mapdemo` | 263,980 | 146,232 | -44.6% |
| `multi-file` | 292,963 | 154,387 | -47.3% |

Every paired `cmd/renderbench` workload still reports zero differing pixels
between Mode 4 and Mode 0.

<h3 id="paired-static-map-proof">Paired static-map proof</h3>

`cmd/renderbench` built `testdata/projects/testio` twice with identical source/assets,
TinyGo 0.41.1, `-gc=none -opt=2`, and the pinned headless mGBA core. After 20
warm-up frames it compared and sampled 120 synchronized 30 Hz ticks.

| Metric | Forced Mode 4 | Forced Mode 0 |
| --- | ---: | ---: |
| LCD pixel differences | 0 | 0 |
| Median / p95 total cycles | 544,417 / 544,417 | 38,409 / 38,409 |
| Median draw cycles | 465,759 | 576 |
| Median commit cycles | 76,924 | 36,341 |
| Cold tile + map upload cycles | n/a | 1,967,744 |
| Missed frames | 0 | 0 |
| ROM bytes (debug build) | 25,980 | 25,528 |
| EWRAM free | 60,600 | 94,640 |

Mode 0 avoids redrawing 600 8x8 cells and copying a 38,400-byte framebuffer on
every tick; its immutable tiles and screen map remain resident in VRAM. That is
why this deliberately static case wins by an unusually large margin. The table
does not predict mutation-heavy, scrolling, sprite, text, or primitive-heavy
games. Those supported shapes have separate parity fixtures; their cost must be
measured with `cmd/renderbench` for the actual project. The 1,967,744-cycle
(~117 ms) cold tile/map upload is excluded from the steady-state median and
occurs during forced blank, before BG0 is enabled.

The complete `tile-v1` matrix is checked in under
[`docs/benchmarks/render-modes`](benchmarks/render-modes/README.md). Camera,
two-layer, composite OBJ/text/palette, 16 `sset`, and 16 `mset` workloads all
passed parity, memory, p95, missed-frame, and ≥20% median-speed gates. Runtime
`fset` remained pixel-exact and 15.67% faster than Mode 4, but deliberately
failed the conservative 20% AUTO gate; `tile-v1` therefore selects Mode 4 for
that source shape.

Layer-first map access has its own
[`layered-map-access-v1`](benchmarks/render-modes/layered-map-access-v1.json)
record. Sixteen `mget(1,x,y)` reads and sixteen literal-layer
`mset(1,x,y,id)` writes per tick are pixel-exact and retain large Mode 0 cycle
savings. Host microbenchmarks keep the unchanged BG0 arities as explicit
regression controls and report zero allocations. Dynamic-layer mutation stays
forced-Mode-0-only in AUTO until its widest 16-write shape is measured.

<h4 id="compile-time-obj-glyph-interning">Compile-time OBJ glyph interning</h4>

Mode 0 `print()` admits static printable-ASCII strings, decimal numbers, and literal colors,
so the linker now stores one 64-byte OBJ tile per unique `(glyph,color)` pair.
The sorted atlas is uploaded once during force blank; repeated occurrences and
later frames reuse the resident tile. OAM and scanline accounting remain per
visible character, while the tiles 480-511 capacity now limits unique pairs.

The `text_intern.lua` fixture draws 32 copies of `A` in color 7. With the same
20-warm-up/120-sample capture before and after interning:

| Metric | Per-occurrence staging | Static interned atlas | Change |
| --- | ---: | ---: | ---: |
| Pixel differences vs Mode 4 | 0 | 0 | exact |
| Median total cycles | 162,434 | 71,117 | -56.22% |
| Median draw cycles | 94,901 | 16,062 | -83.08% |
| Median commit cycles | 65,551 | 53,403 | -18.53% |
| Mode 0 ROM | 53,808 B | 52,332 B | -1,476 B |
| Mode 0 EWRAM free | 94,864 B | 96,848 B | +1,984 B |
| Missed frames | 0 | 0 | unchanged |

Against Mode 4, the resulting Mode 0 cartridge is 57.91% faster instead of
3.86%, with zero differing pixels. It still budgets 32 OAM entries and 256
worst-line OBJ cycles; interning removes duplicate tile construction and
per-frame tile DMA, not the hardware cost of displaying each character.

<h4 id="packed-16x16-obj-proof">Packed 16x16 OBJ proof</h4>

The `pack16.lua` fixture draws eight 16x16 sprites from one immutable source
rectangle and covers all four flip combinations. Compared with commit
`e2fc183` immediately before packing, the same 20-warm-up/120-sample capture
measured:

| Metric | Four 8x8 OBJs | One packed 16x16 OBJ | Change |
| --- | ---: | ---: | ---: |
| Pixel differences vs Mode 4 | 0 | 0 | exact |
| Median total cycles | 66,914 | 61,828 | -7.60% |
| Median draw cycles | 18,148 | 11,292 | -37.78% |
| Median commit cycles | 46,832 | 48,602 | +3.78% |
| Mode 0 ROM | 51,752 B | 53,020 B | +1,268 B |
| Mode 0 EWRAM free | 96,928 B | 96,904 B | -24 B |
| Missed frames | 0 | 0 | unchanged |

The steady path uploads no packed tile data after first use. The static proof
now accounts for one OAM entry per cached rectangle and its true 16-cycle
scanline width.

<h4 id="packed-32x32-obj-proof">Packed 32x32 OBJ proof</h4>

The `pack32.lua` fixture draws two distinct 32x32 sprites and animates all four
flip combinations. Compared with the same pre-packing commit and capture
settings:

| Metric | Sixteen 8x8 OBJs | One packed 32x32 OBJ | Change |
| --- | ---: | ---: | ---: |
| Pixel differences vs Mode 4 | 0 | 0 | exact |
| Median total cycles | 63,259 | 48,318 | -23.62% |
| Median draw cycles | 14,302 | 3,214 | -77.53% |
| Median commit cycles | 46,832 | 43,230 | -7.69% |
| Mode 0 ROM | 52,400 B | 53,484 B | +1,084 B |
| Mode 0 EWRAM free | 96,904 B | 96,872 B | -32 B |
| Missed frames | 0 | 0 | unchanged |

The two sources fill the entire 32-cell cache and both render as one OBJ per
draw. Each still costs its true 32-pixel width in the scanline-cycle proof.

<h4 id="full-oam-and-scanline-proof">Full OAM and scanline proof</h4>

The renderer now separates the GBA's two physical limits: 128 OAM entries per
frame and 1,226 normal-OBJ rendering cycles per scanline. Literal Y positions
are composed on the hardware's wrapping 0..255 coordinate; dynamic positions
are assumed to overlap in the worst case. Horizontally offscreen objects are
not discounted.

| Workload | OAM | Worst line | Mode 4 median | Mode 0 median | Saving | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 distributed 8x8 sprites | 64 | 64 cycles | 219,164 | 149,119 | 31.96% | 0 |
| 38 overlapping packed 32x32 sprites | 38 | 1,216 cycles | 1,458,945 | 116,580 | 92.01% | 0 |

The second case is ten cycles below the official 1,226-cycle ceiling; a 39th
32-pixel OBJ is rejected at build time because 1,248 cycles cannot be proven
safe. Mode 0 missed no frames in either capture. The deliberately raster-heavy
Mode 4 side of the 38-sprite characterization missed 140 cumulative ticks, so
that record is a parity and hardware-boundary proof rather than a balanced
performance comparison.

For bounded dynamic one-cell sprites, ID 0 has an additional runtime-only fast
path: when its logical 8x8 tile is entirely the default-transparent source
color 0, the draw emits no OAM entry. Authored or `sset()`-mutated nonzero
pixels disable the fast path immediately and preserve ordinary `spr(0)` output.
The compiler deliberately continues to budget each call as active, so AUTO
eligibility never depends on a particular animation frame. The paired
`inactive_sprite_slots.lua` workload alternates 64 calls between IDs 0 and 1
and is the parity/performance fixture for this path. Across 120 synchronized
frames it records zero differing pixels and no missed frames: Mode 0 measures
68,920 median cycles versus 171,235 for Mode 4 (59.75% lower) and returns
37,088 EWRAM bytes.

<h4 id="object-depth-between-four-backgrounds">Object depth between four backgrounds</h4>

`object_depth_lanes.lua` draws the same 8x8 object after each of four real,
overlapping BG passes. Mode 0 derives priorities 3, 2, 1, and 0 from painter
order; ordinary and packed objects use the same Attribute-2 rule. The analyzer
still applies the shared OAM/scanline proof and rejects the unavailable lane
behind every materialized BG.

| Metric | Mode 4 | Mode 0 |
| --- | ---: | ---: |
| Pixel differences across 120 synchronized frames | 0 | 0 |
| Median total cycles | 1,215,615 | 50,854 |
| Median draw cycles | 1,136,997 | 6,750 |
| Median commit cycles | 76,924 | 42,528 |
| EWRAM free | 62,340 B | 82,956 B |
| Static object proof | software painter | 4 OAM / 16 line cycles |

This is a 95.82% median-cycle saving and returns 20,616 EWRAM bytes. Mode 0
records one cold-start missed frame from the four-layer upload; Mode 4 records
140 cumulative missed ticks on the deliberately full-screen four-pass raster
workload. The parity gate therefore establishes exact ordering independently
of the separate frame-delivery characterization.

```sh
cd runtime
make benchspike-gba
mgba benchspike.gba     # the table prints on screen
```

Host-side regression benchmarks (relative cost, allocation checking) live in
`runtime/bench_test.go`:

```sh
cd runtime
go test -bench=. -benchmem
```

Target runtime hot paths must report `0 allocs/op` on the Go heap after
initialization. This does not mean Lua tables and strings are allocation-free:
they use the separate managed Lua heap. Host tooling and benchmark setup may
have different allocation requirements.

<h3 id="the-budget">The budget</h3>

One display frame is **280,896 cycles** (1232 cycles x 228 scanlines,
59.73 Hz). The generated loop runs `_update`/`_draw` at **30 Hz** - once
every two display frames, PICO-8's native rate - so one game tick owns
**561,792 cycles**. For the Mode 4 framebuffer path, fixed per-tick costs include:

| Cost | Cycles | % tick |
| --- | ---: | ---: |
| First write after a flip: copy the visible page to the hidden page (skipped by a full `cls()`) | ~39,000 | 7% |
| Input poll, tick, palette, flip | ~1,000 | <1% |

Drawing targets the hidden page (which cannot tear), and nothing is copied at
present time. In practice, treat **~520,000 cycles as the ceiling for
`_update` plus `_draw` combined**.

Two display frames provide twice the CPU budget of a one-frame loop. In the
recorded per-cell Mode 4 workload, a dense 16x16 room redraw fits, while a
30x20 redraw at ~832k cycles exceeds even the two-frame budget. Sparse rooms
or eligible raster-cache calls have different costs; measure the actual game.
The LCD scans at approximately 59.73 Hz, so the two-frame target is about
29.86 updates per second. The TM2/TM3 pacer prevents an early flip; on-time
ticks show each image for two scans. An overrun can hold an image longer and
cause uneven cadence. Pacing does not make an expensive frame fit.

The frame profiler (`Context.SetProfiling`) reports exact per-phase costs
on real hardware; `MissedFrames` counts ticks that overran the two-frame
slot.

<h4 id="what-a-tick-buys-you-30-hz-vs-60-hz">What a tick buys you: 30 Hz vs 60 Hz</h4>

| | 60 Hz loop | 30 Hz loop (current) |
| --- | ---: | ---: |
| Budget per game tick | 280,896 cycles | **561,792 cycles** |
| Fixed blit share | 27% | **14%** |
| Full 16x16 room `map()` redraw, dense art | 126% - did not fit | **63% - fits** |
| Complete Celeste-class tick (cls + room + 30 sprites + 24 particles + HUD) | ~123% - did not fit | **61% + 14% blit = 75% - fits** |
| Aligned 8x8 sprites after `cls` | ~70-95 | **~180** |
| Input latency | up to ~17 ms | up to ~33 ms |

The input-latency row describes the polling interval, not total end-to-end
latency: rendering, display scanout, and an emulator can add delay. The
approximately 30 Hz loop trades a longer polling interval for twice the CPU
budget. Overruns can still extend a tick beyond its target slot.

<h3 id="measured-costs-per-call">Measured costs per call</h3>

| Operation | Cycles | % tick (30 Hz) | Notes |
| --- | ---: | ---: | --- |
| `cls(c)` | 67,542 | 12% | DMA3 fill; cost is fixed |
| `rectfill` full screen | 67,723 | 12% | full-width spans use the same DMA fill |
| `rectfill` 32x32 | ~2,500 | <1% | word-store span loop |
| `map` 30x20, 50% transparent art | 832,381 | 148% | still cannot run every tick |
| `map` 30x20, fully opaque art | 763,901 | 136% | packed words help but cells dominate |
| `spr` 8x8 x100, aligned | 207,698 | 37% | ~2,077 per sprite |
| `spr` 8x8 x100, odd x | 225,874 | 40% | ~2,259 per sprite |
| `spr` 8x8 x100, flipped | 296,186 | 53% | ~2,962 per sprite |
| `sspr` 8x8 -> 16x16 x10 | 136,882 | 24% | ~13,700 per stretch |
| `print` 40-char row | 36,138 | 6% | ~900 per glyph |
| `line` full diagonal | 44,903 | 8% | ~187 per pixel |
| `circfill` r=16 | 22,445 | 4% | span-based |
| `sin`+`cos` x100 | 34,291 | 6% | ~343 per pair, LUT |
| framebuffer DMA blit | 77,093 | 14% | paid every tick |
| `map` 16x16 dense | 354,046 | 63% | a full PICO-8 room, worst-case art |
| `map` 16x16 half-empty | 184,446 | 33% | realistic platformer density |
| `map` 16x16 with layer mask | 101,054 | 18% | masked passes reject cells cheaply |
| `map` 30x16 half-empty | 344,718 | 61% | wide scroller band |
| **complete PICO-8 tick** | **344,785** | **61%** | cls + 16x16 room + 30 sprites + 24 particles + HUD |
| **complete GBA tick** | **352,077** | **62%** | cls + 17% 30x20 room + 1 sprite + HUD ([`mapdemo`](../examples/mapdemo)) |

The complete PICO-8 tick fixture costs 344,785 cycles before the ~77k upload,
leaving approximately 140k cycles in the two-frame slot. This is evidence for
that synthetic workload, not a guarantee for a port. Game logic, allocations,
collection, and other drawing can consume the remaining budget.

<h4 id="what-these-numbers-mean-in-practice">What these numbers mean in practice</h4>

- **The measured PICO-8-sized room (16x16 cells) redraw fits the tick budget**
  with the tested dense art (63% before clearing, sprites, and upload). This is the workload that makes
  room-based platformer ports straightforward: no dirty-rectangle
  bookkeeping needed.
- **Map-cell cost hierarchy** (each measured): an *empty* cell (sprite 0)
  is ~free - `map()` skips it before any pixel work, so a half-empty room
  costs half a dense one. A *fully opaque* tile is the fast path (packed
  32-bit copies). A *partially transparent* tile costs ~9% more (per-byte
  fallback around its color-0 holes). Level-art rules that follow: never
  fill sky/holes with opaque "background-colored" tiles - leave them
  empty and let `cls()` provide the backdrop for free. For drawn tiles,
  baking the backdrop into the art keeps the fast path, but only when the
  backdrop is actually known (a flat sky); tiles that sit over varying
  content need their transparency, and its ~9% premium is a fair price -
  don't contort art to dodge it.
- **A dense 30x20 full-GBA-screen `map()` every tick still does not fit**
  (149%). Workarounds, in preference order: sparse platformer rooms fit
  (62% for a full tick at 17% cell density - see [`mapdemo`](../examples/mapdemo));
  the measured 30x16 half-empty band costs 61% before other work; the
  measured 16x16 masked pass costs 18%; or draw
  once per room and per tick patch only the strip a camera scroll exposed
  (~90k cycles per 64-cell patch).
- **Sprite budget by scenario** (8x8 `spr`, blit already subtracted;
  "realistic" leaves ~20% headroom for logic, particles, and HUD):

  | Frame also does... | Max aligned | Realistic |
  | --- | ---: | ---: |
  | sprites only | 233 | ~190 |
  | `cls` + sprites | 200 | ~160 |
  | `cls` + sprites, all odd-x | 184 | ~145 |
  | `cls` + sprites, all flipped | 140 | ~110 |
  | `cls` + half-empty 16x16 `map` + sprites | 112 | ~90 |
  | `cls` + dense 16x16 `map` + sprites | 30 | ~25 |

  Rule of thumb: **~200 sprites bare, ~90-110 in a real game that also
  redraws its room**. A short object list can fit comfortably in these measured scenarios, but
  object count alone does not bound game cost. 16x16 sprites cost ~4x an 8x8, so ~50 of them
  after `cls`. Offscreen sprites reject before pixel work and are
  near-free, so drawing the whole entity list without camera culling is
  fine.
- Prefer even x positions (the packed fast path needs 4-byte alignment);
  prefer unflipped art (pre-flip in the sheet when a sprite is always
  mirrored) - flipped sprites cost ~43% more.
- `sspr` stretching costs ~6.5x an equivalent `spr`. A few per tick are
  fine; scaling every entity every tick is not.

<h3 id="how-the-runtime-spends-memory">How the runtime spends memory</h3>

| Region | Size | Speed | What lives there |
| --- | --- | --- | --- |
| IWRAM | 32 KB | 32-bit, zero-wait | stacks (9 KB: 8 KB user + 1 KB IRQ), blit/print/span cores (~5 KB), TinyGo runtime data |
| EWRAM | 256 KB | 16-bit, 2-wait | `Context` (framebuffer, palettes), mutable sheet/map/flag RAM, 2x56 KB Lua heap |
| VRAM | 96 KB | 16-bit | two Mode 4 pages |
| ROM | up to 32 MB | 16-bit + prefetch | code, assets, LUTs |

Sprite sheets are cell-compacted by default: when static analysis finds no
`sset()` call, generated ROM and EWRAM contain only non-empty 8x8 cells plus a
two-byte logical-ID index. Pixel drawing still reads EWRAM—the direct-ROM
prototype measured 100 sprites at 223,509 cycles versus 207,699 from EWRAM
and made a dense map exceed the tick budget. The final compact-EWRAM path
measured 100 sprites at 211,107 cycles (about 1.6% over the old dense layout)
and preserved the mapdemo-class full-tick result within measurement noise. Calling `sset()` anywhere opts
the project into a complete mutable EWRAM sheet. Flags remain mutable in
EWRAM because `fset()` is cheap at one byte per sprite.

The single most important architectural fact: **ARM code fetched from ROM
pays two bus accesses per instruction; the same code in IWRAM pays one
zero-wait access.** Moving the pixel cores to IWRAM (`//go:section .iwram`)
more than doubled sprite and print throughput without changing a line of
logic. IWRAM is the scarcest resource; only proven hot loops earn a place
there, and the linker map (`make benchspike-elf`, then check `.data` size)
must stay well under 32 KB.

Projects can opt measured hot Lua helpers into IWRAM with `iwramFunctions`
in `project.json`, for example `"iwramFunctions": ["draw_particles"]`.
Names must identify plain top-level functions from the resolved Lua source;
duplicates, unknown names, methods and the `_init`, `_update`, `_draw`
callbacks are rejected. The setting is optional, works with either renderer,
and survives editor saves. It changes code placement only: selected functions
receive `//go:noinline` and `//go:section .iwram`; their arguments, return
values and Lua behavior stay the same. Other functions retain normal placement.

The available IWRAM is shared with runtime code, data, the 8 KiB user stack,
the 1 KiB IRQ stack, and 96 reserved bytes. A helper's callees are not relocated
automatically. Check the build's linker-measured IWRAM usage and compare real
GBA cycle counts before keeping a selection; avoiding inlining can also cost
performance. The linker rejects an overflowing selection rather than silently
moving functions back to ROM. Remove names from the array to restore ordinary
placement; omitting the array preserves existing projects' behavior.

<h3 id="runtime-design-rules-enforced-not-aspirational">Runtime design rules (enforced, not aspirational)</h3>

1. **Zero Go-heap allocations after Init.** `make smoke-gba` fails the
   build if TinyGo reports any runtime allocation. Lua tables and strings
   live in two fixed 56 KiB semispaces with a Cheney collector when the Lua
   heap is linked.
2. **No divides in per-pixel or per-cell loops.** ARM7TDMI has no divide
   instruction; a software divide costs hundreds of cycles. `spr`/`map`
   convert sprite ids to cells with a fixed-point reciprocal multiply.
3. **Word writes wherever alignment allows.** The framebuffer fill, packed
   sprite rows, and the VRAM blit all move 32 bits per store. VRAM itself
   cannot take 8-bit writes at all, which is why presentation is DMA of
   whole words.
4. **Fixed costs go to hardware.** `cls` and full-width `rectfill` are DMA3
   fills; the frame blit is DMA3 with the whole VDraw window to run in.
5. **Bounds checks stay out of pixel loops.** The blit cores take raw
   pointers after their callers establish bounds from the cell clamps. The
   clamps are tested; the cores are covered by the golden-framebuffer test.

<h3 id="good-practice--bad-practice-for-game-code">Good practice / bad practice for game code</h3>

**Do:**

- **Call `cls()` every tick without guilt.** It is a fixed-cost 32-bit
  fill of the hidden VRAM page: about 24,000 cycles, 4% of the tick, no
  matter what was on screen. The framebuffer is persistent (nothing
  implicitly clears it), so a tick that draws without `cls` first copies
  the previous image (~39,000 cycles). For a full redraw, `cls` is
  therefore cheaper than skipping it. Patching only what changed pays off
  when the patch is small and you accept the bookkeeping.
- Keep per-frame table work on the array part (`t[i]` with `i` in
  1..cap). Array access is a direct indexed read; hash access costs a
  probe sequence with 12-byte compares.
- Static field syntax (`object.x`) is monomorphic by default: its first
  execution validates the table and resolves the hash slot, then that access
  site assumes the value remains a table with the same field layout. This
  removes repeated type/hash/key checks from record-like game state. Replacing
  the value with another type, adding/removing fields in a way that changes its
  layout, or using different record shapes at the same function access site is
  unsupported; use indexed access (`object["x"]`) when the shape is dynamic.
- A chunk-scope, fixed-shape numeric record is scalar-replaced when the whole
  program proves that its identity and table layout are unobservable. Every
  field then becomes one direct `rt.Number` member: no table allocation,
  static field key, hash slot, or field cache is linked. Aliasing the record,
  using bracket indexing, rebinding it, adding a field, storing an explicit
  non-number, or shadowing its name automatically retains the ordinary table
  path. Debug builds expose scalarized fields as `record.field` watch entries.
  In a 64-iteration target workload, the conservative scalar path measured
  259,324 -> 43,915 median Mode 0 cycles (-83.07%) and 41,352 -> 26,212 ROM
  bytes (-36.61%), with zero differing pixels; the real first-room Celeste
  frame measured 45,224 -> 44,705 cycles and 44,780 -> 30,076 debug-ROM bytes.
- Retained maps at or below the 16,384-cell threshold select a compile-time
  compact runtime in both renderers, so their ROM cannot retain the chunk
  loader, streaming lookup, or sparse-edit table. On the first-room Celeste
  cartridge this changed the release ROM from 27,708 to 24,616 bytes (-11.2%)
  and the Mode 0 debug benchmark from 44,705 to 44,521 median cycles, with zero
  differing pixels or missed frames. Streaming maps use the fixed streaming
  path in Mode 0 (36,333 -> 28,390 cycles in the paired map-only target
  workload). Mode 4 deliberately retains its generic streaming path: forcing
  the same specialization measured 397,312 -> 411,340 cycles, so that losing
  variant is not selected.
- On a compact map, `mget`/`mset` with either their implicit BG0 form or a
  literal/proven-constant layer compile directly to that layer's dense storage
  slot. Dynamic layers and every streaming map retain the full logical-layer
  lookup. Four collision rectangles per tick measured 22,248 -> 21,720 Mode 0
  cycles and 662,505 -> 662,021 Mode 4 cycles; ROM changed by -40 and -168
  bytes respectively, with identical draw/commit costs, EWRAM, missed frames,
  and pixels.
- A cartridge with no `sset()` specializes its renderer at link time. Mode 4
  uses the compact immutable-sheet representation and removes its storage
  branch from every `spr`, `sspr`, and `map` sample; the collision fixture
  measured 711,140 -> 662,021 cycles (-6.91%), draw 629,936 -> 580,813, and
  debug ROM 24,408 -> 22,596 bytes. Mode 0 deliberately does not use that
  representation because its earlier steady-state candidate regressed by 66
  cycles. Instead, a narrower Mode 0 tag preserves the faster sheet layout and
  deletes only the impossible per-frame dirty-tile scan. That changed the same
  fixture from 47,668 -> 21,720 cycles (-54.43%), commit 41,425 -> 15,451, cold
  initialization 3,399,010 -> 3,348,082 cycles, and ROM 23,040 -> 22,544 bytes.
  Every pair had zero differing pixels, zero new missed frames, and unchanged
  EWRAM.
- The periodic Lua collector and its two 56 KiB target semispaces are omitted
  when the whole program is proven allocation-free after numeric-record scalar
  replacement. Any ordinary/aliased table, dynamic string operation, boxed
  global, uncertain call, or disabled proof keeps the original collector and
  memory unchanged. On the first-room Celeste cartridge this changed the
  release ROM from 23,036 to 21,280 bytes, debug ROM from 25,212 to 23,516,
  and free EWRAM from 96,240 to 210,928 bytes; median/p95 stayed non-regressive
  at 43,751 -> 43,747 cycles with zero differing pixels or missed frames. A
  heap-free target also replaces the otherwise unreachable handle table, scan
  queue, and root stacks with compile-only sentinel slots. On the constant
  collision workload this returned another 28,380 EWRAM bytes in both
  renderers with bit-identical frames and exactly unchanged total/draw/commit
  cycles. At that stage the release Celeste cart had 239,324 bytes free EWRAM
  and remained 21,188 ROM bytes. Allocating carts retain the full metadata and
  both semispaces.
- Audio control builtins (`sfx`, `music`, `sample`, `gba_note`, and
  `gba_stop`) participate in that same no-allocation proof: they may change
  audio state, but cannot allocate a Lua table or string. In an otherwise
  heapless cart calling `sfx` and `music` every tick, the Mode 0 median changed
  from 2,912 to 2,883 cycles, ROM from 34,476 to 32,576 bytes, and free EWRAM
  from 108,976 to 252,040 bytes. Mode 4 changed from 149,032 to 148,952 cycles,
  ROM from 33,988 to 32,100 bytes, and free EWRAM from 70,848 to 213,920 bytes.
  Both renderer pairs had zero differing pixels and zero missed frames.
- `print` stays conservatively allocating for a dynamic/non-string first
  argument because `PrintV` may call `tostr`. When that argument is a literal
  string, however, it is already interned and generated code uses the
  literal-only `PrintStaticV` call graph. That graph cannot reach `tostr`, the
  dynamic-string decoder, either 56 KiB semispace, or the root-stack metadata.
  The literal-text fixture measured 41,084 -> 36,568 Mode 0 cycles (-10.99%),
  19,596 -> 16,676 ROM bytes, and 143,068 more free EWRAM. Mode 4 measured
  164,215 -> 159,746 cycles (-2.72%), 26,136 -> 17,840 ROM bytes, and the same
  EWRAM gain. Both pairs had zero differing pixels and zero missed frames. A
  real TinyGo link test rejects any heap symbols or a Mode 0 `Context` at or
  above 16 KiB; this caught an intermediate implementation whose generic
  string decoder retained the heap even though source-level effect analysis
  correctly reported the cart as heapless.
- First-observed numeric globals, locals, numeric-for counters, arithmetic
  expressions, and numeric record fields use native 16:16 `rt.Number` storage
  and operations. Reassigning one of those bindings or fields to a different
  Lua type is unsupported. Keep genuinely polymorphic values behind bracket
  indexing or separate bindings. The paired 16-body physics fixture in
  [`testdata/projects/physics-stress`](../testdata/projects/physics-stress) measured a 32.10%
  reduction in update/overhead cycles with zero differing pixels across 600
  Mode 4 frames.
- A chunk-scope numeric binding initialized once from a literal or an earlier
  proven constant is emitted as an exact 16:16 compile-time constant. Any
  second write, pre-declaration read, shadowing, multi-assignment, computed
  initializer, or declaration after the first function retains ordinary
  storage. The paired first-room Celeste collision workload performs four
  `solid_rect` queries per tick. Constant propagation changed Mode 0 from
  22,792 to 21,720 median cycles and 22,824 to 22,544 debug-ROM bytes; Mode 4
  changed from 662,765 to 662,021 cycles. Both returned 16 more EWRAM bytes,
  produced zero differing pixels, and retained the same missed-frame counts.
  The real Mode 0 release cartridge changed from 21,280 to 21,188 ROM bytes.
- The transpiler counts unique interned static strings exactly and selects the
  smallest safe 1/8/32/128/256-entry `Context` tier. The public 256-string
  source limit and overflow behavior are unchanged. The string-free Celeste
  collision workload returned 4,080 EWRAM bytes in both renderers while ROM,
  total/draw/commit cycles, missed frames, and pixels remained exactly
  unchanged. The one-entry tier is a sentinel for the zero-string case. After
  this tier, fixed map-slot lowering, and immutable-sheet upload elision, the
  collision-only Celeste benchmark cartridge at that measurement was 20,612
  ROM bytes with 243,404 bytes of EWRAM free. This is not the later full port
  with player, objects, effects and music.
- Fixed, non-escaping global numeric array literals use compact 4-byte Number
  elements instead of 12-byte Value slots. Direct indexed reads/writes and
  `#array` stay native; an escape, shadow, hash field, or explicit non-number
  assignment keeps the generic table. The 64-particle, 512-access/tick
  diagnostic measured 3,253,949 -> 542,498 median cycles (-83.33%), 3,200 ->
  1,040 bytes of Lua heap allocation (-67.50%), and 63,940 -> 39,364 ROM bytes
  (-38.44%), with unchanged linker EWRAM and zero differing pixels across 600
  Mode 4 frames. Element types are permanent under the same monomorphic
  contract as numeric bindings.
- Direct user functions receive native `rt.Number` parameters only when every
  static call site supplies a proven number. Return positions become native
  independently when every control-flow path supplies a number in that slot;
  mixed, nil-padded, falling-through, or unknown positions stay as `rt.Value`.
  The paired 12-body,
  ~280-call/tick fixture in
  [`testdata/projects/function-physics-stress`](../testdata/projects/function-physics-stress)
  measured 985,948 -> 740,131 median cycles (-24.93%) and a 36.27% reduction
  in update/overhead residual, with zero differing pixels across 600 Mode 4
  frames. This deliberately over-budget workload amplifies call overhead; the
  percentage is not a promise for ordinary games.
- Multi-return specialization preserves Lua expansion while allowing mixed Go
  tuples such as `(rt.Number, rt.Value, rt.Number)`. In the dedicated 240-call
  fixture at
  [`testdata/projects/multi-return-physics-stress`](../testdata/projects/multi-return-physics-stress),
  native tuple results measured 2,206,075 -> 1,169,327 shipping-path cycles
  (-47.00%), 1,927,649 -> 896,819 update/overhead cycles (-53.48%), and 50,688
  -> 50,532 ROM bytes (-0.31%). With the same root-protected path in both
  builds, they measured 2,438,893 -> 1,363,142 cycles (-44.11%), 2,163,634 ->
  1,082,622 update/overhead cycles (-49.96%), and 57,148 -> 56,716 ROM bytes
  (-0.76%). Both comparisons had zero differing pixels across 600 Mode 4
  frames and unchanged EWRAM.
- Calls to transitively proven non-allocating functions bypass the generated
  temporary-root push/pop wrapper. Allocation through table creation/growth,
  string creation/concatenation, an unknown builtin, or an unproven recursive
  cycle keeps the protected path. A dedicated workload with one live table
  root and 384 pure numeric calls per tick in
  [`testdata/projects/noalloc-call-stress`](../testdata/projects/noalloc-call-stress) measured
  330,184 -> 239,432 median cycles (-27.49%) and 106,632 -> 15,774
  update/overhead cycles (-85.21%), with zero differing pixels across 600 Mode
  4 frames. Its one cold-start missed tick fell to zero. This fixture isolates
  wrapper overhead and deliberately overrepresents tiny helper calls.
- Table deletion and inspection operations (`del`, `deli`, `count`, and the
  `all`/`pairs` iterator tokens) are included in the same effect proof. Unlike
  `add`, a field/index store, or `foreach`, these operations cannot grow a
  table or invoke an allocating callback. A 128-pair-per-tick `deli`/`count`
  fixture measured 530,084 -> 472,461 Mode 0 cycles (-10.87%) and 27,948 ->
  26,796 ROM bytes; Mode 4 measured 688,174 -> 632,906 cycles (-8.03%) and
  27,976 -> 26,688 ROM bytes. EWRAM was unchanged and both pairs produced zero
  differing pixels. Mode 0 retained zero missed frames in both runs.
- Reuse tables. Create a persistent pool at chunk scope or assign a global
  in `_init`, then recycle its entries. This avoids
  `add`/`del` churn: `#t` and `add` both walk the array border, and the
  collector's pause grows with live data.
- Use `flr()` results cached in locals when the same coordinate feeds
  several draws; repeated conversions and arithmetic still consume CPU cycles, even with
  numeric specialization.
- Profile on hardware early with `SetProfiling` + `PerformanceStats`;
  `MaxUpdate`/`MaxDraw` catch the worst frame, not the average.

**Avoid:**

- Repeated full-screen per-cell map rendering. A qualifying static Mode 4
  plane now uses a raster cache; measure the complete frame and its fallbacks
  before assuming it fits. See the Celeste measurements linked above.
- `sspr` scaling on many entities per frame.
- Flipping sprites at draw time when the art could carry both facings.
- String building in `_update`/`_draw`. Concat allocates from the Lua
  heap every call and drives collector pauses; `print` of a static string
  allocates nothing.
- Deep `del(t, value)` on long arrays: it is a linear search plus a
  shift. `deli(t, i)` with a known index avoids the search but still shifts entries; swap-with-last and
  pop is O(1).

<h3 id="known-limits-clear-expectations">Known limits (clear expectations)</h3>

| Limit | Value | Where enforced |
| --- | --- | --- |
| Update rate | 30 Hz (two 59.73 Hz scans per tick) | generated loop |
| Measured Mode 4 sprite budget (cls + draw) | ~180 8x8, workload dependent; Mode 0 instead has OAM/scanline limits | budget math above |
| Full-screen map redraws per tick | cache/renderer dependent; measure the full frame | map raster eligibility and target timing |
| Lua heap | 56 KB live data when allocation is reachable; collector and both 56 KiB semispaces are link-elided after a whole-program no-allocation proof | `runtime/heap.go`, transpiler effect analysis |
| Table array part | 256 entries | `maxArrayCap` |
| Table hash part | 128 entries | `maxHashCap` |
| String length | 4,096 bytes | `maxStringBytes` |
| Interned static strings | 256 | `maxStaticStrings` |
| Locals rooted across calls | 512 | `maxLocals` |
| Map streaming threshold | >16,384 retained plane-cells streams from ROM; compact storage is link-time specialized in both renderers, streaming storage in Mode 0 only | `mapcache.go`, build tags |
| Streaming map edits (`mset`) | 3,072 distinct cells | `MapEditFillLimit` |
| EWRAM for assets + heap + Context | 256 KB total | linker + `ValidateHeapBudget` |
| Continuous positive world coordinate | 32,767 px (4,095 whole 8px tiles) | signed 16:16 `Number` |
| Cartridge ROM | 32 MiB total | GBA address space + `ValidateROM` |

When a limit fails, it fails closed and observably: heap exhaustion returns
`HeapOOM` (nil result in Lua) and preview builds show `HEAP OOM` in the editor,
map-edit exhaustion sets `LastMapError`, and the build refuses ROMs whose
static assets cannot fit EWRAM.

<h3 id="measuring-your-own-game">Measuring your own game</h3>

```lua
-- game.lua: watch the real per-phase costs on screen
function _draw()
  cls(0)
  -- ... your drawing ...
end
```

`stat(1)` reports the previous tick's work as a fraction of the two-display-frame
budget; `stat(2)` reports the maximum since boot. Both work without enabling
phase profiling. Display either with `print(stat(1), 8, 8, 15)` inside `_draw()`.
Editor preview builds enable `SetProfiling(true)` automatically for detailed
phase telemetry; hand-written Go cartridges can enable it explicitly.
`MissedFrames` counts ticks that overrun the two-frame slot, including the
renderer presentation deadline. Validate both cadence and pixels on your
intended emulator/hardware; host CPU percentage is not this measurement.


</details>
