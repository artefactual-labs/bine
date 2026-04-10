<p align="left">
  <a href="https://github.com/artefactual-labs/bine/releases/latest"><img src="https://img.shields.io/github/v/release/artefactual-labs/bine.svg?color=orange" alt="Latest release"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="Apache 2.0 license"/></a>
  <a href="https://codecov.io/gh/artefactual-labs/bine"><img src="https://img.shields.io/codecov/c/github/artefactual-labs/bine" alt="Codecov"/></a>
</p>

# bine

**bine** manages external binary tools required by a development project.

You declare the tools your project needs in `.bine.json`. `bine` downloads them
into a project-scoped cache and gives you a consistent way to run them. This
keeps versions aligned across local development and CI without polluting global
system paths.

## Why bine

- **Project-scoped:** Each project gets its own binary cache.
- **Reproducible:** Pin exact tool versions when you need deterministic builds.
- **Flexible:** Install tools from GitHub releases or Go packages.
- **Simple to use:** Run tools with `bine run` or add them to `PATH` with
  `bine env`.

## Installation

### Option 1: Download a release binary

Download the archive for your platform from the [releases page] and place the
binary somewhere in your `PATH`.

For example, on Linux `amd64`:

```sh
curl -L -o ~/.local/bin/bine \
  https://github.com/artefactual-labs/bine/releases/download/v0.21.0/bine_0.21.0_linux_amd64
chmod +x ~/.local/bin/bine
```

Choose the release asset that matches your OS and architecture.

### Option 2: Use `go tool`

If you already manage development tools through Go, you can add `bine` as a Go
tool:

```sh
go get -tool github.com/artefactual-labs/bine@latest
```

In that setup, invoke it as `go tool bine ...`.

## Quick start

Create a `.bine.json` file in your project root:

`.bine.json` accepts JSON with comments.

```jsonc
{
  "project": "my-awesome-project",
  "bins": [
    {
      "name": "golangci-lint",
      "url": "https://github.com/golangci/golangci-lint",
      "version": "2.0.2",
      "asset_pattern": "{name}-{version}-{goos}-{goarch}.tar.gz"
    }
  ]
}
```

Install the configured tools:

```sh
bine sync
```

Run a managed binary:

```sh
bine run golangci-lint --help
```

Or add the project bin directory to your shell `PATH`:

```sh
# Bash or Zsh
source <(bine env --shell=bash)

# Fish
bine env --shell=fish | source

# POSIX shells
eval "$(bine env --shell=sh)"
```

After that, you can call the tool directly:

```sh
golangci-lint --help
```

## Configuration

The `.bine.json` file defines the binaries available in the current project.

```jsonc
{
  "project": "example-project",
  "bins": [
    {
      "name": "jq",
      "version": "1.8.0",
      "url": "https://github.com/jqlang/jq",
      "asset_pattern": "{name}-{goos}-{goarch}",
      "tag_pattern": "{name}-{version}",
      "modifiers": {
        "goos": {
          "darwin": "macos"
        }
      }
    },
    {
      "name": "govulncheck",
      "go_package": "golang.org/x/vuln/cmd/govulncheck",
      "version": "latest"
    }
  ]
}
```

Each entry in `bins` uses one installation strategy:

- GitHub release assets, using fields such as `url` and `asset_pattern`
- Go packages, using `go_package`

### Go package versions

When `go_package` is used, `version` supports two modes:

- Pinned mode: set `version` to a specific release such as `"v0.30.0"` or
  `"0.30.0"`.
- Latest-tracking mode: omit `version`, or set it to `"latest"`.

For example, this `go install` command:

```sh
go install golang.org/x/vuln/cmd/govulncheck@latest
```

maps to:

```json
{
  "name": "govulncheck",
  "go_package": "golang.org/x/vuln/cmd/govulncheck",
  "version": "latest"
}
```

Typical workflow:

1. `bine sync` installs the current latest version.
2. `bine list --outdated` checks whether the installed resolved version is now behind.
3. `bine upgrade govulncheck` refreshes that tool if a newer release exists.

If you need to rebuild cached binaries without changing their configured
versions, for example after switching Go toolchains, use `bine get --force
<NAME>`, `bine sync --force`, or `bine reinstall`.

`bine upgrade` without an argument upgrades every configured binary.

`bine` keeps track of the exact version installed from `latest`, so it can
later report whether that cached binary is stale without rewriting
`.bine.json`.

### `asset_pattern` variables

Use template variables in `asset_pattern` to match upstream release filenames.

| Variable | Description | Example |
|---|---|---|
| `{name}` | Binary `name` from the config. | `jq` |
| `{version}` | Version without a `v` prefix. | `1.8.0` |
| `{goos}` | Go OS identifier (`runtime.GOOS`). | `linux`, `darwin` |
| `{goarch}` | Go architecture identifier (`runtime.GOARCH`). | `amd64`, `arm64` |
| `{os}` | System OS name (`uname -s`). | `Linux`, `Darwin` |
| `{arch}` | System architecture (`uname -m`). | `x86_64`, `arm64` |
| `{triple}` | Rust-style target triple. | `x86_64-unknown-linux-gnu` |

Combine `asset_pattern` with `modifiers` when an upstream project uses
non-standard naming.

### `tag_pattern` variables

`tag_pattern` controls how Git tags are constructed when resolving GitHub
releases. The default is `v{version}`.

| Variable | Description | Example |
|---|---|---|
| `{name}` | Binary `name` from the config. | `jq` |
| `{version}` | Version without a `v` prefix. | `1.8.0` |

## Commands

Use `bine --help` for the full command reference.

Core subcommands:

- `bine config get <KEY>`: Print a configuration value.
- `bine env`: Output shell code that adds the project bin directory to `PATH`.
- `bine get [--force] <NAME>`: Download one binary and print its path.
- `bine list`: List configured binaries.
- `bine path`: Print the current project bin directory.
- `bine reinstall`: Reinstall all configured binaries. Alias for `bine sync --force`.
- `bine run <NAME> [ARGS...]`: Download a binary and execute it.
- `bine sync [--force]`: Install all binaries defined in `.bine.json`.
- `bine upgrade [NAME]`: Upgrade one binary or all configured binaries.
- `bine version`: Print the current `bine` version.

Global flags:

- `-v, --verbosity`: Increase log verbosity.
- `--cache-dir`: Override the cache directory location.
- `--github-api-token`: Provide a GitHub API token for authenticated requests.

## GitHub REST API rate limiting

`bine` uses the GitHub REST API to inspect releases and download binaries from
GitHub repositories. Unauthenticated requests are limited to 60 requests per
hour.

You can pass a token either with `--github-api-token` or through the
`BINE_GITHUB_API_TOKEN` environment variable:

```sh
export BINE_GITHUB_API_TOKEN=your_token_here
bine list --outdated
```

## Examples

See the [`examples`] directory for integration patterns:

- [`examples/fish`] for Fish shell helpers
- [`examples/make`] for Make-based workflows
- [`examples/just`] for Just-based workflows

## FAQ

### Why not use `go get -tool`?

`go get -tool` is useful for Go module-based binaries, but it does not cover
non-Go tools such as `jq` or `shfmt`.

That said, `go get -tool` is a good way to install `bine` itself in Go-based
projects:

```sh
go get -tool github.com/artefactual-labs/bine@latest
go tool bine path
```

### How does bine compare to asdf or mise?

[`asdf`] and [`mise`] are broader tools with more features. `bine` stays focused
on lightweight, project-scoped binary management for development workflows.

### How does bine compare to Nix?

Nix provides fully reproducible and isolated environments. `bine` is narrower:
it focuses on managing development binaries with less setup and a smaller
surface area.

### How does bine work with Fish shell?

For one-off use, this is enough:

```fish
bine env --shell=fish | source
```

If you install `bine` as a Go tool instead, use:

```fish
go tool bine env --shell=fish | source
```

For a reusable helper function, see [`examples/fish`].

[releases page]: https://github.com/artefactual-labs/bine/releases
[`examples`]: ./examples
[`examples/fish`]: ./examples/fish
[`examples/make`]: ./examples/make
[`examples/just`]: ./examples/just
[`asdf`]: https://asdf-vm.com/
[`mise`]: https://mise.jdx.dev/
