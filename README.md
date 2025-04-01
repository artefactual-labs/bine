# bine

**bine** is a simple binary downloader for development environments.

At Artefactual, we manage multiple projects that use Makefiles, similar to those
suggested in https://github.com/bufbuild/makego, to ensure the availability of
various binaries locally. bine offers a simpler and more efficient alternative
based on a simple configuration file.

## Usage

You can install `bine` using `go get`:

    go get -tool github.com/artefactual-labs/bine@latest

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

For more examples, see the [`examples`](./examples) folder.
