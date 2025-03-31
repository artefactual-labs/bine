#!/usr/bin/env -S just --justfile

GOLANGCI_LINT := `go tool bine get golangci-lint`

lint:
  @{{GOLANGCI_LINT}} run

fmt:
  @{{GOLANGCI_LINT}} fmt
