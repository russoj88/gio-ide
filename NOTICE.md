# Third-party notices

The Unlicense covers the gio-ide editor. See `LICENSE` at the repository root.

gio-ide also contains source from other projects. Those parts keep their own licenses.

- The gopls fork in `internal/gopls` uses the BSD-3-Clause license. See `LICENSE`, `PATENTS`, and `NOTICE.md` in that directory.
- Embedded gofumpt v0.12.0 is in `internal/thirdparty/gofumpt`. See `LICENSE` and `LICENSE.google`.
- Embedded goimports from `golang.org/x/tools` v0.50.0 is in `internal/thirdparty/goimports`. See `LICENSE`.
- The Go highlight query in `internal/syntax/queries/highlights.scm` uses the MIT license. The notice is in that file.

`packaging/licenses.sh` writes the license text of each Go module into `THIRD_PARTY_LICENSES` when a package is built. Git ignores that generated file.
