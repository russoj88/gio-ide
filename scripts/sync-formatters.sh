#!/usr/bin/env bash
# Sync embedded formatters into internal/thirdparty. Does not commit.
#
# Versions copied from the module cache (this script does not run go get):
#   gofumpt v0.12.0
#   goimports from golang.org/x/tools v0.50.0
#   govendor snapshot go1.27.0 (gofumpt internal/govendor/version.txt)
#
# CI excludes internal/thirdparty from gofumpt. Do not reformat this tree;
# upstream formatting must stay. The script deletes and recopies both trees,
# rewrites import paths, and reapplies the ProcessEnv.Context patch.
set -euo pipefail

cd "$(dirname "$0")/.."

MODCACHE="$(go env GOMODCACHE)"
if [[ -z "$MODCACHE" ]]; then
	echo "sync-formatters: go env GOMODCACHE is empty" >&2
	exit 1
fi
GOFUMPT_MOD="$MODCACHE/mvdan.cc/gofumpt@v0.12.0"
XTOOLS_MOD="$MODCACHE/golang.org/x/tools@v0.50.0"
MOD="github.com/russoj88/gio-ide"
DST_GOFUMPT="internal/thirdparty/gofumpt"
DST_GOIMPORTS="internal/thirdparty/goimports"

if [[ ! -d "$GOFUMPT_MOD" ]]; then
	echo "sync-formatters: missing gofumpt module cache: $GOFUMPT_MOD" >&2
	echo "sync-formatters: download it with: go mod download mvdan.cc/gofumpt@v0.12.0" >&2
	exit 1
fi
if [[ ! -d "$XTOOLS_MOD" ]]; then
	echo "sync-formatters: missing x/tools module cache: $XTOOLS_MOD" >&2
	echo "sync-formatters: download it with: go mod download golang.org/x/tools@v0.50.0" >&2
	exit 1
fi

copy_file() {
	local src="$1" dst="$2"
	if [[ ! -f "$src" ]]; then
		echo "sync-formatters: missing source file: $src" >&2
		exit 1
	fi
	mkdir -p "$(dirname "$dst")"
	cp "$src" "$dst"
	chmod u+w "$dst"
}

rm -rf "$DST_GOFUMPT" "$DST_GOIMPORTS"
mkdir -p internal/thirdparty

# gofumpt: format, version, and the govendor snapshot. No tests, no testdata,
# no gen_govendor.go, no package-main files.
for rel in \
	format/format.go \
	format/rewrite.go \
	format/simplify.go \
	internal/version/version.go
do
	copy_file "$GOFUMPT_MOD/$rel" "$DST_GOFUMPT/$rel"
done
copy_file "$GOFUMPT_MOD/LICENSE" "$DST_GOFUMPT/LICENSE"
copy_file "$GOFUMPT_MOD/LICENSE.google" "$DST_GOFUMPT/LICENSE.google"

while IFS= read -r -d '' src; do
	rel="${src#"$GOFUMPT_MOD"/}"
	base="$(basename "$src")"
	if [[ "$base" == *_test.go ]]; then
		continue
	fi
	copy_file "$src" "$DST_GOFUMPT/$rel"
done < <(find "$GOFUMPT_MOD/internal/govendor" -type f \( -name '*.go' -o -name 'version.txt' \) -print0)

# goimports: the non-test files goimports needs to fix imports. No testdata,
# no generators, no package main, no event/export.
# The leading internal/ is dropped. A nested internal/ directory would be
# invisible to internal/format, because Go only allows imports from the
# parent of that internal directory.
for rel in \
	internal/imports/fix.go \
	internal/imports/imports.go \
	internal/imports/mod.go \
	internal/imports/mod_cache.go \
	internal/imports/sortimports.go \
	internal/imports/source.go \
	internal/imports/source_env.go \
	internal/gocommand/invoke.go \
	internal/gocommand/invoke_unix.go \
	internal/gocommand/invoke_notunix.go \
	internal/gocommand/vendor.go \
	internal/gocommand/version.go \
	internal/gopathwalk/walk.go \
	internal/stdlib/stdlib.go \
	internal/stdlib/import.go \
	internal/stdlib/deps.go \
	internal/stdlib/manifest.go \
	internal/modindex/modindex.go \
	internal/modindex/directories.go \
	internal/modindex/lookup.go \
	internal/modindex/symbols.go \
	internal/modindex/index.go \
	internal/event/event.go
do
	copy_file "$XTOOLS_MOD/$rel" "$DST_GOIMPORTS/${rel#internal/}"
done
for sub in internal/event/core internal/event/keys internal/event/label; do
	found=0
	dest="${sub#internal/}"
	for src in "$XTOOLS_MOD/$sub"/*.go; do
		base="$(basename "$src")"
		if [[ "$base" == *_test.go ]]; then
			continue
		fi
		copy_file "$src" "$DST_GOIMPORTS/$dest/$base"
		found=1
	done
	if [[ "$found" -eq 0 ]]; then
		echo "sync-formatters: no Go files in $XTOOLS_MOD/$sub" >&2
		exit 1
	fi
done
copy_file "$XTOOLS_MOD/LICENSE" "$DST_GOIMPORTS/LICENSE"

python3 - "$DST_GOFUMPT" "$DST_GOIMPORTS" "$MOD" <<'PY'
import sys
from pathlib import Path

gofumpt = Path(sys.argv[1])
goimports = Path(sys.argv[2])
mod = sys.argv[3]

def rewrite(root: Path, old: str, new: str) -> None:
    for path in root.rglob("*.go"):
        text = path.read_text()
        if old not in text:
            continue
        path.write_text(text.replace(old, new))

rewrite(
    gofumpt,
    "mvdan.cc/gofumpt/",
    f"{mod}/internal/thirdparty/gofumpt/",
)
rewrite(
    goimports,
    "golang.org/x/tools/internal/",
    f"{mod}/internal/thirdparty/goimports/",
)

# Guards: these prefixes must stay on the main module.
astutil = "golang.org/x/tools/go/ast/astutil"
for path in list(gofumpt.rglob("*.go")) + list(goimports.rglob("*.go")):
    text = path.read_text()
    if "mvdan.cc/gofumpt/" in text:
        raise SystemExit(f"sync-formatters: gofumpt import prefix remains in {path}")
    if "golang.org/x/tools/internal/" in text:
        raise SystemExit(f"sync-formatters: x/tools internal prefix remains in {path}")
fmt = (gofumpt / "format/format.go").read_text()
fix_src = (goimports / "imports/fix.go").read_text()
mod_src = (goimports / "imports/mod.go").read_text()
env_src = (goimports / "imports/source_env.go").read_text()
if astutil not in fmt or astutil not in fix_src:
    raise SystemExit("sync-formatters: astutil import was rewritten")
if "golang.org/x/mod/module" not in mod_src:
    raise SystemExit("sync-formatters: golang.org/x/mod import was rewritten")
if "golang.org/x/sync/errgroup" not in env_src:
    raise SystemExit("sync-formatters: golang.org/x/sync import was rewritten")

# Local patch: ProcessEnv.Context lets a caller time out go env and go list.
# Idempotent: a second run on an already patched tree is a no-op.
field = (
    "\t// Context is used for go env and go list. Nil means context.Background.\n"
    "\tContext context.Context\n"
)
method = (
    "func (e *ProcessEnv) cmdctx() context.Context {\n"
    "\tif e != nil && e.Context != nil {\n"
    "\t\treturn e.Context\n"
    "\t}\n"
    "\treturn context.Background()\n"
    "}\n\n"
)

if '\t"context"\n' not in fix_src:
    anchor = "import (\n"
    if anchor not in fix_src:
        raise SystemExit("sync-formatters: fix.go import block not found")
    fix_src = fix_src.replace(anchor, anchor + '\t"context"\n', 1)

if "\tContext context.Context\n" not in fix_src:
    anchor = "\tinitialized bool // see TODO above\n"
    if anchor not in fix_src:
        raise SystemExit("sync-formatters: ProcessEnv field anchor not found in fix.go")
    fix_src = fix_src.replace(anchor, field + "\n" + anchor, 1)

if "func (e *ProcessEnv) cmdctx()" not in fix_src:
    anchor = "func (e *ProcessEnv) goEnv()"
    if anchor not in fix_src:
        raise SystemExit("sync-formatters: goEnv anchor not found in fix.go")
    fix_src = fix_src.replace(anchor, method + anchor, 1)

replacements = {
    "getFixes(context.Background(), fset, f, filename, env)":
        "getFixes(env.cmdctx(), fset, f, filename, env)",
    'e.invokeGo(context.TODO(), "env",':
        'e.invokeGo(e.cmdctx(), "env",',
}
for old, new in replacements.items():
    if old in fix_src:
        fix_src = fix_src.replace(old, new, 1)
    elif new not in fix_src:
        raise SystemExit(f"sync-formatters: fix.go call not found: {old}")

mod_replacements = {
    "gocommand.VendorEnabled(context.TODO(), inv, r.env.GocmdRunner)":
        "gocommand.VendorEnabled(r.env.cmdctx(), inv, r.env.GocmdRunner)",
    "gocommand.WorkspaceVendorEnabled(context.Background(), inv, r.env.GocmdRunner)":
        "gocommand.WorkspaceVendorEnabled(r.env.cmdctx(), inv, r.env.GocmdRunner)",
    'r.env.invokeGo(context.TODO(), "list",':
        'r.env.invokeGo(r.env.cmdctx(), "list",',
}
for old, new in mod_replacements.items():
    if old in mod_src:
        mod_src = mod_src.replace(old, new, 1)
    elif new not in mod_src:
        raise SystemExit(f"sync-formatters: mod.go call not found: {old}")

def expect(text: str, needle: str, where: str) -> None:
    if text.count(needle) != 1:
        raise SystemExit(f"sync-formatters: expected exactly one {needle!r} in {where}")

expect(fix_src, "\tContext context.Context\n", "fix.go")
expect(fix_src, "func (e *ProcessEnv) cmdctx()", "fix.go")
expect(fix_src, "getFixes(env.cmdctx(), fset, f, filename, env)", "fix.go")
expect(fix_src, 'e.invokeGo(e.cmdctx(), "env",', "fix.go")
expect(mod_src, "gocommand.VendorEnabled(r.env.cmdctx(), inv, r.env.GocmdRunner)", "mod.go")
expect(mod_src, "gocommand.WorkspaceVendorEnabled(r.env.cmdctx(), inv, r.env.GocmdRunner)", "mod.go")
expect(mod_src, 'r.env.invokeGo(r.env.cmdctx(), "list",', "mod.go")
# cmdctx itself returns context.Background() when Context is nil. That is the
# only remaining use; production go invocations must not.
if fix_src.count("context.Background()") != 1 or "context.TODO()" in fix_src:
    raise SystemExit("sync-formatters: unexpected context.Background/TODO in fix.go")
if "context.Background()" in mod_src or "context.TODO()" in mod_src:
    raise SystemExit("sync-formatters: unexpected context.Background/TODO in mod.go")

(goimports / "imports/fix.go").write_text(fix_src)
(goimports / "imports/mod.go").write_text(mod_src)
PY

cat > internal/thirdparty/UPSTREAM <<'EOF'
gofumpt=v0.12.0
goimports=golang.org/x/tools v0.50.0
govendor_go=go1.27.0
synced=2026-10-01

These trees are source copies, not module requirements. The main module
cannot require mvdan.cc/gofumpt v0.12.0 or golang.org/x/tools v0.50.0:
gofumpt v0.10.0 and the gopls fork's x/tools pseudo-version are already
required, and those versions must stay. scripts/sync-formatters.sh refreshes
the copies from the module cache and does not commit. CI excludes
internal/thirdparty from gofumpt so upstream formatting stays.

The copied goimports still executes the go command (go env and go list).
Its internal/ directory is flattened to internal/thirdparty/goimports/ so
the editor can import it. A nested internal/ directory would be hidden.
Local patch, reapplied on every sync: ProcessEnv.Context is the context for
those calls. Nil means context.Background. ProcessEnv.cmdctx returns it.
EOF

echo "sync-formatters: copied gofumpt v0.12.0 and goimports (x/tools v0.50.0); not committed"
