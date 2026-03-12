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
lint *args="--fix": (install "golangci-lint")
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

# Dispatch a GitHub release workflow.
release target:
  #!/usr/bin/env bash
  set -euo pipefail

  case "{{ target }}" in
    minor)
      payload='{"publish_minor":true,"publish_patch":false,"version":""}'
      ;;
    patch)
      payload='{"publish_minor":false,"publish_patch":true,"version":""}'
      ;;
    v[0-9]*.[0-9]*.[0-9]*)
      payload='{"publish_minor":false,"publish_patch":false,"version":"{{ target }}"}'
      ;;
    *)
      echo "usage: just release minor|patch|vMAJOR.MINOR.PATCH" >&2
      exit 1
      ;;
  esac

  printf '%s\n' "$payload" | gh workflow run release.yml --ref main --json

# Watch the latest GitHub release workflow run.
release-watch:
  #!/usr/bin/env bash
  set -euo pipefail

  run_id="$(gh run list --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
  gh run watch "$run_id"
