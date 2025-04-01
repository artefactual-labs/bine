# bine

**bine** is a simple binary downloader for development environments.

## Background

At Artefactual, we manage multiple projects that rely on command-line tools to
support development workflows. While [`go get -tool`] is a great feature for
managing Go module-based binaries, it doesn't help with other essential tools
that aren't written in Go, e.g. `jq`, `shfmt`... that's where `bine` comes in!

## Installation

> [!NOTE]
> For the time being, precompiled binaries are not available, so you'll need a
> Go compiler to install bine.

You can install `bine` using `go get`:

    go get -tool github.com/artefactual-labs/bine@latest

This way the tool is managed as part of your module dependencies.

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

Then use `bine` to download and run the binary for your platform:

    $ go tool bine run golangci-lint -- version
    golangci-lint has version 2.0.2 built with go1.24.1 from 2b224c2c on 2025-03-25T21:36:18Z

Or use `bine` to get the path to the binary:

    $ go tool bine get golangci-lint
    /home/ethan/.cache/bine/bine/linux/amd64/bin/golangci-lint

For more examples, see the [`examples`] folder.

[`go get -tool`]: https://go.dev/doc/toolchain#go-get-tool
[`examples`]: ./examples
