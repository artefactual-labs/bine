<p align="left">
  <a href="https://github.com/artefactual-labs/bine/releases/latest"><img src="https://img.shields.io/github/v/release/artefactual-labs/bine.svg?color=orange"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg"/></a>
  <a href="https://codecov.io/gh/artefactual-labs/bine"><img src="https://img.shields.io/codecov/c/github/artefactual-labs/bine"/></a>
</p>

# bine

**bine** helps you manage external binary tools required for your development
projects. It reads a `.bine.json` configuration file, downloads specified
binaries into a local, project-scoped cache, and makes them available for you to
run.

This ensures your whole team uses the exact same tool versions without polluting
global system paths or requiring manual installation steps.

## Core concepts

- **Declarative:** Define all your project's binary dependencies in a single
  `.bine.json` file.
- **Project-scoped:** Binaries are stored in a local cache directory specific to
  your project (e.g.: `~/.cache/bine/<project_name>`), avoiding conflicts.
- **Reproducible:** Pin exact versions to ensure your team and CI/CD
  environments use the same tools.
- **Seamless integration:** Use `bine run` to execute tools directly, or
  `bine env` to add them to your shell's `PATH`.

## Installation

You can download *bine* as a self-contained static binary from the [releases
page]; just place it in a directory included in your system `PATH` to use it
from the command line.

For example:

    curl -L -o ~/.local/bin/bine https://github.com/artefactual-labs/bine/releases/download/v0.21.0/bine_0.21.0_linux_amd64
    chmod +x ~/.local/bin/bine

## Quick start

1. **Create a configuration file.**

   In the root of your project, create a `.bine.json` file. This file defines
   the tools you need. It supports JSON with comments.

   ```jsonc
   {
     "project": "my-awesome-project",
     "bins": [
       {
         // Fast linters runner for Go projects.
         "name": "golangci-lint",
         "url": "https://github.com/golangci/golangci-lint",
         "version": "2.0.2",
         "asset_pattern": "{name}-{version}-{goos}-{goarch}.tar.gz"
       }
     ]
   }
   ```

2. **Install the binaries.**

   Run `bine sync` to download and install all the tools defined in your config file.

       bine sync

3. **Run a tool.**

   Use `bine run` to execute a managed binary. It works just like `npx` or
   `bundle exec`.

       bine run golangci-lint --help

4. **Add binaries to your PATH (recommended).**

   To use the tools directly in your shell, use `bine env`:

   ```sh
   # Bash, Zsh (process substitution)
   source <(bine env --shell=bash)

   # Fish (source command)
   bine env --shell=fish | source

   # POSIX shells (sh, dash, etc.)
   eval "$(bine env)"
   ```

   Now you can call your tools directly:

       golangci-lint --help

## Configuration file

The `.bine.json` file is the heart of the tool. Here is a detailed breakdown of
its structure:

```jsonc
{
  // A unique name for your project. Used to create a scoped cache directory.
  "project": "string",

  // A list of binaries to manage.
  "bins": [
    {
      // The name you use to run the binary, e.g. `bine run jq`.
      "name": "jq",
      // The semantic version to install, e.g. "1.8.0".
      "version": "1.8.0",

      //
      // Option 1: Install from a release asset URL.
      //
      // The URL of the GitHub repository.
      "url": "https://github.com/jqlang/jq",
      // A template for the downloadable asset's filename, e.g. "jq-darwin-amd64".
      "asset_pattern": "{name}-{goos}-{goarch}",
      // A template for the Git tag. Defaults to "v{version}".
      "tag_pattern": "{name}-{version}",
      // A map to substitute template variables, useful when the asset names don't follow the common patterns.
      "modifiers": {
        "goos": {
          // jq uses "macos" instead of "darwin" in its asset names.
          "darwin": "macos"
        }
      }

      //
      // Option 2: Install from a Go package.
      //
      // The Go module path to install the binary from.
      // "go_package": "golang.org/x/tools/cmd/stringer",
    }
  ]
}
```

### Asset pattern variables

The `asset_pattern` field uses templates to construct the correct download URL
for your platform.

| Variable | Description | Example |
|---|---|---|
| `{name}` | The binary's `name` from the config. | `jq` |
| `{version}` | The `version` (without a `v` prefix). | `1.8.0` |
| `{goos}` | Go's OS identifier (`runtime.GOOS`). | `linux`, `darwin` |
| `{goarch}` | Go's architecture identifier (`runtime.GOARCH`).| `amd64`, `arm64` |
| `{os}` | System's OS name (`uname -s`). | `Linux`, `Darwin` |
| `{arch}` | System's architecture (`uname -m`). | `x86_64`, `arm64` |
| `{triple}` | The Rust-style target triple. | `x86_64-unknown-linux-gnu` |

It can be combined with the `modifiers` field to handle project-specific
naming conventions. See the example above for how to use it.

### Tag pattern variables

The `tag_pattern` field is used to construct the Git tag for the binary found
in GitHub releases. It defaults to `v{version}` but can be customized.

| Variable | Description | Example |
|---|---|---|
| `{name}` | The binary's `name` from the config. | `jq` |
| `{version}` | The `version` (without a `v` prefix). | `1.8.0` |

## Commands

*bine* provides several commands to manage your binaries.

Use `bine --help` to see the full list of commands and options:

```
COMMAND
  bine -- Simple binary manager for developers.

USAGE
  bine [FLAGS] <SUBCOMMAND> ...

bine helps manage external binary tools needed for development projects.

It downloads specified binaries from their sources into a local cache directory,
ensuring you have the right versions without cluttering your system.

SUBCOMMANDS
  env       Output shell commands to set up the PATH system variable.
  get       Download a binary and print its path.
  list      Print the list of binaries.
  path      Print the path of the binary store.
  run       Download a binary and run it.
  sync      Install all binaries defined in the configuration file.
  upgrade   Upgrade all binaries defined in the configuration file.
  version   Print the current version of bine.

FLAGS
  -v, --verbosity INT             Log verbosity level. The higher the number, the more verbose the output. (default: -1)
      --cache-dir STRING          Path to the cache directory.
      --github-api-token STRING   GitHub API token for authentication.
```

### GitHub Rest API rate limiting

*bine* uses the GitHub Rest API to fetch release information and download
binaries from GitHub repositories. By default, unauthenticated requests are
limited to 60 requests per hour. Use the ``--github-api-token`` command-line
flag to pass your token or set the `BINE_GITHUB_API_TOKEN` environment variable.

Usage example in Bash:

```bash
export BINE_GITHUB_API_TOKEN=your_token_here
bine list --outdated
```

## Frequently asked questions

### Why not use `go get -tool`?

While `go get -tool` is a great feature for managing Go module-based binaries,
it doesn't help with other essential tools that aren't written in Go, e.g. `jq`
or `shfmt`.

That said, we use `go get -tool` to install *bine* itself in Go projects:

    go get -tool github.com/artefactual-labs/bine@latest

Invoke *bine* with the `go tool` command to ensure it uses the Go toolchain's
environment:

    $ go tool bine path
    /home/ethan/.cache/bine/project/linux/amd64/bin

### How does *bine* compare to asdf?

[asdf] is much better overall, but we created *bine* mainly as a greenfield
project so we could try out some new ideas, including using LLMs during
development. *bine* is less sophisticated and was born from the idea of
replacing dependency management in Make-based projects like [`makego`].

[asdf]: https://asdf-vm.com/

### How does *bine* compare to mise?

mise is also ahead of bine in terms of usability and features. In fact, it may
even surpass asdf. If you haven't tried it yet, it's definitely worth a look.
mise can leverage existing asdf plugins like [asdf-golang] to `go install`
the Go tools your project needs.

[mise]: https://mise.jdx.dev/
[asdf-golang]: https://github.com/asdf-community/asdf-golang

### How does *bine* compare to Nix?

Nix provides fully reproducible and isolated development environments, while
bine focuses on simple version management for tools.

### How does *bine* integrate with integration or build systems?

We have [`examples`] showing hwo to use *bine* with [`make`] and [`just`].

### How does *bine* integrate with Fish shell?

For Fish shell users, you can create a simple function that not only sets up
your `PATH` but also enhances your prompt to show the current `bine` project
name.

Create a file named `bine-env.fish` in your Fish functions directory (e.g.,
`~/.config/fish/functions/bine-env.fish`) with the following content:

```fish
function bine-env
    go tool bine env | source

    if not functions -q original_fish_prompt
        functions -c fish_prompt original_fish_prompt
    end

    # Requires 'jq' to parse `.bine.json`.
    function fish_prompt
        original_fish_prompt
        set PROJECT_NAME (go tool bine config get project)
        echo -n "→ [$PROJECT_NAME] "
    end
end
```

You can now call `bine-env` from your project's root directory.


[releases page]: https://github.com/artefactual-labs/bine/releases
[`examples`]: ./examples
[`make`]: ./examples/make
[`just`]: ./examples/just
[`makego`]: https://github.com/bufbuild/makego/tree/main/make/go
