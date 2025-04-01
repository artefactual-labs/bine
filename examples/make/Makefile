# We assume that go and bine have been already installed.

# Include `bine path` in our system path.
export PATH := $(shell bine path):$(PATH)

default: test

# Install tparse using bine.
tparse:
	@echo Installing tparse...
	@bine get tparse > /dev/null

# Install go-mod-outdated using bine.
go-mod-outdated:
	@echo Installing go-mod-outdated...
	@bine get go-mod-outdated > /dev/null

# Test report.
test: tparse
	@echo Running tparse...
	@go test -json -v ./... | tparse

# List direct dependencies with updates available.
deps: go-mod-outdated
	@echo Running go + go-mod-outdated...
	@go list -u -m -json all | go-mod-outdated -update -direct
