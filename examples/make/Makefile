# We assume that go is already installed.
# We invoke `bine` via `go tool bine`, i.e. defined as a tool in `go.mod`.

# Include `bine path` in our system path.
# Only compatible with GNU make!
export PATH := $(shell go tool bine path):$(PATH)

default: test

tool-%:
	@echo Installing $*...
	@go tool bine get $* 1> /dev/null

# Test report.
test: tool-tparse
	go test -json -v ./... | tparse

# List direct dependencies with updates available.
deps: tool-go-mod-outdated
	go list -u -m -json all | go-mod-outdated -update -direct

# Generate goa code.
gen-goa: tool-goa
	goa --help
