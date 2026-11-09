# docker-scanner

A security and version auditing tool for Docker Compose projects. Scans your compose files, flags floating tags, detects hardcoded secrets, and recommends safe versions based on the 72-hour rule.

## Why

Using `latest` tags in production is a security risk. You have no audit trail, no reproducibility, and breaking changes can be pulled in without warning. Hardcoded passwords in compose files are another common problem that gets worse over time.

docker-scanner finds these issues across all your projects in one run and gives you actionable recommendations.

### Why this matters — a real-world example

While building this tool, I discovered my n8n instance was running `latest` which had silently pulled version `2.18.3` — a pre-release build with a git hash suffix. The actual latest stable release was `2.17.7`. Because n8n runs database migrations on startup, I couldn't safely downgrade. I also couldn't switch to the `stable` tag because it points to `2.17.7`, which would be a downgrade.

My only option was to pin to `2.18.3` and move forward carefully. By the time you notice you're on the wrong version, it might be too late to go back.

## Install

### Pre-built binaries

Grab the latest release from [releases](https://github.com/tresero/docker-scanner/releases). Binaries are available for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64).

### From source

Requires Go 1.23 or later.

```bash
git clone https://github.com/tresero/docker-scanner.git
cd docker-scanner
go build -o docker-scanner .
```

## Quick start

```bash
# Scan current directory, text output
./docker-scanner

# Scan a specific directory
./docker-scanner -dir ~/docker-projects

# HTML report (good for email)
./docker-scanner -dir ~/docker-projects -format html -output report.html

# JSON output (pipe to jq, feed to monitoring)
./docker-scanner -dir ~/docker-projects -format json | jq '.images[] | select(.uses_latest)'

# More cautious — only recommend versions older than 7 days
./docker-scanner -dir ~/docker-projects -safe-days 7
```

Point `-dir` only at compose projects you trust, such as your own stacks. The scanner reads every compose file, `.env` and Dockerfile under it, follows symlinks, and contacts whatever registries those files name, so a project written by someone else can steer what it reads and where it connects.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-dir` | `.` | Root directory to scan for compose files |
| `-format` | `text` | Output format: `text`, `md`, `html`, or `json` |
| `-output` | stdout | Write report to file instead of stdout |
| `-safe-days` | `3` | Only recommend versions older than N days |
| `-skip-remote` | `false` | Skip registry version lookups |
| `-overrides` | none | Path to overrides YAML (opt-in, see [docs](docs/overrides.md)) |
| `-verbose` | `false` | Show detailed scan progress |
| `-version` | | Print the version and exit. A build from source prints `dev`. |

## What you get

- Recursive scan of `docker-compose.yml` and `compose.yml` files
- Flags floating tags (`latest`, `main`, `apache`, `bookworm`, etc.)
- Shows actual running container versions via the Docker daemon
- Detects hardcoded secrets, missing `.env` files
- Recommends safer versions with real, pullable Docker tags
- Status indicators: up to date, upgrade available, floating tag, downgrade
- Text, Markdown, HTML, or JSON reports
- Single binary, no runtime dependencies

## Documentation

- **[Concepts](docs/concepts.md)** — How version detection works, what counts as a pinned tag, the 72-hour rule, supported registries
- **[Version source overrides](docs/overrides.md)** — Optional override system for images where registry-based detection doesn't work
- **[Automated scanning with email](docs/automation.md)** — A daily security email for an Ubuntu server, with docker-scanner covering its Docker images
- **[Architecture and contributing](docs/architecture.md)** — Project structure, how to add registries, security checks, output formats

## License

MIT License. See [LICENSE](LICENSE) for details.

## Author

Jon Griffin ([@tresero](https://github.com/tresero))
