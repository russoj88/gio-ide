# Fork patches

Applied in filename order after the import rewrite. `git apply --3way` from the repository root.

| ID | Upstream file | Change | Reason |
|---|---|---|---|
| P1 | gopls/internal/server/general.go | `(*server).Exit` returns an error instead of `os.Exit(1)` | Exit must never end the editor. gio-ide never calls Exit. |
| P2 | gopls/internal/filecache/filecache.go | `GetOrFatal` reports `bug.Reportf` and returns a cache miss | A cache fault must not end the editor. |
| P3 | golang.org/x/tools/internal/packagesinternal | `init` sets `TypecheckCgo` (4096) and `DepsErrors` (1024) | `go/packages` sets these in its own copy. The fork copy never receives them. `GetDepsErrors` stays nil. |
| P4 | every copied non-test file with a `go` statement | `go f()` becomes `guard.Go(func() { f() })` | A caller's `recover` does not catch panics in other goroutines. |
| P5 | gopls/internal/cache/cache.go | the 100 MB `ballast` variable is removed | gio-ide uses a memory limit instead. |
| P6 | remaining non-test `os.Exit` / `log.Fatal` sites | replaced with `panic` | `lint-fork` forbids process-ending calls. Init-time checks in `settings/analysis.go` and `settings/staticcheck.go` stay. |
| P7 | golang.org/x/tools/internal/typesinternal/classify_call.go | `UsedIdent` and `interfaceMethod` are local functions | `golang.org/x/tools` v0.49 removed the `go:linkname` targets in `go/types/typeutil`. |
| P8 | gopls/internal/settings/analysis.go | drop the extra `scannererr` and `sqlrowserr` entries | `vet.Suite` in `golang.org/x/tools` v0.49 already contains both analyzers. A second entry stops the process. |

## Fork-time checks

P3. `packagesinternal.GetDepsErrors` is left at its default and returns nil.
`cache/load.go` assigns that value to `metadata.Package.DepsErrors`, so the field is always nil.
What is lost: the dependency package's `ImportStack`, `Pos`, and `Err` (`depsErrors` on `go/packages.Package`).
`Package.Errors` is still copied, so ordinary package errors remain.
The fixture diagnostic test still reports a type error in the edited file.
The loss matters for richer dependency-error diagnostics, not for a type error in the open file.

P4. In the seam closure (`server`, `cache`, `settings`, `protocol` and the packages they import) the unpatched source has 43 `go` statements in 30 non-test files.
Test helpers copied so the cache session tests compile (`testenv`, `jsonrpc2/servertest`, `test/integration/fake`) add 4 statements in 3 files.
Those are guarded too. `internal/gopls/guard` itself uses `go` and is exempt from `lint-fork`.
