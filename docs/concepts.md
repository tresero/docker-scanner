# Concepts

How docker-scanner decides what's safe, what's not, and what to recommend.

## What counts as a pinned tag

docker-scanner uses an allowlist approach: a tag is considered safely pinned if it matches one of these patterns. Anything else is flagged.

| Pattern | Examples |
| --- | --- |
| Semver | `1`, `1.2`, `1.2.3`, `v1.2.3` |
| Semver with pre-release | `1.2.3-rc1`, `1.2.3-rc.1`, `1.2.3-beta.2` |
| Semver with build metadata | `1.2.3+build.5` |
| Version with variant suffix | `16-alpine`, `8.5-fpm`, `1.2.3-bookworm`, `v1.2.3-arm64` |
| Date-based | `2024.05.01`, `20240501`, `2024-05-01`, `24.05` |
| Git hash | `a1b2c3d` (7+ hex chars) |

Common floating tags that get flagged: `latest`, `main`, `master`, `dev`, `stable`, `nightly`, `edge`, `apache`, `fpm`, `alpine`, `bookworm`, `jammy`, `bullseye`.

This is intentionally an allowlist rather than a denylist. New floating tags appear all the time — `noble`, `lunar`, project-specific channels — and a denylist would always lag. The allowlist asks one question: does this tag *look like* a version? If not, warn.

## Running version detection

When Docker is available on the host, docker-scanner queries the Docker daemon to show the actual version running for each container. This is critical for identifying situations where:

- `latest` silently pulled a pre-release or nightly build
- The running version is newer than the recommended safe version (downgrade risk ⬇️)
- A major version jump has occurred (💥)

Some images don't expose their version through standard labels or tags. These are shown as `latest (unknown)` in the report — which is itself a reason to pin to a specific version.

## The 72-hour rule

By default, docker-scanner will not recommend a version that was published less than 3 days ago. The principle: let the early adopters find the landmines first.

Most supply chain attacks and broken releases are discovered and pulled within 24-48 hours. Waiting 72 hours before adopting a new version dramatically reduces your exposure.

You can adjust this with `-safe-days`:

```bash
# More cautious
./docker-scanner -safe-days 7

# Living dangerously
./docker-scanner -safe-days 0
```

The rule applies to both the registry path and the override path — the scanner won't recommend a release younger than `safe-days` from either source.

## What gets scanned

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

For registries marked "Fallback" (no manifest date support), the [overrides system](overrides.md) can point at upstream GitHub releases instead.

### Security checks

- Hardcoded passwords (`PASSWORD`, `DB_PASS`, `MYSQL_ROOT_PASSWORD`, etc.)
- Hardcoded API keys and tokens (`API_KEY`, `API_TOKEN`, `TOKEN`, etc.)
- Hardcoded secrets (`SECRET`, `SECRET_KEY`, etc.)
- Credentials embedded in URLs (`://user:pass@host`)
- Missing `.env` files when environment variables use `${VAR}` syntax

## Report symbols

The HTML report includes a legend, but for reference:

| Symbol | Meaning |
| --- | --- |
| ✅ | Image is pinned to a specific version |
| ⚠️ | Image uses a floating tag |
| 💥 | Recommended version is a major version jump |
| ⬇️ | Recommended version is older than running version |
| `latest (unknown)` | Container running but version couldn't be determined |
| `not running` | Image defined but no container running |
| `(age unknown)` | Recommended version exists but release date couldn't be fetched |