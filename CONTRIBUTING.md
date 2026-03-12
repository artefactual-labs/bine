# Contributing

## Releasing

Releases are published through the `Release` GitHub Actions workflow on
`main`. Use the Just helper to dispatch it.

Before running a release, make sure you are authenticated with the GitHub CLI
and have permission to run workflows in this repository:

```sh
gh auth status
```

Dispatch a release:

```sh
just release minor
just release patch
just release v1.2.3
```

Watch the most recent release run:

```sh
just release-watch
```

`minor` publishes the next minor version from the highest published release.
`patch` publishes the next patch version from the highest published release.
An exact version must use the `vMAJOR.MINOR.PATCH` format.
