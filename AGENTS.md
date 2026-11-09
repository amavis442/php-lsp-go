# AGENTS.md

PHP language server written in Go, used from Neovim. Clean-room implementation: never decompile or
copy Intelephense (closed source); only reproduce behaviour.

Plans, design, decisions and status live in the agent vault, not in this repo:
`D:\Noot\Claude\projects\php-lsp-go\` (`noot -A context php-lsp-go`).

## Rules

- TDD: failing test first. Unit, integration and end-to-end (LSP over stdio) tests are mandatory.
- Feature-first clean architecture under `internal/`: `features/<feature>/{domain,data,presentation}`,
  shared code in `internal/core/`. Domain packages import only the standard library and pure core
  packages. Only `internal/core/syntax` uses the parser library; only `internal/server` and
  `*/presentation` use the LSP protocol library.
- Pure Go: no cgo. The project must build with `CGO_ENABLED=0`.
- Code, comments and commits in English. Commit as `Amavis442 <patteun8@gmail.com>`, no
  Co-Authored-By trailers, never `--no-verify`.
- Work on `feat/*` / `fix/*` branches. Never push `main` or tags.
- On Windows use PowerShell, not bash.

## Commands

```powershell
$env:CGO_ENABLED = '0'
gofmt -l .
go vet ./...
go test ./...
go build -o php-lsp-go.exe ./cmd/php-lsp-go
```
