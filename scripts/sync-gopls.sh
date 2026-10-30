#!/usr/bin/env bash
# Outline-level sync of the gopls fork. Never commits or pushes.
# Usage: scripts/sync-gopls.sh gopls/vX.Y.Z
set -euo pipefail
TAG="${1:?usage: sync-gopls.sh gopls/vX.Y.Z}"
MOD="github.com/russoj88/gio-ide"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git clone --quiet --depth 1 --branch "$TAG" https://go.googlesource.com/tools "$WORK/tools"
XVER="$(cd "$WORK/tools/gopls" && go list -m -f '{{.Version}}' golang.org/x/tools)"
XDIR="$(go mod download -json "golang.org/x/tools@$XVER" | jq -r .Dir)"
go run ./cmd/goplssync closure  --gopls "$WORK/tools/gopls" > "$WORK/pkgs.txt"
go run ./cmd/goplssync copy     --gopls "$WORK/tools/gopls" --xtools "$XDIR" \
                                  --pkgs "$WORK/pkgs.txt" --dst internal/gopls
go run ./cmd/goplssync rewrite  --module "$MOD" --dir internal/gopls
# Patches are unified diffs with a/internal/gopls paths. Apply from the repo root.
shopt -s nullglob
for p in internal/gopls/patches/*.patch; do
  git apply --3way "$p"
done
go run ./cmd/goplssync gomod    --from "$WORK/tools/gopls/go.mod"
go mod tidy
# The toolchain may rewrite the go line. Keep the module on the current release.
python3 - <<'PY'
from pathlib import Path
p = Path("go.mod")
lines = p.read_text().splitlines()
out = []
for line in lines:
    if line.startswith("go "):
        out.append("go 1.27.2")
    else:
        out.append(line)
p.write_text("\n".join(out) + "\n")
PY
printf 'tag=%s\ncommit=%s\nxtools=%s\nmodule_floor=golang.org/x/tools@v0.49.0\n' "$TAG" \
  "$(git -C "$WORK/tools" rev-parse HEAD)" "$XVER" > internal/gopls/UPSTREAM
export CGO_ENABLED=1
go build -trimpath -o "$WORK/gio-ide" ./cmd/gio-ide
go vet ./...
go run ./cmd/goplssync lint --dir internal/gopls
go test ./internal/intel/... ./internal/gopls/...
echo "sync-gopls: reviewed tree is ready; not committed"
