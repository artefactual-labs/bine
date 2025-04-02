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

[releases page]: https://github.com/artefactual-labs/bine/releases
[`examples`]: ./examples
