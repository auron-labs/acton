#!/usr/bin/env bash
# Bump the pinned go-git version in go.mod, regenerate third_party/go-git and test.
# Usage: scripts/update-go-git.sh [version|latest]   (default: latest v5)
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
mod=github.com/go-git/go-git/v5
cd "$root"

drop_instructions() {
  cat >&2 <<MSG
The local go-git patch can be dropped:
  rm -rf third_party scripts/setup-go-git.sh scripts/update-go-git.sh
  go mod edit -dropreplace $mod && go mod tidy
and remove the go-git tasks and depends from mise.toml, the .gitignore entry and the guard test in internal/act.
MSG
}

if go mod graph | awk '$1 ~ /^github.com\/nektos\/act@/ && $2 ~ /^github.com\/go-git\/go-git\/v[6-9]/ {f=1} END {exit !f}'; then
  echo "act requires go-git v6 or later." >&2
  drop_instructions
  exit 1
fi

req=${1:-latest}
case "$req" in
  latest)
    scratch=$(mktemp -d)
    trap 'rm -rf "$scratch"' EXIT
    (cd "$scratch" && go mod init scratch >/dev/null 2>&1)
    req=$(cd "$scratch" && GOFLAGS=-mod=mod go list -m -f '{{.Version}}' "$mod@latest")
    ;;
  [0-9]*) req=v$req ;;
  v[0-9]*) ;;
  *) echo "error: invalid version: $req" >&2; exit 1 ;;
esac

case "$req" in
  v5.*) ;;
  *) echo "Requested go-git $req is not v5." >&2; drop_instructions; exit 1 ;;
esac

backup=$(mktemp -d)
cp go.mod go.sum "$backup"
restore() { cp "$backup/go.mod" "$backup/go.sum" .; }

echo "Pinning go-git $req"
go mod edit -require="$mod@$req"
if ! scripts/setup-go-git.sh; then
  restore
  exit 1
fi
go mod tidy
(cd third_party/go-git && go test -run TestVerifyExtensions .)
go test ./internal/act -run TestGoGitPatchWorktreeConfigExtension
echo "go-git pinned at $req with local patch."
