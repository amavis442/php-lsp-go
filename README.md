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
