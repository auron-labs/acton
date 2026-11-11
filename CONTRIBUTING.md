# Contributing

Contributions are welcome. For anything larger than a small fix, open an issue first to
discuss it.

## Setup

Prerequisites:

- [mise](https://mise.jdx.dev), which installs Go, golangci-lint, hk, and the other tools
  pinned in `mise.toml`
- A running Docker engine, to run workflows with act
- Xcode command-line tools on macOS

```sh
mise install
hk install --mise   # git hooks: pre-commit checks and commit message validation
```

## Tasks

| Command | Purpose |
| --- | --- |
| `mise run dev` | Run the app in development mode |
| `mise run build` | Build the app with `go tool mygo build` |
| `mise run test` | Run `go test ./...` |
| `mise run lint` | Run golangci-lint |
| `mise run check` | Run all hk checks over the repository |
| `mise run fix` | Apply hk auto-fixes |

CI runs `test` and `lint` on pull requests.

## Commits and pull requests

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org)
(`feat:`, `fix:`, ...). The `commit-msg` hook enforces this. release-please derives
version bumps and the changelog from these messages, so choose the type carefully.

Pull requests are squash-merged, so the PR title should itself be a conventional commit
message. Maintainer release steps are in [docs/RELEASING.md](docs/RELEASING.md).

By contributing you agree that your work is licensed under the [MIT License](LICENSE).
See also the [Code of Conduct](CODE_OF_CONDUCT.md).
