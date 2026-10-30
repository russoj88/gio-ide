# Fork notice

This tree is a fork of the `gopls` and `golang.org/x/tools/internal` packages
from the Go tools repository.

- Origin: https://go.googlesource.com/tools
- Tag: `gopls/v0.23.0`
- Commit: `014f87ff5c01915bc90f4f11a6bb8aea3e0edbd7`
- Module: `golang.org/x/tools` `v0.47.1-0.20260707181000-a299dadba899` (copy)
- Build floor: `golang.org/x/tools` `v0.49.0` (see `cmd/goplssync` `keepAtLeast`)

Import rewrite, applied after copy:

- `golang.org/x/tools/gopls/internal/<pkg>` becomes `github.com/russoj88/gio-ide/internal/gopls/<pkg>`
- `golang.org/x/tools/internal/<pkg>` becomes `github.com/russoj88/gio-ide/internal/gopls/xtools/<pkg>`

`golang.org/x/tools/go/...`, `golang.org/x/tools/imports`, and
`golang.org/x/tools/refactor/...` stay on the public module.

Local changes are the patches in `patches/`. See `PATCHES.md`.
The `guard` package is added by gio-ide so a panic in a forked goroutine
cannot end the editor.

Files named in `//go:build ignore` (code generators) are not copied.
They are not part of the compiled packages.
