# Version source overrides

By default, docker-scanner gets all version information directly from container registries (Docker Hub, GHCR, etc.). This works for the vast majority of images.

For rare cases where the registry doesn't expose usable version tags, you can configure overrides that point at upstream GitHub releases instead.

## When you might need overrides

Most images don't need overrides. The scanner handles:

- **Docker Hub** — tags with timestamps, newest first
- **GHCR** — reads the whole paginated tag list
- **lscr.io** — routed through Docker Hub for reliable newest-first ordering
- **Other OCI registries** — generic fallback

You might need an override if an image:

- Uses non-semver tags that can't be compared
- Has a registry that doesn't expose tags at all
- Publishes GitHub releases with version info not reflected in tags

## Creating an overrides file

```bash
mkdir -p ~/.config/docker-scanner
nano ~/.config/docker-scanner/overrides.yml
```

### Lookup order

1. `-overrides PATH` flag
2. `$XDG_CONFIG_HOME/docker-scanner/overrides.yml`
3. `~/.config/docker-scanner/overrides.yml`

## File format

```yaml
overrides:
  - image: docker.io/grafana/grafana
    source: github_release
    repo: grafana/grafana

  - image: ghcr.io/some-org/some-image
    source: github_release
    repo: some-org/some-repo
```

| Field | Description |
| --- | --- |
| `image` | Exact match against the image's `registry/name` from your compose file |
| `source` | Only `github_release` is supported in v1 |
| `repo` | The GitHub `owner/repo` to query |

When an override matches, the scanner:

1. Fetches the latest GitHub release for the repo
2. Applies the `safe-days` cutoff to the release
3. Cross-references against actual Docker registry tags to find a pullable tag. The tag must match the release's version exactly or as a whole version inside a longer tag (`4.0.17.2952` matches `4.0.17.2952-ls309`, but `1.2.1` doesn't match `1.2.10`), and the tag must also be older than `safe-days`. A rebuild pushed yesterday is a new image even when its release is old.
4. Falls back to the registry path if no matching Docker tag exists

This ensures recommendations are always real, pullable Docker tags. The age shown is the tag's own age when the registry dates it, and the release's age otherwise.

## GitHub rate limits

Anonymous GitHub API requests are limited to 60/hour. For larger fleets, set a `GITHUB_TOKEN` env var (5000/hour authenticated):

```bash
export GITHUB_TOKEN=ghp_yourtoken
./docker-scanner -dir ~/docker-projects
```

A read-only [fine-grained personal access token](https://github.com/settings/tokens?type=beta) with public-repo access is sufficient.

## Fallback behavior

If an override fails (GitHub unreachable, repo renamed, no matching Docker tag), the scanner falls back to registry-based detection automatically. In verbose mode, you'll see why:

```
Override v4.0.17.2967 has no matching Docker tag older than 3 days, falling back to registry
```
