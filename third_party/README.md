# third_party

`go-git/` is generated and git-ignored. It is the go-git version pinned in the root `go.mod` with `go-git.patch` applied, wired in through a `replace` directive.

## Why

go-git v5 rejects repositories with `extensions.worktreeConfig = true` and `core.repositoryformatversion` 0 or unset (git sets this on sparse-checkout), failing with `core.repositoryformatversion does not support extension: worktreeconfig`. act opens the workspace with go-git, so Acton cannot run workflows in such repositories. `go-git.patch` backports https://github.com/go-git/go-git/pull/1982 (issue https://github.com/go-git/go-git/issues/1943): lowercase `extensionsValidForV0` keys and `worktreeconfig` added to `builtinExtensions`. `TestGoGitPatchWorktreeConfigExtension` in `internal/act` guards it.

## Workflow

- `mise run setup-go-git` downloads the pinned version, applies the patch and writes `go-git/.acton-version`. It is a no-op when that marker matches the pin and the patch. Every mise task that runs Go depends on it; run it manually before bare `go` commands on a fresh clone (CI release does).
- `mise run update-go-git [version|latest]` bumps the pin (default: latest v5), regenerates, runs `go mod tidy` and the tests. It reports when the patch no longer applies, is obsolete, or act needs go-git v6.

The version is pinned, not floated, because `go.sum` lists go-git's own dependencies.

## Removal

Delete this directory, the scripts, the mise tasks and the `replace` directive when act moves to go-git v6 or the fix ships in a go-git v5 release.
