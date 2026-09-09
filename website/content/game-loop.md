# The game loop

A game keeps doing two things: changing its state, then showing that state.
mTile gives you three functions to organize that work.

## Set up once with `_init()`

Use `_init()` to prepare the game when the cartridge starts. For example, reset
the score or start a song. Variables can also be initialized at the top of the
file. Keep a variable's type consistent throughout the game.

```lua
x = 112
score = 0

function _init()
  x = 112
  score = 0
end
```

## Change things with `_update()`

Read input, move objects, check collisions, and update scores here. This is
the part of the loop that decides what happens next.

```lua
function _update()
  if btn(0) then x -= 2 end
  if btn(1) then x += 2 end
  x = mid(0, x, 232)
end
```

`btn(0)` is left and `btn(1)` is right. The `mid()` call keeps an 8-pixel-wide
sprite inside the 240-pixel-wide screen.

## Show it with `_draw()`

Clear the screen, draw the world, then draw the player and interface. Drawing
should show the state that `_update()` already calculated.

```lua
function _draw()
  cls(24)
  spr(1, x, 72)
  print("LEFT AND RIGHT TO MOVE", 8, 8, 15)
end
```

These three blocks form one small `game.lua`. Draw something in sprite **1**
in the Sprites workspace, save, and select Build + Reload. The explorer artwork
in [Coin Quest](../../internal/build/templates/coin-quest/README.md) is ready to use.

## How often does it run?

`_init()` runs once. `_update()` and `_draw()` target one update every two GBA
display frames, or about **30 times per second**. At that rate, moving two pixels
per update moves about 60 pixels in a second. Expensive frames can miss the
budget; the rate is a target, not a guarantee.

There is no `_update60()` callback. Do not call the lifecycle functions yourself;
the generated game loop owns them. Put reusable behavior in named helpers such
as `move_player()` or `restart()` instead.

## A few habits that help

- Initialize your variables before you use them.
- Update the score in `_update()`, so drawing it cannot award extra points.
- Use one `cls()` at the start of `_draw()` when you want AUTO to consider
  hardware rendering.
- Add one behavior, build, and check it before adding the next.

Next, [make your first game](../../internal/build/templates/coin-quest/README.md)
or read the [language reference](../../docs/reference.md#lua-dialect).
