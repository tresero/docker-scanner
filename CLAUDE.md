# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

docker-scanner is a Go CLI tool that audits Docker Compose projects for floating tags, hardcoded secrets, and outdated images. It recommends safe versions using a "72-hour rule" (only suggest versions older than N days). Single binary, only external dependency is `gopkg.in/yaml.v3`.

## Build and test commands

```bash
go build -o docker-scanner .          # Build
go test ./... -v -race                 # Run all tests
go test ./pkg/registry/ -run TestPick  # Run a single test
go vet ./...                           # Vet
# CI also runs golangci-lint
```

## Architecture

The pipeline flows through packages in order: **scanner → parser → security + registry/overrides → report**

- `main.go` — CLI flags, orchestration, override loading (keeps env access out of packages)
- `pkg/models/` — Shared data structures (`Image`, `ImageInfo`, `SecurityIssue`, `Project`, `RegistryVersion`)
- `pkg/scanner/` — Recursively finds compose files, groups by project directory
- `pkg/parser/` — Extracts images from compose YAML, queries Docker daemon for running versions
- `pkg/registry/` — `Registry` interface with implementations: DockerHub, GHCR (also handles lscr.io), GenericRegistry (OCI fallback). `safepick.go` applies the N-day rule; `semver.go` handles version filtering/sorting
- `pkg/overrides/` — Alternative version source for images where registry detection fails. Config loaded from explicit path → `$XDG_CONFIG_HOME/docker-scanner/overrides.yml` → `~/.config/...` → bundled `defaults.yml` (via `//go:embed`). Sources include GitHub releases and GitHub tags
- `pkg/security/` — `Checker` interface: hardcoded secret detection and .env file checks. Patterns defined in `internal/config/config.go`
- `pkg/report/` — Text, Markdown, and HTML formatters. HTML uses `html/template` with embedded template
- `internal/config/` — Constants: compose filenames, secret regex patterns, unsafe tag patterns, ignored directories, timeouts

## Extension points

- **New registry**: implement `Registry` interface (`Name`, `Supports`, `FetchVersions`), add to `DefaultRegistries()` in `registry.go`
- **New security check**: implement `Checker` interface (`Name`, `Check`), add to `DefaultCheckers()`
- **New output format**: add formatter in `pkg/report/` taking `[]models.ImageInfo` → string, wire into `main.go` format switch
- **New override source**: add `Source` constant in `overrides.go`, register in `validSources`, add case in `resolve.go`

## Key conventions

- Go 1.22 module (`docker-scanner`), no Makefile — use `go` commands directly
- Tabs for indentation (standard Go formatting)
- Packages under `pkg/` are the public API; `internal/config/` holds constants
- Environment variable access is restricted to `main.go` — packages receive values via function parameters
- The `overrides` package uses a `Getter` interface to abstract HTTP calls for testability
