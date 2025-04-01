#!/usr/bin/env -S just --justfile

GOLANGCI_LINT := `go tool bine get golangci-lint`
GO_MOD_OUTDATED := `go tool bine get go-mod-outdated`

[private]
default:
  @just --list --unsorted

lint:
  @{{GOLANGCI_LINT}} run

fmt:
  @{{GOLANGCI_LINT}} fmt

deps:
  @go list -u -m -json all | {{GO_MOD_OUTDATED}} -direct -update
