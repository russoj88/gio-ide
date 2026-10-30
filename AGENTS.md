# AGENTS.md

Instructions for coding agents working in this repository.

This is a Linux desktop Go code editor (gio-ide). The UI is Gio. Code intelligence is an in-process fork of gopls. Syntax color is tree-sitter. Git is go-git. There is no plugin system. Linux is the only supported OS. Do not add Windows or macOS packaging or CI for other kernels.

The product definition is [README.md](README.md). If README and this file disagree on *what to build*, README wins. If they disagree on *how to write the code*, this file wins.

If a request is ambiguous, would add a dependency, or would add a package that is not needed yet, ask before doing it. Prefer the smallest change that works.

## Principles

These override convenience and "industry standard" project layouts.

1. **Simplicity.** Solve the current problem with the fewest types, packages, and abstractions. Do not add interfaces, managers, services, or config layers until a second use appears.
2. **Readability.** Write code a person can follow top to bottom. Names should say what the value is. Control flow should be obvious. Avoid cleverness, hidden side effects, and "magic" helpers that save three lines.
3. **Standard library first.** Before adding a module, check whether the stdlib already does it (`os`, `path/filepath`, `context`, `errors`, `log`, `testing`, `testing/synctest`, `time`, `sync`).
4. **Document well.** Every exported identifier gets a godoc comment. Packages get a package comment. Comments explain intent, invariants, and non-obvious constraints — never restate the next line of code.
5. **Separate domain from Gio.** `internal/buffer`, `internal/bus`, `internal/commands`, `internal/format`, `internal/git`, `internal/keymap`, `internal/search`, `internal/settings`, `internal/syntax`, `internal/theme`, `internal/workspace`, `internal/xdg`, and `internal/runtimemem` must **not** import `gioui.org`. UI packages call those functions and draw the results.

## Stack

- **Language:** Go. Always use the latest available version of Go. Put that version in `go.mod` (currently `go 1.27.2`). If the latest version is not installed locally, wait until it is and tell the user — do not proceed with an older toolchain.
- **Module path:** `github.com/russoj88/gio-ide`.
- **UI:** Gio (`gioui.org`). `CGO_ENABLED=1`. The window loop stays on the goroutine that runs `app.Run`. Do not call Gio window APIs from worker goroutines.
- **Code intelligence:** the forked gopls tree in `internal/gopls` (see `internal/gopls/UPSTREAM`). The editor calls it in-process. Do not shell out to a `gopls` binary. The fork was copied from the `xtools` pseudo-version in `UPSTREAM`. The module requirement may be newer: `cmd/goplssync` `keepAtLeast` holds `golang.org/x/tools` at v0.49.0 so the security floors for `x/crypto`, `x/image`, `x/mod`, `x/net`, and `x/text` are not downgraded. Do not take `mvdan.cc/gofumpt` v0.12.0 as a module requirement. `honnef.co/go/tools` and `golang.org/x/vuln` stay on the versions that fork was copied against, because staticcheck and vulncheck share `go/analysis` types with that `x/tools`. `golang.org/x/telemetry` follows the tools module. The `gofumpt` *binary* used to format this repo is `mvdan.cc/gofumpt@v0.12.0`, which is not that module.
- **Format on save:** The editor formats on save with source copies under `internal/thirdparty`, not with the v0.10.0 module and not by execing gofumpt or goimports. The gofumpt copy is v0.12.0. The goimports copy is `golang.org/x/tools` v0.50.0. The `mvdan.cc/gofumpt` module requirement stays v0.10.0 for the fork. Refresh the copies with `scripts/sync-formatters.sh` (does not commit). goimports still runs the `go` command. `editor.gofix_on_save` runs `go fix -diff` on the saved file through an overlay and does not write other files in the package. Do not gofumpt `internal/thirdparty`. CI excludes it.
- **Syntax:** `github.com/tree-sitter/go-tree-sitter` and `github.com/tree-sitter/tree-sitter-go` at a matching grammar ABI.
- **Git:** `github.com/go-git/go-git/v5`. The editor does not shell out to `git` for status or commits.
- **Config:** TOML under `$XDG_CONFIG_HOME/gio-ide` and `<root>/.gio-ide/settings.toml`. No Cobra, Viper, or flags library beyond the standard `flag` package in `main`.
- **CI:** GitHub Actions in `.github/workflows/ci.yml`. Pull requests and pushes run format, tidy, `go test -race`, vet, and build. The format check and the test run skip `internal/gopls` and `internal/thirdparty`. Vet and build still include those trees. A push to `main` packages the `.deb` and publishes a GitHub Release. Do not add a Docker deploy workflow. Do not add Windows or macOS packaging jobs.
- **Packaging:** `cmd/package-debian.sh <version> [outdir]`. It builds the binary, writes a `.deb`, a standalone Linux binary, and `SHA256SUMS`.

## Repository layout

```
.
├── AGENTS.md
├── README.md
├── go.mod
├── .github/workflows/ci.yml # GitHub Actions
├── cmd/gio-ide/            # main: flags, version, open a directory
├── cmd/goplssync/          # copy, rewrite, patch, and lint the fork
├── cmd/package-debian.sh   # .deb + linux binary for GitHub Releases
├── docs/                   # user manual
├── packaging/              # desktop file, icon script, AppImage, tarball
├── scripts/sync-formatters.sh  # refresh embedded formatters; does not commit
├── scripts/sync-gopls.sh   # refresh the fork from a gopls tag; does not commit
├── testdata/fixture/       # tiny module the intel tests open
└── internal/
    ├── app/                # Gio shell, window, commands wiring
    ├── editor/             # text view
    ├── ui/                 # shared Gio widgets
    ├── terminal/           # terminal widget and PTY session
    ├── intel/              # in-process gopls host (no gioui.org)
    ├── buffer/             # text storage (no gioui.org)
    ├── bus/                # in-process event bus (no gioui.org)
    ├── commands/           # command registry (no gioui.org)
    ├── format/             # go fix, goimports, and gofumpt (no gioui.org)
    ├── git/                # go-git status and commits (no gioui.org)
    ├── harness/            # headless Gio frames for tests
    ├── keymap/             # key bindings (no gioui.org)
    ├── search/             # find in file and project (no gioui.org)
    ├── settings/           # TOML settings (no gioui.org)
    ├── syntax/             # tree-sitter highlighting (no gioui.org)
    ├── theme/              # color maps (no gioui.org)
    ├── workspace/          # file index (no gioui.org)
    ├── xdg/                # config and state paths (no gioui.org)
    ├── runtimemem/         # memory limit (no gioui.org)
    └── gopls/              # forked gopls; edit only through the sync tool
```

Rules:

- `cmd/gio-ide` parses flags and calls `app.Run`. It does not contain editor logic.
- Domain packages expose plain structs, functions, and errors.
- Follow [Organizing a Go module](https://go.dev/doc/modules/layout). Commands live in `cmd/<name>`. Code that other modules must not import lives in `internal/`. Do not add `pkg/`, `mvc`, `services`, or a `utils` package.
- Do not add a Makefile, Taskfile, Compose file, or Dockerfile unless the user asks.
- Do not hand-edit `internal/gopls` except by changing `cmd/goplssync` and the patches under `internal/gopls/patches`, then running `scripts/sync-gopls.sh`. The patches must keep the editor process alive (`os.Exit` and `log.Fatal` stay out of the fork).

## Commands

```sh
CGO_ENABLED=1 go run ./cmd/gio-ide
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./...
go vet ./...
go fix ./...
gofumpt -w .
go mod tidy
go run ./cmd/goplssync lint --dir internal/gopls
./cmd/package-debian.sh 0.1.0
```

`gofumpt` (`mvdan.cc/gofumpt@v0.12.0`) is the formatter for everything except `internal/gopls` and `internal/thirdparty`. Do not use `gofmt` or `go fmt`. Do not add a Makefile. CI skips those trees in the format check so the fork can stay formatted as upstream shipped it. Format the fork only when a sync intentionally does so. Do not gofumpt `internal/thirdparty`.

`GIO_IDE_WATCHDOG=1` logs slow-frame stacks. `GIO_IDE_UPDATE_GOLDEN=1` rewrites syntax goldens.

## Go style

Follow [Effective Go](https://go.dev/doc/effective_go), [Package names](https://go.dev/blog/package-names), and `gofumpt`. Project-specific:

- **gofumpt is the format** (`mvdan.cc/gofumpt@v0.12.0`). Tabs, gofumpt grouping of imports (stdlib, then third party). Do not use `gofmt` or `go fmt`. Do not reformat `internal/gopls` unless you are refreshing the fork. Do not gofumpt `internal/thirdparty`.
- **Errors are values.** Return `error`. Messages must be explanatory and helpful. Wrap the cause with `%w` whenever there is one. Do not panic in library or UI code except for programmer invariants. Do not use `_ = err`.
- **No new `init()`.** Wire the window in `main` and `app.Run`. The piece table's byte-order sentinel and the gopls fork already use `init`; do not add more.
- **No globals** for app state. Pass config, the bus, and clients as arguments.
- **Interfaces** belong at the consumer, and only when there are two implementations or a test double that cannot be a concrete type.
- **Context** is the first parameter of functions that do I/O or can be cancelled.
- **Logging:** the standard `log` package, with a `gio-ide:` prefix on process messages. Do not add a logging framework.
- **Tables in tests.** Use `testing` and table-driven tests. Do not add testify, ginkgo, or gomock.
- **No sleep in tests.** Do not `time.Sleep` to wait for timers, TTLs, or other goroutines. Use `testing/synctest` (`synctest.Test`) so time is fake and the test is deterministic. Use `synctest.Wait` when you need other goroutines in the bubble to idle. `time.Sleep` is allowed only inside a synctest bubble (it advances the fake clock). A test that waits on a real process (the `go` command, a PTY) waits on that process's output, not on a sleep loop, when the API allows it.
- **Don't stutter.** `settings.Settings` not `settings.SettingsConfig`.
- **Package names.** Short, lower case, no underscores. The last element of the import path is the package name. Do not use `util`, `common`, or `misc`. Do not use a name the caller wants for a variable, such as `db` or `buf`.
- **Don't export** what nothing outside the package needs.

## Dependencies

Allowed without discussion, at versions already required by `go.mod`:

- `gioui.org`
- `github.com/creack/pty`
- `github.com/danielgatis/go-vte`
- `github.com/fatih/gomodifytags`
- `github.com/fsnotify/fsnotify`
- `github.com/go-git/go-git/v5` and `github.com/go-git/go-billy/v5`
- `github.com/google/go-cmp` (tests)
- `github.com/pelletier/go-toml/v2`
- `github.com/sergi/go-diff`
- `github.com/tree-sitter/go-tree-sitter` and `github.com/tree-sitter/tree-sitter-go`
- `golang.org/x/image`, `golang.org/x/mod`, `golang.org/x/sync`, `golang.org/x/text`, `golang.org/x/tools`, `golang.org/x/telemetry`, `golang.org/x/vuln`
- `honnef.co/go/tools` and `mvdan.cc/gofumpt` (fork pins; see Stack)
- `mvdan.cc/xurls/v2`
- Go standard library

Ask (or justify in the PR) before adding anything else. In particular **do not** add Cobra, Viper, testify, an LSP-over-stdio client, Monaco, or a second UI toolkit.

When a third-party module is justified, pick one that is small, maintained, and does one job. Run `go mod tidy`. Do not vendor unless the user asks. Do not move `golang.org/x/tools` below the `keepAtLeast` floor in `cmd/goplssync`. The fork copy commit stays in `UPSTREAM`.

## What not to do

- Do not import `gioui.org` from the domain packages listed above.
- Do not add a Makefile, a Docker deploy workflow, or CI for other kernels.
- Do not commit `bin/`, `dist/`, or generated license dumps.
- Do not commit binaries or secrets.
- Do not add dependencies, files, or packages "for later".
- Do not call `os.Exit` from the gopls fork.

## When you are done

- `go fix ./...` then `gofumpt -w` the files you touched outside `internal/gopls` and `internal/thirdparty`.
- `go test ./...` and `go vet ./...` must pass.
- `go run ./cmd/goplssync lint --dir internal/gopls` must pass when the fork was touched.
- New exported API should have godoc. New packages should have a package comment.
- `go.mod` / `go.sum` should only change when dependencies actually changed.
