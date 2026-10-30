# gio-ide

> [!WARNING]
> This project is in alpha testing.

gio-ide is a Linux desktop Go code editor. It is written in Go with [Gio](https://gioui.org). Code intelligence is an in-process fork of gopls. Syntax color comes from tree-sitter. Git comes from go-git. There is no plugin system.

## Requirements

- Linux amd64
- Go 1.27.2 (the `go` line in `go.mod`)
- `CGO_ENABLED=1`
- A C compiler and the Gio system libraries (Wayland, X11, GLES, Vulkan). On a Debian-family machine:

```bash
sudo apt install gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
  libxkbcommon-dev libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev \
  libffi-dev libxcursor-dev libvulkan-dev
```

- The `go` command on `PATH` at run time, for code intelligence. go fix and goimports on save use it too. The editor still saves the file if `go` is missing.

## Run

```bash
export CGO_ENABLED=1
go run ./cmd/gio-ide /path/to/module
```

`go run ./cmd/gio-ide -version` prints the version. A build that does not set `-X main.version` prints `dev`.

Settings, the keymap, and themes live under `$XDG_CONFIG_HOME/gio-ide` (default `~/.config/gio-ide`). A project file is `<root>/.gio-ide/settings.toml`.

Scratch files live in `$XDG_STATE_HOME/gio-ide/scratches` (default `~/.local/state/gio-ide/scratches`) and appear under Scratches in the project tree.

The user manual is [docs/manual.md](docs/manual.md). It describes the editor as it works today.

The Unlicense covers the gio-ide editor. See [LICENSE](LICENSE). Code from other projects keeps its own license. See [NOTICE.md](NOTICE.md).

## Source

The module path is `github.com/russoj88/gio-ide`.

```bash
git clone https://github.com/russoj88/gio-ide.git
```

## Build

```bash
CGO_ENABLED=1 go build -trimpath -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o bin/gio-ide ./cmd/gio-ide
go test ./...
go vet ./...
go run ./cmd/goplssync lint --dir internal/gopls
./cmd/package-debian.sh 0.1.0
```

`./cmd/package-debian.sh` writes `dist/gio-ide_<version>_<arch>.deb`, a standalone `dist/gio-ide_<version>_linux_<goarch>` binary, and `dist/SHA256SUMS`. The `.deb` Depends on the shared libraries `ldd` reports, and suggests `golang-go`. With no release tag, `packaging/build-deb.sh` uses Version `0.1.0+git.<shortsha>` so `dpkg` accepts it.

`packaging/build-appimage.sh` and `packaging/build-tarball.sh` are optional local packagers. They are not part of CI.

gio-ide does not shell out to `git`. Git status and commits use go-git. Debugging, refactoring, and the test runner are not in v1. Save can run `go fix`, embedded goimports, and embedded gofumpt on `.go` files when `editor.gofix_on_save`, `editor.goimports_on_save`, or `editor.gofumpt_on_save` is set. All three default off.

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs format, tidy, `go test -race`, vet, and build. Tests skip `internal/gopls` and `internal/thirdparty`. A push to `main` publishes the `.deb` on the GitHub Release `v0.0.0`. The Debian version is `0.0.0`. Each push replaces that release. This tag stays until the project uses real versions.

## Release check

Run these on X11 and on Wayland before a release:

1. Open a repository with about 10,000 Go files. The tree appears while gopls loads. Typing stays smooth. Go to file responds as you type.
2. Switch buffers with Alt+Left and Alt+Right. Open a split. Make a multi-cursor edit of 5 lines, and undo it in one step.
3. Find and replace in a file and in the project.
4. Hover, completion, go to declaration, back, find usages, diagnostics, and the Problems panel.
5. Terminal: bash, `git log` with a pager, vim, panel resize, copy and paste.
6. Git: edit a file and see gutter marks. Commit. The commit stages every changed file. Create and switch a branch.
7. Commit in a repo with a pre-commit hook. The notice appears.
8. Remap a key in keymap.toml. It applies without a restart. Switch between dark and light themes.
9. Run intel.restart. The state goes to Restarting, then Ready. Diagnostics return.
10. Remove `go` from PATH. The editor still works. A banner explains why code intelligence is off.
11. Watch the heap in the status bar during the 10,000-file load. The UI stays responsive.
