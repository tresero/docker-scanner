# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

docker-scanner is a Go 1.22 CLI that audits Docker Compose projects for floating tags, hardcoded secrets, and outdated images, then recommends versions under a "72-hour rule" (`-safe-days`, default 3). `gopkg.in/yaml.v3` is the only external dependency.

## Commands

```bash
go build -o docker-scanner .           # Build
go test ./... -v -race                 # All tests
go test ./pkg/registry/ -run TestPick  # Single test
go vet ./...                           # Vet (CI also runs golangci-lint with default config)
go run . -dir <path> -verbose          # Real scan against live registries
```

- `-version` prints `main.version`, which release builds set with `-ldflags -X main.version=<tag>` (see `docs/architecture.md`). Other builds print `dev`.
- Tests make no registry or GitHub calls, so confirm changes to live fetching with a real scan.
- The build output `docker-scanner` is gitignored. `dist/checksums.txt` is tracked and holds the latest release's checksums; update it with each release.
- `registry.go`, `checker.go`, and some other files are space-indented. Keep reformatting out of unrelated diffs.

## Architecture

`main.go` drives one pass per project: **scanner → parser → security → overrides/registry → report**. `recommend.go` applies the recommendation logic described below to each image.

### Recommendations

Every recommendation must be a real, pullable tag taken from registry data. Upstream version strings are never shown directly.

- **Registry path:** `registry.Lookup` → `FilterAndSortTags` or `FilterAndSortDockerHubTags` → `Pick`. Filtering drops arch-specific, pre-release, git-hash, and build-ID tags and removes partial tags (`5.0` when `5.0.2` exists). Pick gets the whole list; `DisplayVersions` caps `available_versions` at `config.VersionFetchLimit` (10) with a stricter filter.
- **Override path:** tried first when an image has an override. `overrides.Resolve` picks an upstream release, then `registry.MatchOverride` must find a registry tag that fits the same pin rules. Any failure or missing match falls through to the registry path.
- **Pin rules (`Pick`):** keep the pin's variant (`tagVariant`). A major or major.minor pin stays on its line and reports a higher major in `NewerMajor`. `StayOnMajor` does the same for databases at any pin, because a new major might not start on their data. Never go below the higher of the pin and the running version (`versionFloor`).
- **N-day rule:** take the newest stable tag older than the cutoff. If dates are known and nothing qualifies, recommend nothing and set `Note`. `positionFallback` runs only when no candidate has a date. Ages come from push dates, so rebuilds reset them (#25).
- **Clock:** `Pick` takes `Now`, so tests use a fixed clock.

### Image references

`parseFile` expands Compose variables first (`pkg/parser/variables.go`): shell, then the `.env` beside the compose file, then the default. An image with an unresolved variable keeps `${NAME}` in its tag, shows ❓, and skips the lookup.

A service with `build:` reports its Dockerfile's base image instead (`pkg/parser/dockerfile.go`), with no running version, because the built image's tag says nothing about its base. `SkipLookup` marks rows a registry can't answer, including images `MarkLocalOnly` finds were never pulled.

`registry.DataUpgradeWarning` adds a note on database images. Its wording comes from upgrading each database on a throwaway volume, so retest there before changing it.

### Tag classification

These rules live in separate places, so a change to what counts as a version may need more than one edit:

- Floating-tag flag (`UsesLatest`): the `looksLikeVersion` allowlist in `pkg/parser/parser.go`
- Recommendation candidates: `isCandidateTag` in `pkg/registry/semver.go`, which uses `preReleaseSuffixes` from `safepick.go`
- `available_versions`: `isCleanVersion` in `pkg/registry/semver.go`, which is stricter and uses `devSuffixes`
- Variants: `tagVariant` and `variantFits` in `pkg/registry/safepick.go`

### Registries

- `Lookup` uses the first handler whose `Supports` matches. `GenericRegistry` matches everything, so it must stay last in `DefaultRegistries()`.
- `GHCRRegistry` also claims `lscr.io`, but fetches those images from the Docker Hub mirror for newest-first ordering.
- GHCR lists tags roughly in push order, 1000 per page, so `fetchAllTags` reads every page: the newest versions are near the end. A partial list makes old tags look newest, so a failed page is an error and the image shows ❓. Each date costs 3 requests (manifest list → platform manifest → config blob), so `fillReleaseDates` dates at most 10 tags that fit the pin.
- Docker Hub lookups (`fetchHubTags`) read the first page of `name=.`, plus up to 3 pages of the pin's own filter when that isn't the full list. An older line on a very busy image can still be missing.
- `GenericRegistry` also uses `fetchAllTags`. It returns no dates, so its results go through `positionFallback`. The exception is a registry that answers 401 with a Docker Hub login challenge, such as `docker.n8n.io`: it's a front for Docker Hub, so it uses `fetchHubTags`.
- `TagBuilder` is optional. A handler that implements it tells `recommender.checkNewerBuild` what a tag points to, for the 🔄 newer-build check.

### Overrides

- `overrides.Load` precedence: `-overrides` path → `$XDG_CONFIG_HOME/docker-scanner/overrides.yml` → `~/.config/docker-scanner/overrides.yml` → embedded `defaults.yml`.
- The embedded defaults are deliberately empty because recommendations are registry-first.
- `github_release` is the only source. A new source needs a `Source` constant, a `validSources` entry, the supported-sources error message in `overrides.go`, and a case in `Resolve`. `overrides.Lookup` is legacy and only tests call it.

### Conventions

- `main.go` reads `XDG_CONFIG_HOME` and `HOME` and passes them in through `LoadOptions`. Packages take values as parameters; the one exception is `GITHUB_TOKEN`, read in `overrides.DefaultGetter`.
- `overrides.Getter` is a function type, so tests inject a fake instead of making HTTP calls.
- `parser.FindContainer` picks the container and its state, including `not created`. An empty state means Docker couldn't be asked.
- `parser.GetRunningVersion` shells out to the `docker` CLI through the `dockerOutput` variable, which tests fake. It returns an empty string when it finds nothing, and usually returns just the compose tag rather than the real version; `registry.ResolveRunning` finds the real one from the version label or digest.
- `recommender` in `recommend.go` holds the per-image logic, with injected registries, getter, and clock.
- `ImageInfo`'s JSON tags are public output read with `jq`, including by `scripts/security-report/security-report`. Add fields; renaming or removing one, or changing what it means, breaks those readers. `status` is filled in by `GenerateJSON` from `ClassifyImage`, the same verdict that colours the other reports.

## Docs

- `docs/concepts.md`: read when changing tag rules, registry support, security checks, or report symbols, and keep it in sync.
- `docs/overrides.md`: read when changing the override YAML schema or sources.
- `docs/architecture.md`: read when adding a registry, checker, or output format, or cutting a release. The interfaces in code are authoritative.
- `scripts/security-report/` is a copy of the script that runs on the servers, where it is maintained. Refresh the copy from a server instead of editing it here.
