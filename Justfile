#!/usr/bin/env -S just --justfile

# Lazy evaluation of variables is not supported yet!
GOLANGCI_LINT := `go tool bine get golangci-lint`
GO_MOD_OUTDATED := `go tool bine get go-mod-outdated`
TPARSE := `go tool bine get tparse`

[private]
default:
  @just --list --unsorted

# Lint the code.
lint:
  @{{GOLANGCI_LINT}} run

# Format the code.
fmt:
  @{{GOLANGCI_LINT}} fmt

# Print all available updates.
deps:
  @go list -u -m -json all | {{GO_MOD_OUTDATED}} -direct -update

# Print a coverage report.
cov file="":
  #!/usr/bin/env bash
  set -euo pipefail
  if [ -n "{{file}}" ]; then
    tmpfile="{{file}}"
  else
    tmpfile=$(mktemp)
  fi
  go test -cover -coverpkg=./... -coverprofile="$tmpfile" ./... 1>/dev/null
  go tool cover -func="$tmpfile"
  echo "coverprofile: $tmpfile (go tool cover -html=$tmpfile)"
