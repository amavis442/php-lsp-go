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

Goto-definition and hover are planned.

## Build and test

```sh
CGO_ENABLED=0 go build -o php-lsp-go ./cmd/php-lsp-go
go vet ./...
go test ./...
```

On Windows, build `php-lsp-go.exe` instead.

The server speaks LSP over stdin and stdout.

## Credits

Written by Amavis442, pair-programmed with Claude (Anthropic). See
[CONTRIBUTORS.md](CONTRIBUTORS.md).

## License

[MIT](LICENSE).
