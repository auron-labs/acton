<p align="center">
  <img src="assets/acton-logo.png" alt="Acton logo" width="128" height="128">
</p>

# Acton

Run GitHub Actions workflows locally in a native desktop app. Acton embeds the
`nektos/act` runner and shows jobs, steps, and their output alongside workflow settings.

## Run from source

On macOS, install Go 1.27.3 and the Xcode command-line tools. Start Docker before
running container-based workflows.

From the repository root:

```sh
go tool mygo dev
```

Choose a project folder containing `.github/workflows`, select a workflow, and
click **Run**. Jobs, steps, and output appear as the workflow runs. Acton loads
defaults from `.actrc`; use **Save** to write settings to the project's `.actrc`.

Select **Overview** under Jobs to read GitHub Actions errors, warnings, and
notices grouped by job and file. Each step's **Problems** tab shows its own
annotations; **Log** keeps the full output available.

Expand **Run preferences** to enable completion notifications or save run output
as JSONL. These app preferences are saved separately from `.actrc`. Run output
saving is off by default. Files are private to your user and stored under
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
| `list_runs` | List runs from the current desktop session. |
| `get_run` | Read jobs, steps, output, and errors. Omit `id` for the latest run. |
| `start_run` | Run the selected project with the current desktop settings. |
| `stop_run` | Cancel the active run and allow cleanup to finish. |
| `get_settings` | Read settings with inline secrets redacted. |
| `update_settings` | Apply a settings `patch` in memory. |

For example, call `update_settings` with
`{"patch":{"DryRun":true,"Event":"push"}}`, then `start_run` with `{}`.
Omitted settings stay unchanged. Omit redacted secret fields instead of sending
their display values back. Settings changes and concurrent starts are rejected
while a run is active. Use the desktop **Save** button to persist workflow
settings to `.actrc`.

## Build

```sh
go tool mygo build
```

The macOS app and disk image are written to `build/darwin-arm64/` on Apple silicon.
