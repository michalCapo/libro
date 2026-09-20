# Project Instructions

## Checks

- Run `make check` before finishing code changes.
- The target checks `go fix`, formatting, `go vet`, `staticcheck`, `gopls`, `golangci-lint`, dead code, builds, and tests.
- It is non-mutating and continues after failures so all problems are reported in one run.

## Commands

- Run `make` to list available commands.
- Run `make install` to build and install Libro locally.
- Run `make release-dry-run` to preview a release.
- Run `make release` to bump, build, tag, push, and publish a release.
