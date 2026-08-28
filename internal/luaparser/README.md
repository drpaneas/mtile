# Mtile Lua parser

Pratt parser for the Lua dialect Mtile compiles. This is not a vendored
GopherLua fork. Lua 5.2 details (long-string first-newline skip, leading-dot
decimals, hex/binary fractions) follow GopherLua `v1.1.1`
(`1388221efeb4a239a053e5932c3d755699055684`) as a behavior baseline.

Language extensions live in the AST instead of being rewritten into ordinary
Lua source before parsing.

Supported extensions:

- `?expr` print shorthand (a `print(...)` call at line start)
- `!=` inequality
- compound assignment, including table/field l-values
- binary integer literals (`0b...`)
- integer division (`\\`)
- bitwise operators `&`, `|`, `^^`, `~`, shifts `<<`, `>>`, logical shift
  `>>>`, and rotates `<<>` / `>><`
- single-line `if (condition) statement`
- `foreach(table, function_name)` statement sugar, lowered to `for value in all(table)`
- the Mtile expression grammar and ordinary Lua control-flow statements

Compound assignment to an indexed or field l-value is lowered immediately.
The base and key are evaluated once into synthetic locals before the
read/modify/write sequence, so `items[next_key()] += amount()` cannot call
either side twice.

The parser rejects closures, `_update60`, and `string.format`. `foreach`
requires exactly a table expression and a static callback name; dynamic
callbacks and callback calls such as `foreach(t, draw())` are rejected.
