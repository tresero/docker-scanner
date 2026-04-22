# docker-scanner

A security and version auditing tool for Docker Compose projects. Scans your compose files, flags images using `latest` tags, detects hardcoded secrets, and recommends safe versions based on the 72-hour rule.

## Why

Using `latest` tags in production is a security risk. You have no audit trail, no reproducibility, and breaking changes can be pulled in without warning. Hardcoded passwords in compose files are another common problem that gets worse over time.

docker-scanner finds these issues across all your projects in one run and gives you actionable recommendations.

## Features

- Recursively scans directories for `docker-compose.yml` and `compose.yml` files
- Flags images using `latest` or other unsafe tags (`main`, `dev`, `nightly`, `edge`)
- Detects hardcoded passwords, API keys, tokens, and secrets in environment variables
- Checks for missing `.env` files when `${VAR}` syntax is used
- Queries Docker Hub, GitHub Container Registry (GHCR), and LinuxServer (lscr.io) for available versions
- Recommends the newest stable version that has been published for at least 72 hours (configurable)
- Filters out pre-release tags (rc, beta, alpha, dev)
- Prefers clean semver tags over suffixed variants (e.g., `0.21.0` over `0.21.0-rocm`)
- Warns about major version jumps that may contain breaking changes
- Reports in text, markdown, or HTML format
- Single binary, no runtime dependencies, cross-platform

## Install

### From source

Requires Go 1.22 or later.

```bash
git clone https://github.com/tresero/docker-scanner.git
cd docker-scanner
go build -o docker-scanner .
```

### Cross-compile

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o docker-scanner-linux .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o docker-scanner-mac .

# Windows
GOOS=windows GOARCH=amd64 go build -o docker-scanner.exe .
```

## Usage

```bash
# Scan current directory, text output
./docker-scanner

# Scan a specific directory
./docker-scanner -dir ~/docker-projects

# Markdown report
./docker-scanner -dir ~/docker-projects -format md -output report.md

# HTML report (good for email)
./docker-scanner -dir ~/docker-projects -format html -output report.html

# Extra cautious — only recommend versions older than 7 days
./docker-scanner -dir ~/docker-projects -safe-days 7

# Skip remote registry lookups (offline mode)
./docker-scanner -dir ~/docker-projects -skip-remote

# Verbose output for debugging
./docker-scanner -dir ~/docker-projects -verbose
```

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-dir` | `.` | Root directory to scan for compose files |
| `-format` | `text` | Output format: `text`, `md`, or `html` |
| `-output` | stdout | Write report to file instead of stdout |
| `-safe-days` | `3` | Only recommend versions older than N days |
| `-skip-remote` | `false` | Skip registry version lookups |
| `-verbose` | `false` | Show detailed scan progress |

## The 72-Hour Rule

By default, docker-scanner will not recommend a version that was published less than 3 days ago. This is based on a simple principle: let the early adopters find the landmines first.

Most supply chain attacks and broken releases are discovered and pulled within 24-48 hours. Waiting 72 hours before adopting a new version dramatically reduces your exposure.

You can adjust this with `-safe-days`:

```bash
# More cautious
./docker-scanner -safe-days 7

# Living dangerously
./docker-scanner -safe-days 0
```

## What Gets Scanned

### Compose files

- `docker-compose.yml`
- `docker-compose.yaml`
- `compose.yml`
- `compose.yaml`

### Skipped directories

The scanner automatically skips `node_modules`, `vendor`, `.git`, and other dependency directories to avoid false positives from third-party compose files.

### Registries supported

| Registry | Tag listing | Release dates |
| --- | --- | --- |
| Docker Hub (`docker.io`) | Yes | Yes |
| GitHub Container Registry (`ghcr.io`) | Yes | Yes (via manifest) |
| LinuxServer (`lscr.io`) | Yes (via GHCR) | Yes (via manifest) |
| Google Container Registry (`gcr.io`) | Fallback | No |
| Other OCI registries | Fallback | No |

### Security checks

- Hardcoded passwords (`PASSWORD`, `DB_PASS`, `MYSQL_ROOT_PASSWORD`, etc.)
- Hardcoded API keys and tokens (`API_KEY`, `API_TOKEN`, `TOKEN`, etc.)
- Hardcoded secrets (`SECRET`, `SECRET_KEY`, etc.)
- Credentials embedded in URLs (`://user:pass@host`)
- Missing `.env` files when environment variables use `${VAR}` syntax

## Automated Scanning with Email

Create a `.env` file in the docker-scanner directory:

```
SCANNER=/home/you/docker-scanner/docker-scanner
SCAN_DIR=/home/you/docker-projects
MAIL_TO=you@example.com
SAFE_DAYS=3
FORMAT=html
```

Create `docker-scan-email.sh`:

```bash
#!/bin/bash
set -a
source /home/you/docker-scanner/.env
set +a

REPORT="/tmp/docker-scan-report.html"
SUBJECT="Docker Security Report - $(date +%Y-%m-%d)"

${SCANNER} -dir "${SCAN_DIR}" -format "${FORMAT}" -output "${REPORT}" -safe-days "${SAFE_DAYS}"

mail -a "Content-Type: text/html; charset=utf-8" \
     -s "${SUBJECT}" \
     "${MAIL_TO}" < "${REPORT}"

rm -f "${REPORT}"
```

Add to crontab:

```bash
chmod +x docker-scan-email.sh
crontab -e

# Weekly on Monday at 8am
0 8 * * 1 /home/you/docker-scanner/docker-scan-email.sh

# Or daily
0 8 * * * /home/you/docker-scanner/docker-scan-email.sh
```

Requires `mailutils` and a working MTA (postfix, msmtp, etc.) on the host.

## Project Structure

```
docker-scanner/
├── main.go                          # Entry point, CLI flags
├── pkg/
│   ├── models/
│   │   └── models.go                # Shared data structures
│   ├── scanner/
│   │   └── scanner.go               # Finds compose files
│   ├── parser/
│   │   └── parser.go                # Extracts images from YAML
│   ├── registry/
│   │   ├── registry.go              # Registry interface
│   │   ├── dockerhub.go             # Docker Hub implementation
│   │   ├── ghcr.go                  # GHCR + lscr.io implementation
│   │   ├── generic.go               # OCI fallback
│   │   ├── semver.go                # Version filtering and sorting
│   │   └── safepick.go              # 72-hour rule logic
│   ├── security/
│   │   ├── checker.go               # Security check interface
│   │   ├── envfile.go               # .env file checker
│   │   └── hardcoded.go             # Hardcoded secrets checker
│   └── report/
│       ├── report.go                # Text formatter
│       ├── markdown.go              # Markdown formatter
│       ├── mdutil.go                # Markdown helpers
│       ├── html.go                  # HTML formatter
│       └── templates/
│           └── report.html          # HTML email template
└── internal/
    └── config/
        └── config.go                # Configuration constants
```

## Contributing

Pull requests welcome. The codebase is designed to be extensible:

- **New registry**: Implement the `Registry` interface in `pkg/registry/`
- **New security check**: Implement the `Checker` interface in `pkg/security/`
- **New output format**: Add a formatter in `pkg/report/`

## License

MIT License. See [LICENSE](LICENSE) for details.

## Author

Jon Griffin ([@tresero](https://github.com/tresero))