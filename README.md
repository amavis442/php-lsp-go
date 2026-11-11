# php-lsp-go

A PHP language server written in Go, built for Neovim. It is a clean-room
implementation in pure Go (no cgo) and parses PHP with
[gotreesitter](https://github.com/odvcencio/gotreesitter) and the
tree-sitter-php grammar.

## Features

- Document synchronisation (incremental)
- Syntax diagnostics
- Document symbols (outline)
- Workspace symbols, from an index of the whole project including `vendor/`
  and the functions, classes and constants built into PHP
- Goto-definition for classes, functions, constants, `use` statements and
  members reached through a class name, `self`, `static`, `parent` or
  `$this`, following traits, parent classes and interfaces. Members of other
  objects (`$user->name()`) need type inference and are not resolved yet.
- Hover for the same names: the declaration's signature, the class or
  namespace it belongs to, and its PHPDoc summary.

Built-in declarations follow the latest PHP version; choosing the project's
PHP version is not supported yet.

## Build and test

```sh
CGO_ENABLED=0 go build -o php-lsp-go ./cmd/php-lsp-go
go vet ./...
go test ./...
```

On Windows, build `php-lsp-go.exe` instead.

The server speaks LSP over stdin and stdout.

## Use with Neovim

Neovim 0.11 or later. Put the binary on your `PATH` (or use its full path in
`cmd`), then in `init.lua`:

```lua
vim.lsp.config('php_lsp_go', {
  cmd = { 'php-lsp-go' },
  filetypes = { 'php' },
  root_markers = { 'composer.json', '.git' },
})
vim.lsp.enable('php_lsp_go')
-- When trying it next to another PHP server, disable that one to avoid
-- duplicate results, for example: vim.lsp.enable('intelephense', false)
```

Open a PHP file in the project and the workspace is indexed in the
background; Neovim shows the progress. Then `gd` jumps to a definition, `K`
shows hover, and `:lua vim.lsp.buf.workspace_symbol()` searches the
workspace.

The first start parses every PHP file, including `vendor/`. The result is kept
in the user cache directory (`php-lsp-go/index`), so later starts only parse
what changed. A new build of the server starts afresh.

To see what the server does: `:checkhealth vim.lsp`, and the LSP log
(`:lua vim.cmd.edit(vim.lsp.log.get_filename())`) after
`:lua vim.lsp.log.set_level('debug')`. The server writes its own problems to
stderr, which ends up in that log.

## Credits

Written by Amavis442, pair-programmed with Claude (Anthropic). See
[CONTRIBUTORS.md](CONTRIBUTORS.md).

## License

[MIT](LICENSE).

The declarations of PHP's built-in functions, classes and constants come from
[JetBrains phpstorm-stubs](https://github.com/JetBrains/phpstorm-stubs),
licensed under Apache-2.0 (see
[third_party/phpstorm-stubs/LICENSE](third_party/phpstorm-stubs/LICENSE)).
They are embedded in the binary and written to the user cache directory
(`php-lsp-go/stubs/<version>`) on first use, so the editor can open them.
