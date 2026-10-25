# `foreach`: call a function for every numbered table item

`foreach` is PICO-8-style statement syntax for applying one named function to
every item in the contiguous numbered part of a table.

```lua
foreach(table_expression, function_name)
```

- `table_expression` is evaluated exactly once.
- `function_name` is a user-defined function or supported builtin name,
  written without parentheses.
- The callback is called once per item and receives that item as its first
  argument.
- Callback return values are ignored.
- Iteration order is numbered-table order: `1`, `2`, `3`, ... through the
  table's current contiguous array length.

## Basic example

```lua
local highscores = {984, 145, 103}

foreach(highscores, print)
```

This is equivalent in intent to:

```lua
for score in all(highscores) do
  print(score)
end
```

It prints:

```text
984
145
103
```

## Custom callback

```lua
local highscores = {984, 145, 103}

function show_score(score)
  print(score)
end

function _draw()
  cls(0)
  cursor(8, 8, 7)
  foreach(highscores, show_score)
end
```

The callback receives one item at a time. A function may ignore the argument,
but a callback that needs more arguments receives `nil` for those not supplied,
following Mtile's ordinary function-call behavior.

```lua
function show_score(score, prefix)
  -- prefix is nil because foreach supplies only score.
  print(score)
end
```

Prefer callbacks designed with exactly one required parameter.

## Table expressions run once

The first argument does not have to be a variable:

```lua
function get_scores()
  print("loading")
  return {984, 145, 103}
end

foreach(get_scores(), print)
```

`get_scores()` is called once. The resulting table remains alive for the
entire loop, even if a callback allocates and triggers the Lua heap collector.
It is not reevaluated for each item.

A table literal is also valid:

```lua
foreach({1, 2, 3}, print)
```

However, a table literal allocates a new Lua table every time execution reaches
it. Do not put that form in `_update` or `_draw` unless creating a fresh table
is intentional. Reuse a table created during initialization instead:

```lua
local values = {1, 2, 3}

function _draw()
  foreach(values, print)
end
```

## Good use cases

### Apply a short operation to every entity

```lua
local enemies = {}

function update_enemy(enemy)
  enemy.x += enemy.vx
  enemy.y += enemy.vy
end

function _update()
  foreach(enemies, update_enemy)
end
```

This is useful when the operation already belongs in a named function and the
caller does not need the item index.

### Draw a list of records

```lua
local particles = {}

function draw_particle(particle)
  pset(particle.x, particle.y, particle.color)
end

function _draw()
  foreach(particles, draw_particle)
end
```

### Perform a one-time initialization pass

```lua
local actors = {player, guard, merchant}

function reset_actor(actor)
  actor.active = true
  actor.timer = 0
end

function _init()
  foreach(actors, reset_actor)
end
```

### Use a one-argument builtin

```lua
foreach(messages, print)
```

A builtin is valid only if its normal Mtile calling convention accepts one
argument. For example, `print` does; a builtin requiring two arguments does
not become a valid callback merely because it is named.

## When `foreach` is the wrong loop

### You need the index

`foreach` passes the value, not its numeric index. Use a numeric loop:

```lua
for i = 1, #highscores do
  print(i)
  print(highscores[i])
end
```

### You need keys and values

`foreach` visits only the numbered array sequence. It does not visit named or
hashed fields.

```lua
local scores = {
  pat = 984,
  sam = 145,
}

for name, score in pairs(scores) do
  print(name)
  print(score)
end
```

### You need `break`

A callback cannot break its caller's loop. Use `all()` explicitly:

```lua
for score in all(highscores) do
  if score < 100 then
    break
  end
  print(score)
end
```

### The loop body is clearer inline

Avoid creating a tiny callback solely to hide a simple, one-off body:

```lua
-- Less direct for a one-off operation:
function move_one(enemy)
  enemy.x += enemy.vx
end
foreach(enemies, move_one)

-- Often clearer:
for enemy in all(enemies) do
  enemy.x += enemy.vx
end
```

### It is a large per-frame hot loop

`foreach` performs a user-function call for every item. On the GBA, calls and
Lua `Value` operations are not free. For short object lists this is usually
irrelevant, but an inline `all()` or numeric loop avoids callback overhead in
a measured hot path.

The generic `all()` path checks the table's contiguous length as it iterates.
The compiler can hoist that length when it proves the loop cannot mutate the
array structure. A numeric loop makes a single captured upper bound explicit:

```lua
local n = #objects
for i = 1, n do
  update_object(objects[i])
end
```

Profile before replacing readable code, but prefer this form when a long list
is processed every 30 Hz tick.

## Avoid structural mutation during iteration

Changing fields inside an item is fine:

```lua
function age_particle(particle)
  particle.life -= 1
end

foreach(particles, age_particle)
```

Changing the structure of the table being iterated is harder to reason about.
For example, deleting an item shifts later values left while the loop index
still advances, so an item may be skipped:

```lua
local particles = {}

function remove_dead(particle)
  if particle.life <= 0 then
    del(particles, particle) -- may skip the item shifted into this position
  end
end

foreach(particles, remove_dead) -- avoid this pattern
```

Use a reverse numeric loop when deleting:

```lua
for i = #particles, 1, -1 do
  if particles[i].life <= 0 then
    deli(particles, i)
  end
end
```

Likewise, appending during `foreach` can extend the current contiguous length,
causing newly added items to be visited. A callback that continually appends
can therefore make the loop run far longer than expected. Treat the iterated
table's structure as stable until the pass finishes.

## Numbered-table and hole behavior

`foreach` follows the same contiguous-array semantics as `all()` and `#table`.
Iteration stops at the first `nil` slot.

```lua
local values = {}
values[1] = 10
values[3] = 30

foreach(values, print)
-- prints 10; the contiguous numbered sequence ends at missing index 2
```

Use dense `1..n` arrays for `foreach`. If sparse numeric keys or named keys are
part of the data model, use `pairs()` instead.

## Callback restrictions

Mtile deliberately does not implement closures or general first-class
function values. The second argument must therefore be a static function name:

```lua
foreach(items, draw_item) -- valid
foreach(items, print)     -- valid
```

The following forms are rejected:

```lua
foreach(items, draw_item())       -- calls it now; not a callback name
foreach(items, callbacks.draw)    -- dynamic field callback
foreach(items, function(x) end)   -- closures are unsupported
foreach(items)                    -- missing callback
```

A named callback can be declared before or after the `foreach` statement,
because the compiler discovers top-level functions before generating calls.
It may use globals and its own parameters/locals, but it cannot capture a
local variable from the calling function because that would require a closure.
Pass required state through the item or store intentional game-wide state in a
global.

Unknown callback names fail at build time:

```lua
foreach(items, misspelled_callback)
-- transpiler error: unsupported call "misspelled_callback"
```

## Statement-only syntax

`foreach` performs side effects and does not produce a value:

```lua
foreach(items, draw_item) -- valid statement
```

Do not use it as an expression:

```lua
result = foreach(items, draw_item) -- invalid
print(foreach(items, draw_item))   -- invalid
```

## Relationship to other loops

| Need | Recommended form |
| --- | --- |
| Call one named function for each numbered value | `foreach(t, f)` |
| Inline work for each numbered value | `for v in all(t) do ... end` |
| Index and value | `for i = 1, #t do local v = t[i] ... end` |
| Reverse traversal or deletion | `for i = #t, 1, -1 do ... end` |
| Named, hashed, or sparse keys | `for k, v in pairs(t) do ... end` |
| Early exit with `break` | explicit `all`, `pairs`, or numeric loop |

## How Mtile implements it

`foreach` is compiler syntax sugar, not a separate runtime iterator. The
parser lowers:

```lua
foreach(table_expression, callback)
```

into the equivalent internal structure of:

```lua
do
  local hidden_table = table_expression
  for hidden_value in all(hidden_table) do
    callback(hidden_value)
  end
end
```

The hidden names cannot collide with source identifiers. The generated loop
roots the table for garbage collection and does not allocate a helper object.
Any allocation comes from the table expression or callback itself—for example,
constructing `{1, 2, 3}` or concatenating strings—not from `foreach`.

## Complete example

```lua
local enemies = {
  {x = 32, y = 64, sprite = 4},
  {x = 80, y = 64, sprite = 4},
  {x = 128, y = 64, sprite = 5},
}

function update_enemy(enemy)
  enemy.x -= 1
  if enemy.x < -8 then
    enemy.x = 240
  end
end

function draw_enemy(enemy)
  spr(enemy.sprite, enemy.x, enemy.y)
end

function _update()
  foreach(enemies, update_enemy)
end

function _draw()
  cls(0)
  foreach(enemies, draw_enemy)
end
```

This is a good `foreach` use: the table is reused, its structure remains
stable during each pass, callbacks naturally take one item, and the caller
does not need indices or early exit.
