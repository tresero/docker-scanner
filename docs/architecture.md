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
│   │   ├── registry.go              # Registry interface
│   │   ├── dockerhub.go             # Docker Hub implementation
│   │   ├── ghcr.go                  # GHCR + lscr.io implementation
│   │   ├── generic.go               # OCI fallback
│   │   ├── semver.go                # Version filtering and sorting
│   │   └── safepick.go              # 72-hour rule logic
│   ├── overrides/
│   │   ├── overrides.go             # Override config + GitHub release parsing
│   │   ├── load.go                  # File location resolution
│   │   ├── resolve.go               # Override → recommendation
│   │   ├── http.go                  # HTTP getter with GITHUB_TOKEN support
│   │   └── defaults.yml             # Bundled defaults (//go:embed)
│   ├── security/
│   │   ├── checker.go               # Security check interface
│   │   ├── envfile.go               # .env file checker
│   │   └── hardcoded.go             # Hardcoded secrets checker
│   └── report/
│       ├── report.go                # Text formatter
│       ├── markdown.go              # Markdown formatter
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
   - Check overrides — if matched, use that path
   - Otherwise call the registry interface
   - Apply `safeDays` cutoff to pick a recommendation
5. **Format** results as text/markdown/HTML

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

### Adding to bundled override defaults

Edit `pkg/overrides/defaults.yml` and submit a PR. Defaults should target images that:

- Are widely used (especially in home-server setups)
- Have version-detection issues on the registry path
- Publish proper GitHub releases (not `github_tag` — that's v2)

### Adding a new override source

Add a `Source` constant in `pkg/overrides/overrides.go`, register it in `validSources`, and add a `case` in the `Resolve` switch in `resolve.go`. Add tests for the parser following the existing `parseLatestRelease` pattern.

## Testing

```bash
go test ./...
```

Test coverage is currently strongest in `pkg/overrides`, `pkg/parser`, and `pkg/registry`. Adding tests when extending is appreciated but not required for small changes.

## Cross-compiling for releases

```bash
mkdir -p dist
GOOS=linux GOARCH=amd64 go build -o dist/docker-scanner-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o dist/docker-scanner-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o dist/docker-scanner-darwin-arm64 .
GOOS=darwin GOARCH=amd64 go build -o dist/docker-scanner-darwin-amd64 .
GOOS=windows GOARCH=amd64 go build -o dist/docker-scanner-windows-amd64.exe .
cd dist && sha256sum docker-scanner-* > checksums.txt && cd ..
```

Then `gh release create` with the binaries attached.