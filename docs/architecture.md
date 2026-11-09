# Architecture and contributing

## Project structure

```
docker-scanner/
├── main.go                          # Entry point, CLI flags, override loading
├── pkg/
│   ├── models/
│   │   └── models.go                # Shared data structures
│   ├── scanner/
│   │   └── scanner.go               # Finds compose files
│   ├── parser/
│   │   └── parser.go                # Extracts images from YAML, queries Docker daemon
│   ├── registry/
│   │   ├── registry.go              # Registry interface + tag matching
│   │   ├── dockerhub.go             # Docker Hub implementation
│   │   ├── ghcr.go                  # GHCR + lscr.io implementation (with pagination)
│   │   ├── generic.go               # OCI fallback
│   │   ├── semver.go                # Version filtering and sorting
│   │   └── safepick.go              # 72-hour rule logic
│   ├── overrides/
│   │   ├── overrides.go             # Override config + GitHub release parsing
│   │   ├── load.go                  # File location resolution
│   │   ├── resolve.go               # Override → recommendation
│   │   ├── http.go                  # HTTP getter with GITHUB_TOKEN support
│   │   └── defaults.yml             # Bundled defaults (empty by default)
│   ├── security/
│   │   ├── checker.go               # Security check interface
│   │   ├── envfile.go               # .env file checker
│   │   └── hardcoded.go             # Hardcoded secrets checker
│   └── report/
│       ├── report.go                # Text formatter + status classification
│       ├── markdown.go              # Markdown formatter
│       ├── json.go                  # JSON formatter
│       ├── mdutil.go                # Markdown helpers
│       ├── html.go                  # HTML formatter (html/template)
│       └── templates/
│           └── report.html          # HTML report template
└── internal/
    └── config/
        └── config.go                # Configuration constants
```

## Scan flow

For each project found by the scanner:

1. **Parse** the compose file → list of images
2. **Get running version** for each image from the Docker daemon
3. **Run security checks** against the compose file
4. **For each image**:
   - If an override is configured and matches, cross-reference the upstream version against actual Docker registry tags. Fall back to the registry path if no matching tag exists.
   - Otherwise, query the registry directly for available versions
   - Apply `safeDays` cutoff to pick a recommendation
5. **Classify** each image: up to date, upgrade available, floating tag, or downgrade
6. **Format** results as text/markdown/HTML/JSON

## Registry strategies

Different registries require different approaches:

- **Docker Hub** returns tags ordered by `last_updated` (newest first), with timestamps. Straightforward.
- **GHCR** returns tags roughly in push order, oldest first, capped at 1000 per page. The newest versions are near the end, so the scanner reads every page, up to 100. If it can't read the whole list, the image shows as not verified, because a partial list makes old tags look like the newest ones.
- **lscr.io** images are routed to Docker Hub (where they're mirrored) to get newest-first ordering.

## Contributing

Pull requests welcome. The codebase is designed to be extensible at several layers.

### Adding a new registry

Implement the `Registry` interface in `pkg/registry/`:

```go
type Registry interface {
    Name() string
    Supports(registry string) bool
    FetchVersions(image models.Image) ([]models.RegistryVersion, error)
}
```

Add the new registry to `DefaultRegistries()` in `registry.go`.

### Adding a new security check

Implement the `Checker` interface in `pkg/security/`:

```go
type Checker interface {
    Name() string
    Check(filePath string) []models.SecurityIssue
}
```

Add to `DefaultCheckers()`.

### Adding a new output format

Add a formatter in `pkg/report/` that takes `[]models.ImageInfo` and returns a string. Wire it into `main.go`'s format switch.

### Adding a new override source

Add a `Source` constant in `pkg/overrides/overrides.go`, register it in `validSources`, and add a `case` in the `Resolve` switch in `resolve.go`. Add tests for the parser following the existing `parseLatestRelease` pattern.

## Testing

```bash
go test ./...
```

## Cross-compiling for releases

Use `scripts/release.sh`, so the binaries, `dist/checksums.txt` and the published assets always match:

```bash
scripts/release.sh build v0.3.9      # vet, test, clean build of all 5 binaries, checksums.txt
git add dist/checksums.txt && git commit -m 'Update dist/checksums.txt to v0.3.9' && git push
scripts/release.sh publish v0.3.9    # gh release create, then verify
scripts/release.sh verify v0.3.9     # recheck the published assets at any time
```

`build` stops on the first failure. It refuses a dirty tree or an existing tag, and deletes every old binary first, so `checksums.txt` can only describe binaries built from the committed source in that run. It stamps the version with `-X main.version` (a build without it reports `dev`) and checks that the host's binary reports it. `publish` re-checks `dist/` against `checksums.txt`, and refuses if `checksums.txt` isn't committed or commits aren't pushed. It then uploads the binaries and `checksums.txt` and compares every published asset's digest with `checksums.txt`.
