<p align="left">
  <a href="https://github.com/artefactual-labs/bine/releases/latest"><img src="https://img.shields.io/github/v/release/artefactual-labs/bine.svg?color=orange"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg"/></a>
  <a href="https://codecov.io/gh/artefactual-labs/bine"><img src="https://img.shields.io/codecov/c/github/artefactual-labs/bine"/></a>
</p>

# bine

**bine** is a simple binary downloader for development environments.

## Background

At Artefactual, we manage multiple projects that rely on command-line tools to
support development workflows. While `go get -tool` is a great feature for
managing Go module-based binaries, it doesn't help with other essential tools
that aren't written in Go, e.g. `jq`, `shfmt`... that's where `bine` comes in!

## Installation

You can download the binary manually from the [releases page].

### Using Go

If you're working on a Go project, the best way to install bine is with
`go get`:

    go get -tool github.com/artefactual-labs/bine@latest

This adds bine as a tool in your Go module dependencies.

You can then run it with:

    $ go tool bine path
    /home/ethan/.cache/bine/project/linux/amd64/bin

To find where Go installed bine:

    $ go tool -n bine
    /home/ethan/.cache/go-build/bb/bbda4ebec23099ffe35c0961f5e0adb9c2970d1a9bbc1893e91a05ad96a310ef-d/bine

## Configuration

Create you configuration file, e.g.:

```json
{
    "project": "bine",
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

Then use bine to download and run the binary for your platform:

    $ bine run golangci-lint -- version
    golangci-lint has version 2.0.2 built with go1.24.1 from 2b224c2c on 2025-03-25T21:36:18Z

Or use bine to get the path to the binary:

    $ bine get golangci-lint
    /home/ethan/.cache/bine/project/linux/amd64/bin/golangci-lint

For more examples, see the [`examples`] folder.

### Go module support

Besides downloading pre-built assets, bine can also help manage binaries that
are Go modules installable via `go install`.

For an example, check out how the [`make`] uses bine to install a binary
directly from a Go module.

### Variable expansion

The `asset_pattern` field in the configuration supports variable expansion to help construct the correct asset filename for download. Bine replaces placeholders within this pattern based on the binary's definition and the environment where `bine` is run.

The following variables are supported:

* `{name}`: The value of the `name` field for the specific binary.
* `{version}`: The value of the `version` field for the specific binary.
* `{goos}`: The Go runtime operating system identifier (e.g., `linux`, `darwin`,
  `windows`). Determined by Go's `runtime.GOOS`.
* `{goarch}`: The Go runtime architecture identifier (e.g., `amd64`, `arm64`).
  Determined by Go's `runtime.GOARCH`.
* `{os}`: The operating system name as reported by `uname -s` (e.g., `Linux`,
  `Darwin`).
* `{arch}`: The machine hardware name as reported by `uname -m` (e.g., `x86_64`,
  `arm64`).

## Authenticating to the GitHub Rest API

If you downloading binaries from GitHub, you may need to authenticate to the
GitHub Rest API to avoid rate limiting. This is especially important if you
are using `bine list --outdated` to check for outdated binaries.

There are two ways to pass the token to bine:

```
# Using an environment variable:
BINE_GITHUB_API_TOKEN=token bine list --outdated

# Using a command-line flag:
bine list --outdated --github-api-token=token
```

[releases page]: https://github.com/artefactual-labs/bine/releases
[`examples`]: ./examples
[`make`]: ./examples/make
