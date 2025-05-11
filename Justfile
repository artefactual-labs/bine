#!/usr/bin/env -S just --justfile

bine_path := `go tool bine path`
export PATH := bine_path + ":" + env_var("PATH")

[private]
default:
  @just --list --unsorted

[private]
install tool:
  @echo "Installing {{ tool }}..."
  @go tool bine get {{ tool }} 1> /dev/null

# Test the code.
test *args: (install "gotestsum")
  gotestsum --format=testdox {{ args }}

# Lint the code.
lint *args: (install "golangci-lint")
  golangci-lint run {{ args }}

# Format the code.
fmt *args: (install "golangci-lint")
  golangci-lint fmt {{ args }}

# Print all available updates.
deps: (install "go-mod-outdated")
  go list -u -m -json all | go-mod-outdated -direct -update

# Print a coverage report.
cov file="coverage.txt": (install "gotestsum")
  #!/usr/bin/env bash
  set -euo pipefail
  if [ -n "{{file}}" ]; then
    tmpfile="{{file}}"
  else
    tmpfile=$(mktemp)
  fi
  gotestsum --format=testdox --junitfile junit.xml -- -cover -coverpkg=./... -coverprofile="$tmpfile" ./...
  go tool cover -func="$tmpfile"
  echo "coverprofile: $tmpfile (go tool cover -html=$tmpfile)"
