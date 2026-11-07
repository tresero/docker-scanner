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
- The running version is ahead of every safe version, so nothing is recommended
- A major version jump has occurred (💥)

Some images don't expose their version through standard labels or tags. These are shown as `latest (unknown)` in the report — which is itself a reason to pin to a specific version.

### Container state

The report also says what state each container is in:

| State | Meaning | Shown as |
|---|---|---|
| `running` | Up, with its health check passing, starting, or not defined | The version alone |
| `unhealthy` | Up, but failing its health check | 🚨 |
| `restarting` | In a restart loop | 🚨 |
| `exited` | Stopped, with its exit code | 🚨 if the code isn't 0 |
| `dead` | Docker couldn't remove or stop it cleanly | 🚨 |
| `paused`, `created` | Paused, or created but never started | Muted |
| `not created` | In compose, with no container | Muted |

Containers are found by `container_name`, by the name Compose generates, or by Compose's labels for that service and folder. If Docker can't be reached, the report says `not running` and the JSON leaves the state out, rather than calling everything never-created.

In JSON, these are `container_state`, `exit_code` and `health` on each image, plus `container_problem_count` for the 🚨 total. They don't affect the version status or the other counts.

### Newer builds

A tag can be re-pushed with the same name, often to pick up base image security fixes. When a container is running, the scanner compares the running image's digest with what its compose tag points to on the registry. If they differ, the report shows 🔄 with the push date and the running image's build date, when they're known.

It says nothing when it can't be sure: a locally built image has no digest to compare, a container on a different tag than compose says would always differ, and a tag pushed before the running image was built isn't an upgrade.

In JSON, these are `newer_build_available`, `newer_build_date` and `running_build_date` on each image, plus `newer_build_count`. They don't affect the version status or the other counts. Each running image takes one more registry request, or three on GHCR.

### Tag mismatches

If compose was edited but the container wasn't recreated, the next `docker compose up -d` will change the running version, possibly during unrelated maintenance. The report shows 🔀 when the compose tag and what the container was created from are both versions and differ, for example `compose 1.2.4, running 1.2.3: next up -d will change this`. It says `downgrade` when the move would be to a lower version.

The real running version is used when it's known, so `16-alpine` that's really 16.15 isn't a mismatch with itself. Floating tags are left to the newer-build check.

In JSON, these are `tag_mismatch` and `tag_mismatch_downgrade` on each image, plus `tag_mismatch_count`.

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

The rule applies to all version sources — the scanner won't recommend a release younger than `safe-days`.

Tag ages come from the registry's push date, not the release date. A rebuilt image counts as new, so an upgrade to a months-old version can be held back after a re-push.

If every tag with a known date is too new, the scanner recommends nothing and the report says why. Only when a registry provides no dates at all does it fall back to skipping the newest tags by position, shown as `(age unknown)`.

## How a recommendation is chosen

The scanner considers every usable tag it reads, and shows the newest 10 as available versions. On Docker Hub it reads the most recently pushed version tags, plus up to 3 pages of tags matching your pin's major or variant, so a very busy image can still have older tags it doesn't see. How far it can move depends on what you pinned:

| Pin | Recommendation |
| --- | --- |
| `8-alpine` (major) | Newest safe tag on 8.x. A newer major is reported separately with 💥 |
| `11.4` (major.minor) | Newest safe tag on the 11.4 line. A newer major is reported separately with 💥 |
| `11.4.8` (full version) | Newest safe tag. A different major is flagged with 💥 |

A tag like `16-alpine` or `latest` doesn't say what's actually running, so the scanner checks the image's version label, then looks for a registry tag with the same digest as the running image. If neither identifies it, for example because the registry has rebuilt that version since you pulled, the image shows as not verified and the note names the tag it would otherwise recommend.

A recommendation is never below the higher of your pin and the running version. If you're already on the newest version it's reported as up to date, whatever the tag's age.

The pinned variant is kept: `8-alpine` only gets `-alpine` tags, and `latest` or a plain version only gets plain tags. The one exception is moving between Alpine releases, such as `-alpine3.23` to `-alpine3.24`, and only at a higher version.

## What gets scanned

### Compose files

- `docker-compose.yml`
- `docker-compose.yaml`
- `compose.yml`
- `compose.yaml`

Variables in `image:` are expanded the way Compose does it: `${VAR}`, `$VAR`, `${VAR:-default}`, `${VAR-default}`, `${VAR:?error}` and `${VAR:+alternate}`. Values come from the shell, then from the `.env` file beside the compose file. If a variable has no value, the image shows as not verified (❓), the note names the variable, and no lookup is made.

### Skipped directories

The scanner automatically skips `node_modules`, `vendor`, `.git`, and other dependency directories to avoid false positives from third-party compose files.

### Registries supported

| Registry | Tag listing | Release dates |
| --- | --- | --- |
| Docker Hub (`docker.io`) | Yes | Yes |
| GitHub Container Registry (`ghcr.io`) | Yes (paginated) | Yes (via manifest) |
| LinuxServer (`lscr.io`) | Yes (via Docker Hub) | Yes |
| Google Container Registry (`gcr.io`) | Fallback | No |
| Other OCI registries | Fallback | No |

For registries marked "Fallback" (no manifest date support), the optional [overrides system](overrides.md) can point at upstream GitHub releases instead.

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
| ✅ | Up to date — pinned to the latest safe version |
| 🟡 | Upgrade available — a newer safe version exists |
| 🔴 | Floating tag — uses `latest` or other unpinned tag |
| ❓ | Not verified — the scan couldn't confirm this image's status, and the note says why |
| 🔄 | Newer build: the compose tag now points to a newer image than the one running |
| 🔀 | Compose pins a different version than the container is running |
| 🚨 | Container needs attention: unhealthy, restarting, dead, or exited with an error |
| 💥 | Major version jump in the recommendation |
| `not running` | Image defined but no container running |
| `(age unknown)` | Recommended version exists but release date couldn't be fetched |