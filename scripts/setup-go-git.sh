#!/usr/bin/env bash
# Generate third_party/go-git: the go-git version pinned in go.mod with
# third_party/go-git.patch applied. Idempotent; see third_party/README.md.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
target=$root/third_party/go-git
patch_file=$root/third_party/go-git.patch
mod=github.com/go-git/go-git/v5

pin=$(awk -v m="$mod" '$1 == m && $2 ~ /^v/ {print $2; exit}' "$root/go.mod")
[ -n "$pin" ] || { echo "error: $mod is not required in go.mod" >&2; exit 1; }
stamp="$pin $(shasum -a 256 "$patch_file" | cut -d' ' -f1)"

if [ -d "$target" ] && [ "$(cat "$target/.acton-version" 2>/dev/null)" = "$stamp" ]; then
  exit 0
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Download from a throwaway module: the root go.mod replaces go-git with the
# directory this script creates, which may not exist yet.
mkdir "$work/q"
(cd "$work/q" && go mod init scratch >/dev/null 2>&1)
src=$(cd "$work/q" && GOFLAGS=-mod=mod go mod download -json "$mod@$pin" | sed -n 's/.*"Dir": "\(.*\)",\{0,1\}$/\1/p')
[ -d "$src" ] || { echo "error: could not download $mod@$pin" >&2; exit 1; }

echo "Preparing go-git $pin with local patch"
cp -R "$src" "$work/new"
chmod -R u+w "$work/new"

if ! (cd "$work/new" && patch -p1 --no-backup-if-mismatch <"$patch_file" >"$work/patch.log" 2>&1); then
  cat "$work/patch.log" >&2
  if awk '/extensionsValidForV0 = /,/^\t}/' "$src/repository_extensions.go" | grep -q '"worktreeconfig"'; then
    echo "The patch looks obsolete: go-git $pin already handles worktreeconfig." >&2
    echo "Remove third_party, scripts/*go-git.sh, the go-git tasks, the replace directive and the guard test." >&2
  else
    echo "The patch no longer applies to go-git $pin; redo it and regenerate third_party/go-git.patch." >&2
  fi
  exit 1
fi

echo "$stamp" >"$work/new/.acton-version"
rm -rf "$target"
mv "$work/new" "$target"
