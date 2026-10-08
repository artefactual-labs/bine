<p align="left">
  <a href="https://github.com/artefactual-labs/bine/releases/latest"><img src="https://img.shields.io/github/v/release/artefactual-labs/bine.svg?color=orange" alt="Latest release"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="Apache 2.0 license"/></a>
  <a href="https://codecov.io/gh/artefactual-labs/bine"><img src="https://img.shields.io/codecov/c/github/artefactual-labs/bine" alt="Codecov"/></a>
</p>

# bine

**bine** manages external binary tools required by a development project.

You declare the tools your project needs in `.bine.json` or `.bine.toml`.
`bine` downloads them into a project-scoped cache and gives you a consistent
way to run them. This keeps versions aligned across local development and CI
without polluting global system paths.

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

Run `bine init my-project` in your project root to create an empty `.bine.json`.
Omit the name to use the directory name, or pass `--format toml` for `.bine.toml`.
Then add tools to the configuration, as shown below.

This is the JSON variant (`.bine.json`):

```jsonc
{
  // Comments are supported.
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

This is the TOML variant (`.bine.toml`):

```toml
project = "my-awesome-project"

[[bins]]
name = "golangci-lint"
url = "https://github.com/golangci/golangci-lint"
version = "2.0.2"
asset_pattern = "{name}-{version}-{goos}-{goarch}.tar.gz"
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

The `.bine.json` or `.bine.toml` file defines the binaries available in the
current project.

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
- Signed Packslip releases, using `packslip`

### Packslip releases

For a publisher that provides a [Packslip](https://packslip.dev/) manifest, specify
its project identity in a `packslip` object and an exact version. Both JSON and
TOML support this structure:

```toml
[[bins]]
name = "hk"
version = "2.4.0"
packslip = { project = "github.com/jdx/hk" }
```

`bine` discovers the release, verifies its GitHub Actions signature and Sigstore
transparency evidence, selects an artifact for the host, and checks its signed
size and digests before extraction. The declared executable path determines what
is installed. No filename recipe is needed, and verification failures never fall
back to an unsigned download.

`name` is the local executable name. Inside `packslip`, optional `command` selects
a publisher's command when renaming it or choosing from several commands. Without
`command`, `bine` matches `name`, then accepts a sole declared command; otherwise
it reports the available commands. Optional `variant` selects a named build;
omitting it selects only builds without a variant. Monorepo identities may
include a tool subpath, such as `github.com/owner/repository/tool`.

TOML also allows a separate table, which belongs to the preceding `[[bins]]` entry:

```toml
[[bins]]
name = "local-hk"
version = "2.4.0"

[bins.packslip]
project = "github.com/jdx/hk"
command = "hk"
```

Put common fields such as `name` and `version` before `[bins.packslip]`; subsequent
fields belong to that table. The equivalent JSON bin entry is:

```json
{
  "name": "local-hk",
  "version": "2.4.0",
  "packslip": { "project": "github.com/jdx/hk", "command": "hk" }
}
```

`packslip.project` is required. Unknown fields inside `packslip` are rejected so
misspelled selectors cannot silently choose a different executable or build.

A Packslip entry cannot also contain `url`, `go_package`, `asset_pattern`,
`tag_pattern`, or `modifiers`. `version` must be a complete semantic version,
optionally prefixed with `v`; `latest` and ranges are not supported. Build metadata
is preserved. `bine list --outdated` and `bine upgrade` check stable Packslip
releases. Upgrades verify and install the candidate before updating its version
in the configuration.

The initial implementation supports standalone executables distributed as raw
files or in tar, tar.gz/tgz, tar.xz, tar.zst, tar.bz2, and zip archives. It copies
the selected regular file; distributions requiring adjacent libraries, data, or
helper executables are outside this installation model. Links are not installed
as executables. Optional resources such as completions and man pages are not
installed. Known unmet OS, glibc, and shared-library requirements stop installation;
unknown checks and missing required commands produce warnings.

Only vendor manifests with GitHub project identities, signed by GitHub Actions
with repository and owner certificate IDs, are currently supported. Domain
projects, key signing, repackagers, and supplementary signed release lists are
not supported. A repository presenting a supplementary list is refused, including
for pinned versions, so its withdrawal policy is never silently ignored.
Manifest and artifact downloads must be publicly accessible HTTPS URLs. GitHub
API credentials are used only for GitHub metadata requests.

Packslip trust is stored separately from installed binaries, in
`bine/packslip/trust.json` beneath the OS user configuration directory (normally
`~/.config` on Linux or `~/Library/Application Support` on macOS). Library callers
can override it with `WithStateDir`. Accepted repository IDs, owner IDs, signing
workflows, and artifact provenance presence survive cache cleanup and forced
reinstallation. Unapproved owner/workflow changes, repository replacement, and
lost provenance are refused. Renames retaining repository and owner IDs keep
their trust. There is no automatic trust-reset option; review a publisher change
before deliberately editing the trust record.

Trust is established on first use on each machine; a shared project lockfile is
not implemented yet. Switching a configured entry to Packslip forces a verified
installation even when its version is unchanged. Once installed, a matching
receipt and local checksum allow ordinary runs without upstream requests.
Trust inspection only reads the existing file; it does not create a lock file or
require write access. Missing trust requires a fresh verified installation.
Acceptance rechecks trust under a writer lock and atomically replaces the file
only when a pin changes, so concurrent inspections see a complete snapshot. A
forced reinstall checks upstream again. Provenance links are remembered to
prevent their removal, but the linked build attestations are not independently
verified.

### Known binaries

`bine` includes built-in defaults for common GitHub release assets. When `url`
matches a known source, `bine` can fill repeated fields such as `asset_pattern`,
`tag_pattern`, and `modifiers`. The bin `name` is only the local executable name;
it is not used to choose library defaults. Any fields you set explicitly still
take precedence.

```toml
[[bins]]
name = "go-mod-outdated"
url = "https://github.com/psampaz/go-mod-outdated"
version = "0.9.0"
```

See the [known binaries library] for the current built-in templates.

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
later report whether that cached binary is stale without rewriting the config
file.

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

- `bine init [--format json|toml] [PROJECT]`: Create an empty project configuration.
- `bine config get <KEY>`: Print a configuration value.
- `bine auth login [HOST]`: Authenticate with a service in a web browser.
- `bine auth logout [HOST]`: Remove saved authentication for a service.
- `bine auth status [HOST]`: Show saved authentication status.
- `bine env`: Output shell code that adds the project bin directory to `PATH`.
- `bine get [--force] <NAME>`: Download one binary and print its path.
- `bine list`: List configured binaries.
- `bine path`: Print the current project bin directory.
- `bine reinstall`: Reinstall all configured binaries. Alias for `bine sync --force`.
- `bine run <NAME> [ARGS...]`: Download a binary and execute it.
- `bine sync [--force]`: Install all binaries defined in the project config file.
- `bine upgrade [NAME]`: Upgrade one binary or all configured binaries.
- `bine version`: Print the current `bine` version.

Global flags:

- `-v, --verbose`: Increase log verbosity. Repeat as `-vv` or `-vvv`; `-vvv` is
  the highest shorthand level we expect to need in practice.
- `--verbosity=N`: Set the log verbosity level explicitly.
- `--cache-dir`: Override the cache directory location.
- `--github-api-token`: Provide a GitHub API token for authenticated requests.
- `--check-interval`: Set the minimum interval between upstream version checks
  performed by commands such as `list --outdated` and `upgrade`.

## Installation failures

Installations and upgrades can leave partial changes when they fail; earlier
changes are not rolled back. After fixing the cause, check the versions in your
configuration and run `bine sync` to retry. If a damaged installation record
blocks recovery, use `bine get --force <NAME>`.

## GitHub REST API rate limiting

`bine` uses the GitHub REST API to inspect releases and download binaries from
GitHub repositories. Unauthenticated requests are limited to 60 requests per
hour.

For interactive use, authenticate through GitHub's OAuth device flow:

```sh
bine auth login github.com
```

`github.com` is the default host, so `bine auth login` is equivalent. Bine
prints a one-time code, opens GitHub in your browser, and waits for you to
approve access. Use `--no-browser` on a headless machine and open the printed
URL yourself. The OAuth app requests read-only access to public information and
stores its access token in the operating system's credential store. GitHub
OAuth app tokens remain active until they are revoked.

Inspect or remove the saved credential with:

```sh
bine auth status github.com
bine auth logout github.com
```

The authentication commands are provider-oriented so additional services can
be supported in the future. Currently, only `github.com` is supported.

For CI or other non-interactive environments, pass a token either with
`--github-api-token` or through the `BINE_GITHUB_API_TOKEN` environment
variable:

```sh
export BINE_GITHUB_API_TOKEN=your_token_here
bine list --outdated
```

An explicitly supplied token takes precedence over a credential saved by
`bine auth login`.

Use `--check-interval` to pace checks across all binary providers. The first
eligible check starts immediately, and time spent performing a check counts
toward the interval:

```sh
bine list --outdated --check-interval=1s
bine upgrade --check-interval=1s
```

When GitHub responds with rate-limit information, Bine honors `Retry-After`
while retrying and reports the reset time when the primary quota is exhausted.

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
[known binaries library]: https://github.com/artefactual-labs/bine/blob/main/bine/library.go
[`examples`]: ./examples
[`examples/fish`]: ./examples/fish
[`examples/make`]: ./examples/make
[`examples/just`]: ./examples/just
[`asdf`]: https://asdf-vm.com/
[`mise`]: https://mise.jdx.dev/
