<p align="center">
  <img src="assets/acton-logo.png" alt="Acton logo" width="128" height="128">
</p>

# Acton

Run GitHub Actions workflows locally in a native desktop app. Acton embeds the
`nektos/act` runner and shows jobs, steps, and their output alongside workflow settings.

## Install

On macOS or Linux, run the install script. It verifies the download against the app's update
signing key and `SHA256SUMS`:

```sh
curl -fsSL https://github.com/auron-labs/acton/releases/latest/download/install.sh | sh
```

The Linux script needs OpenSSL 3.0 or later. Or download the build for your platform from the
[GitHub Releases](https://github.com/auron-labs/acton/releases) page.

| Platform | Asset |
| --- | --- |
| macOS (Apple silicon, Intel) | `acton-<version>-<arch>.dmg`; open it and drag Acton to Applications |
| Linux, Debian-based | `acton_<version>_<arch>.deb` |
| Linux, other | `acton-<version>-linux-<arch>.tar.gz`, which contains `install.sh` to install for your user under `~/.local` |

Linux releases also include a `PKGBUILD` for Arch Linux (`makepkg -si`). Release builds are
published for macOS arm64 and amd64 and Linux amd64 and arm64. Windows is not built.

macOS builds are ad-hoc signed, not notarized, so Gatekeeper blocks the first launch.
Control-click Acton and choose **Open**, or allow it under System Settings > Privacy & Security.

Installed apps check GitHub Releases for signed updates and offer to install them. Copies
installed by a package manager, such as the Debian package, are updated by it instead.

Running workflows that use containers requires a running Docker engine.

## Quick start

Choose a project folder containing `.github/workflows`, tick the workflows to run
(none selected runs every workflow triggered by the event), and click **Run**.
Jobs, steps, and output appear as the workflow runs. Acton loads defaults from
`.actrc`; use **Save** to write settings to the project's `.actrc`, which stores
each selected workflow as a `-W` line.

Each project opens in its own tab, and tabs can run at the same time. The **Home**
tab lists every open project with its run status, and shows how many runs are in
progress. A tab can be closed once its run has finished. Every run executes in a
helper process (the Acton executable started with `--act-worker`), so concurrent
projects don't share environment or logger state.

Select **Workflow** under Jobs to read the source of the workflow files the run
executes while it runs.

Select **Overview** under Jobs to read GitHub Actions errors, warnings, and
notices grouped by job and file. Each step's **Problems** tab shows its own
annotations; **Log** keeps the full output available.

Expand **Run preferences** to turn off completion notifications (on by default)
or save run output as JSONL. These app preferences are saved separately from
`.actrc`. Run output saving is off by default. Files are private to your user and stored under
`~/.local/share/acton/runs/<project-path-hash>/<timestamp>/`, or under
`$XDG_DATA_HOME/acton/runs/` when that variable contains an absolute path. Each
job gets a `<workflow>-<job>.jsonl` file; matrix variants and colliding names
get a suffix. Records contain masked `msg` fields. Unmasked records, including
process-wide diagnostics and output from `--insecure-secrets`, are replaced
with an omission message in `runner.jsonl`.

Acton removes its project-owned containers and temporary job volumes before
and after each run, including reusable workflows and cancellation. Temporary
workspace and environment volumes are isolated per run. User-supplied volumes,
the tool cache, images, and unlabelled containers from older versions are
preserved. A conflicting unowned container name causes the run to fail safely.

## Connect an agent

Start the desktop app with its optional local MCP server:

```sh
go run ./cmd/acton --mcp /absolute/path/to/project
```

Configure an MCP client to use the Streamable HTTP endpoint
`http://127.0.0.1:7337/mcp`. To choose another port, pass
`--mcp=127.0.0.1:7447`. The server accepts numeric loopback addresses only and
rejects requests from other browser origins. An attached local agent can run
workflow commands with the same filesystem, Docker, and network access as the
desktop app.

| Tool | Action |
| --- | --- |
| `list_runs` | List runs from the current desktop session across all projects. |
| `get_run` | Read jobs, steps, output, and errors. Omit `id` for the project's latest run. |
| `start_run` | Run a project with its current desktop settings. |
| `stop_run` | Cancel a project's active run and allow cleanup to finish. |
| `get_settings` | Read settings with inline secrets redacted. |
| `update_settings` | Apply a settings `patch` in memory. |

Every tool except `get_run` with an `id` accepts an optional absolute `project`
directory; it defaults to the selected tab. `update_settings` and `start_run` open
a tab for a project that isn't open yet without switching to it. Runs for different
projects can overlap, and each project accepts one run at a time.

For example, call `update_settings` with
`{"project":"/path/to/app","patch":{"DryRun":true,"Event":"push","Workflows":[".github/workflows/ci.yml"]}}`,
then `start_run` with `{"project":"/path/to/app"}`.
Omitted settings stay unchanged. Omit redacted secret fields instead of sending
their display values back. Settings changes and concurrent starts for a project
are rejected while its run is active. Use the desktop **Save** button to persist
workflow settings to `.actrc`.

## Troubleshooting

- **Container workflows fail to start:** start your Docker engine and run again.
- **MCP client cannot connect:** confirm Acton was started with `--mcp` and the client uses
  the same port (default `7337`). Only numeric loopback addresses are accepted.
- **Settings are not kept:** **Save** writes to the project's `.actrc`. Changes made without
  saving, including through MCP `update_settings`, apply to the current session only.
- **macOS says the app cannot be opened:** see the Gatekeeper note under [Install](#install).

## Build

```sh
go tool mygo build
```

The macOS app and disk image are written to `build/darwin-arm64/` on Apple silicon.

## Development

Setup, checks, and commit conventions are in [CONTRIBUTING.md](CONTRIBUTING.md). To run from
source (Go 1.27.3; Xcode command-line tools on macOS):

```sh
mise install
mise run dev
```

For releases and auto-update setup, see [docs/RELEASING.md](docs/RELEASING.md).

## Security

See [SECURITY.md](SECURITY.md) for supported versions and how to report a vulnerability.

## License

[MIT](LICENSE)
