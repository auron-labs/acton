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

## Build

```sh
go tool mygo build
```

The macOS app and disk image are written to `build/darwin-arm64/` on Apple silicon.
