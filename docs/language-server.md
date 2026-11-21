# Lua language server

`mtile lsp` provides builtin completion, hover documentation, and live compiler
diagnostics for mTile's Lua dialect. It runs independently of the native editor
and uses the [`mtile` command included in the desktop download](getting-started.md#use-mtile-in-a-terminal).

Configure your editor's LSP client to start that executable with the argument
`lsp`. The connection uses standard LSP JSON-RPC framing over stdin/stdout;
command errors and help go to stderr. No TCP port or running mTile GUI is needed.

```text
mtile lsp [project-dir]
```

An explicit project directory takes precedence. Otherwise the server uses the
client's first workspace folder or root URI. Without a workspace root, opening
a Lua file selects the nearest ancestor containing `project.json`, or the file's
directory when there is no project file. Start one server per project.

## Neovim

With Neovim 0.11 or later, add this to your configuration, using the installed
binary or its absolute path:

```lua
vim.lsp.config('mtile', {
  cmd = { '/absolute/path/to/mtile', 'lsp' },
  filetypes = { 'lua' },
  root_markers = { 'project.json' },
})
vim.lsp.enable('mtile')
```

Open a project's Lua file. Use `K` for hover help and insert-mode
`Ctrl-X Ctrl-O` for builtin completion. Diagnostics appear through Neovim's
normal diagnostic UI. See the [Neovim LSP guide](https://neovim.io/doc/user/lsp.html)
for completion UI and diagnostic display preferences.

For VS Code, a language-client extension must launch the same command with the
project directory as its workspace root. VS Code's built-in Lua highlighting
alone does not connect to a language server; this release ships the server,
without a dedicated VS Code extension. Other stdio LSP clients can configure
the command directly.

Use mTile's server for mTile projects. A standard Lua server does not understand
all of mTile's operators, shorthand syntax, or compiler restrictions and may
report conflicting errors.

## What the editor reports

- Completion lists supported builtins by prefix. Hover includes the signature,
  usage notes, an example, and the API's renderer classification, using the same
  reference as the website and the compiler's exhaustive renderer contract.
- Syntax checks accept mTile's dialect. Completion and hover remain available
  while later portions of the buffer are incomplete. Comments, strings, and
  table members are excluded from builtin help.
- Project checks follow the entry point and static `require()` graph, including
  open unsaved buffers and new unsaved modules. They validate assets and run
  the same transpiler and renderer checks as a build, without writing generated
  code, saving files, discovering TinyGo, or linking a ROM.
- A forced Mode 0 failure is an error. An AUTO fallback is informational: the
  project can still build with Mode 4. Stable codes such as
  `M0_MODE4_BUILTIN` and the original module locations match build diagnostics.
  Clients choose how informational diagnostics appear; they may use an icon
  or Problems entry instead of a squiggle.
- The compiler reports its first blocking error or fallback reason. Fixing it
  can reveal the next restriction. The server does not invent additional
  eligibility rules or relax the [renderer contract](render-modes.md).

Analysis is debounced by 200 ms and runs in a background worker. Edits clear
published diagnostics and invalidate older in-flight results. Syntax, asset,
and transpiler failures suppress renderer conclusions until the project can
be checked again. LSP ranges use UTF-16 coordinates, with conversion from the
compiler's byte-based source positions.

Assets and `project.json` are read from disk; unsaved overlays apply to Lua
sources. Clients supporting dynamic file-watch registration receive watchers
for Lua, project settings, and assets, and changes trigger reanalysis. Other
clients recheck on Lua edits, saves, and explicit watched-file notifications.
Linker memory usage and measured runtime performance still require a build.

The native Develop workspace consumes the same project checker. Its Problems
panel shows live renderer selection, informational fallbacks, and compiler
errors after a short editing pause. Clicking Problems opens the first
diagnostic's source. Long messages wrap and scroll; drag to select and use
Cmd/Ctrl+C, or **COPY ALL**, to copy the complete original diagnostic.
Asset checks use the last saved assets, refreshed when the editor saves them.

This first version supplies builtin help and compiler diagnostics. Local-symbol
completion, go-to-definition, rename, formatting, and debugging are outside its
scope.
