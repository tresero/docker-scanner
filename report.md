# Docker Compose Security & Version Report

**Generated:** 2026-04-27 14:59:22

---

## Summary

| Metric | Count |
| --- | --- |
| Projects scanned | 21 |
| Images found | 33 |
| Using `latest` tag | **25** :warning: |
| Security issues | **8** :warning: |

## Images

| Status | Project | Service | Image | Tag | Recommended |
| --- | --- | --- | --- | --- | --- |
| :warning: | ollama | ollama | `docker.io/ollama/ollama` | `latest` | `0.21.2` (3 days) |
| :white_check_mark: | ollama | open-webui | `ghcr.io/open-webui/open-webui` | `main` | - |
| :warning: | postgres | pgadmin | `docker.io/dpage/pgadmin4` | `latest` | `9.14.0` (3 weeks) |
| :white_check_mark: | giga-directus | directus | `docker.io/directus/directus` | `11.17.3` | `11.17.3` (1 week) |
| :warning: | hledger-massage | hledger-web | `docker.io/dastapov/hledger` | `latest` | `1.52` (1 month) |
| :warning: | hledger-mayuli | hledger-web | `docker.io/dastapov/hledger` | `latest` | `1.52` (1 month) |
| :warning: | hledger-safira | hledger-web | `docker.io/dastapov/hledger` | `latest` | `1.52` (1 month) |
| :white_check_mark: | jongriffinmusic-directus | directus | `docker.io/directus/directus` | `11.17.3` | `11.17.3` (1 week) |
| :white_check_mark: | nextcloud | nextcloud | `docker.io/nextcloud` | `stable` | `33.0.1` (1 month) |
| :white_check_mark: | nextcloud | nextcloud_redis | `docker.io/redis` | `7-alpine` | `8.6.2` (3 days) |
| :warning: | salsablanca | directus | `docker.io/directus/directus` | `latest` | `11.17.3` (1 week) |
| :warning: | tunnel | cloudflared | `docker.io/cloudflare/cloudflared` | `latest` | `2026.3.0` (1 month) |
| :warning: | safira-office-directus | directus | `docker.io/directus/directus` | `latest` | `11.17.3` (1 week) |
| :warning: | vikunja | vikunja | `docker.io/vikunja/vikunja` | `latest` | `2.3` (2 weeks) |
| :warning: | caddy | caddy | `ghcr.io/serfriz/caddy-cloudflare` | `latest` | `2.11.1` (age unknown) |
| :warning: | homepage | homepage | `ghcr.io/gethomepage/homepage` | `latest` | `v1.4.4` (age unknown) |
| :warning: | media-server | radarr | `lscr.io/linuxserver/radarr` | `latest` | `3.0.0.4000-ls29` (age unknown) |
| :warning: | media-server | prowlarr | `lscr.io/linuxserver/prowlarr` | `latest` | - |
| :warning: | media-server | bazarr | `lscr.io/linuxserver/bazarr` | `latest` | `v0.9.0.6-ls95` (age unknown) |
| :warning: | media-server | qbittorrent | `lscr.io/linuxserver/qbittorrent` | `latest` | - |
| :warning: | media-server | beets | `lscr.io/linuxserver/beets` | `latest` | `1.4.9-ls76` (age unknown) |
| :warning: | media-server | navidrome | `docker.io/deluan/navidrome` | `latest` | `0.61.2` (2 weeks) |
| :warning: | media-server | jellyfin | `docker.io/jellyfin/jellyfin` | `latest` | `10.11.8` (3 weeks) |
| :warning: | media-server | sonarr | `lscr.io/linuxserver/sonarr` | `latest` | `3.0.4.999-ls73` (age unknown) |
| :warning: | monitoring-hub | grafana | `docker.io/grafana/grafana` | `latest` | `13.0.1` (1 week) |
| :white_check_mark: | monitoring-hub | cadvisor | `gcr.io/cadvisor/cadvisor` | `v0.49.1` | `v0.54.1` (age unknown) |
| :warning: | monitoring-hub | prometheus | `docker.io/prom/prometheus` | `latest` | `v3.11.2` (2 weeks) |
| :white_check_mark: | paperless | broker | `docker.io/library/redis` | `7` | `8.6.2` (3 days) |
| :warning: | paperless | paperless | `ghcr.io/paperless-ngx/paperless-ngx` | `latest` | `2.9.0` (age unknown) |
| :white_check_mark: | ob1-mcp | ob1-mcp | `docker.io/node` | `20-slim` | `25.9.0` (4 days) |
| :warning: | music-assets-directus | directus | `docker.io/directus/directus` | `latest` | `11.17.3` (1 week) |
| :warning: | salsablanca-directus | directus | `docker.io/directus/directus` | `latest` | `11.17.3` (1 week) |
| :warning: | stirling-pdf | stirling-pdf | `docker.io/stirlingtools/stirling-pdf` | `latest` | `2.10.0` (3 days) |

## :warning: Images Using `latest`

Using `latest` is a security risk because:

- Builds are not reproducible
- Breaking changes can be pulled in without warning
- No audit trail of which version is running

### `docker.io/ollama/ollama` (ollama)

- **Service:** ollama
- **File:** `/home/jon/docker-projects/ollama/compose.yml`
- **Recommended:** `0.21.2` (released 3 days ago)
- **Available versions:** `0.21.3-rc0`, `0.21.2`, `0.21.2-rocm`, `0.21.2-rc1`, `0.21.2-rc0`, `0.21.1`, `0.21.1-rocm`, `0.21.1-rc1`, `0.21.1-rc0`, `0.21.0`

### `docker.io/dpage/pgadmin4` (postgres)

- **Service:** pgadmin
- **File:** `/home/jon/docker-projects/postgres/compose.yml`
- **Recommended:** `9.14.0` (released 3 weeks ago)
- **Available versions:** `9.14.0`, `9.14`, `9.13.0`, `9.13`, `9.12.0`, `9.12`, `9.11.0`, `9.11`, `9.10.0`, `9.10`

### `docker.io/dastapov/hledger` (hledger-massage)

- **Service:** hledger-web
- **File:** `/home/jon/docker-projects/hledger-massage/docker-compose.yml`
- **Recommended:** `1.52` (released 1 month ago)
- **Available versions:** `1.52`, `1.52-dev`, `1.51.2`, `1.51.2-dev`, `1.51.1`, `1.51.1-dev`, `1.51`, `1.51-dev`, `1.50.3`, `1.50.3-dev`

### `docker.io/dastapov/hledger` (hledger-mayuli)

- **Service:** hledger-web
- **File:** `/home/jon/docker-projects/hledger-mayuli/docker-compose.yml`
- **Recommended:** `1.52` (released 1 month ago)
- **Available versions:** `1.52`, `1.52-dev`, `1.51.2`, `1.51.2-dev`, `1.51.1`, `1.51.1-dev`, `1.51`, `1.51-dev`, `1.50.3`, `1.50.3-dev`

### `docker.io/dastapov/hledger` (hledger-safira)

- **Service:** hledger-web
- **File:** `/home/jon/docker-projects/hledger-safira/docker-compose.yml`
- **Recommended:** `1.52` (released 1 month ago)
- **Available versions:** `1.52`, `1.52-dev`, `1.51.2`, `1.51.2-dev`, `1.51.1`, `1.51.1-dev`, `1.51`, `1.51-dev`, `1.50.3`, `1.50.3-dev`

### `docker.io/directus/directus` (salsablanca)

- **Service:** directus
- **File:** `/home/jon/docker-projects/salsablanca/docker-compose.yml`
- **Recommended:** `11.17.3` (released 1 week ago)
- **Available versions:** `11.17.3`, `11.17.2`, `11.17.1`, `11.17.0`, `11.17`, `11.16.1`, `11.16`, `11.16.0`, `11.15.4`, `11.15.3`

### `docker.io/cloudflare/cloudflared` (tunnel)

- **Service:** cloudflared
- **File:** `/home/jon/docker-projects/tunnel/docker-compose.yml`
- **Recommended:** `2026.3.0` (released 1 month ago)
- **Available versions:** `2026.3.0`, `2026.3.0-arm64`, `2026.3.0-amd64`, `2026.2.0`, `2026.2.0-arm64`, `2026.2.0-amd64`, `2026.1.2`, `2026.1.2-arm64`, `2026.1.2-amd64`, `2026.1.1`

### `docker.io/directus/directus` (safira-office-directus)

- **Service:** directus
- **File:** `/home/jon/docker-projects/safira-office-directus/docker-compose.yml`
- **Recommended:** `11.17.3` (released 1 week ago)
- **Available versions:** `11.17.3`, `11.17.2`, `11.17.1`, `11.17.0`, `11.17`, `11.16.1`, `11.16`, `11.16.0`, `11.15.4`, `11.15.3`

### `docker.io/vikunja/vikunja` (vikunja)

- **Service:** vikunja
- **File:** `/home/jon/docker-projects/vikunja/docker-compose.yml`
- **Recommended:** `2.3` (released 2 weeks ago)
- **Available versions:** `2.3`, `2.3.0`, `2.2.2`, `2.2.1`, `2.2`, `2.2.0`, `2.1`, `2.1.0`, `2.0`, `2.0.0`

### `ghcr.io/serfriz/caddy-cloudflare` (caddy)

- **Service:** caddy
- **File:** `/home/jon/docker-projects/caddy/compose.yml`
- **Recommended:** `2.11.1` (age unknown)
- **Available versions:** `2.11.2`, `2.11.1`, `2.11`, `2.10.2`, `2.10.0`, `2.10`, `2.9.1`, `2.9`, `2.8.4`, `2.8.1`

### `ghcr.io/gethomepage/homepage` (homepage)

- **Service:** homepage
- **File:** `/home/jon/docker-projects/homepage/docker-compose.yml`
- **Recommended:** `v1.4.4` (age unknown)
- **Available versions:** `v1.4.5`, `v1.4.4`, `v1.4.3`, `v1.4.2`, `v1.4.1`, `v1.4`, `v1.4.0`, `v1.3.2`, `v1.3.1`, `v1.3`

### `lscr.io/linuxserver/radarr` (media-server)

- **Service:** radarr
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`
- **Recommended:** `3.0.0.4000-ls29` (age unknown)
- **Available versions:** `3.0.0.3790-ls29`, `3.0.0.4000-ls29`, `3.0.0.3978-ls27`, `3.0.0.3790-ls28`, `3.0.0.4037-ls29`, `3.0.0.3987-ls27`, `3.0.0.4005-ls29`, `3.0.0.3989-ls28`, `3.0.0.3986-ls27`, `3.0.0.3989-ls29`

### `lscr.io/linuxserver/prowlarr` (media-server)

- **Service:** prowlarr
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`

### `lscr.io/linuxserver/bazarr` (media-server)

- **Service:** bazarr
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`
- **Recommended:** `v0.9.0.6-ls95` (age unknown)
- **Available versions:** `v0.9.0.5-ls95`, `v0.9.0.6-ls95`

### `lscr.io/linuxserver/qbittorrent` (media-server)

- **Service:** qbittorrent
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`

### `lscr.io/linuxserver/beets` (media-server)

- **Service:** beets
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`
- **Recommended:** `1.4.9-ls76` (age unknown)
- **Available versions:** `1.4.9-ls75`, `1.4.9-ls76`, `1.4.9-ls77`, `1.4.9-ls78`, `1.4.9-ls79`, `1.4.9-ls80`

### `docker.io/deluan/navidrome` (media-server)

- **Service:** navidrome
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`
- **Recommended:** `0.61.2` (released 2 weeks ago)
- **Available versions:** `0.61.2`, `0.61.1`, `0.61.0`, `0.60.3`, `0.60.2`, `0.60.0`

### `docker.io/jellyfin/jellyfin` (media-server)

- **Service:** jellyfin
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`
- **Recommended:** `10.11.8` (released 3 weeks ago)
- **Available versions:** `10.11.8`, `10.11.7`, `10.11.6`, `10.11.5`, `10.11.4`, `10.11.3`, `10.11.2`, `10.11`

### `lscr.io/linuxserver/sonarr` (media-server)

- **Service:** sonarr
- **File:** `/home/jon/docker-projects/media-server/docker-compose.yml`
- **Recommended:** `3.0.4.999-ls73` (age unknown)
- **Available versions:** `3.0.4.1002-ls75`, `3.0.4.999-ls73`, `3.0.4.993-ls70`, `3.0.4.1009-ls1`, `3.0.4.995-ls72`, `3.0.4.994-ls71`, `3.0.4.1006-ls77`, `3.0.4.1003-ls76`, `3.0.4.1000-ls74`, `2.0.0.5344-ls1`

### `docker.io/grafana/grafana` (monitoring-hub)

- **Service:** grafana
- **File:** `/home/jon/docker-projects/monitoring-hub/docker-compose.yml`
- **Recommended:** `13.0.1` (released 1 week ago)
- **Available versions:** `13.0.1`, `13.0.1-ubuntu`, `13.0`, `13.0-ubuntu`, `12.4.3`, `12.4.3-ubuntu`, `12.4.2`, `12.4.2-ubuntu`, `12.4.1`, `12.4.1-ubuntu`

### `docker.io/prom/prometheus` (monitoring-hub)

- **Service:** prometheus
- **File:** `/home/jon/docker-projects/monitoring-hub/docker-compose.yml`
- **Recommended:** `v3.11.2` (released 2 weeks ago)
- **Available versions:** `v3.11.2`, `v3.11.2-distroless`, `v3.11.2-busybox`, `v3.11.1`, `v3.11.1-distroless`, `v3.11.1-busybox`, `v3.11.0`, `v3.11.0-rc.0`, `v3.11.0-distroless`, `v3.11.0-busybox`

### `ghcr.io/paperless-ngx/paperless-ngx` (paperless)

- **Service:** paperless
- **File:** `/home/jon/docker-projects/paperless/docker-compose.yml`
- **Recommended:** `2.9.0` (age unknown)
- **Available versions:** `2.9`, `2.9.0`, `2.8.6`, `2.8.5`, `2.8.4`, `2.8.3`, `2.8.2`, `2.8.1`, `2.8.0`, `2.8`

### `docker.io/directus/directus` (music-assets-directus)

- **Service:** directus
- **File:** `/home/jon/docker-projects/music-assets-directus/docker-compose.yml`
- **Recommended:** `11.17.3` (released 1 week ago)
- **Available versions:** `11.17.3`, `11.17.2`, `11.17.1`, `11.17.0`, `11.17`, `11.16.1`, `11.16`, `11.16.0`, `11.15.4`, `11.15.3`

### `docker.io/directus/directus` (salsablanca-directus)

- **Service:** directus
- **File:** `/home/jon/docker-projects/salsablanca-directus/docker-compose.yml`
- **Recommended:** `11.17.3` (released 1 week ago)
- **Available versions:** `11.17.3`, `11.17.2`, `11.17.1`, `11.17.0`, `11.17`, `11.16.1`, `11.16`, `11.16.0`, `11.15.4`, `11.15.3`

### `docker.io/stirlingtools/stirling-pdf` (stirling-pdf)

- **Service:** stirling-pdf
- **File:** `/home/jon/docker-projects/stirling-pdf/docker-compose.yml`
- **Recommended:** `2.10.0` (released 3 days ago)
- **Available versions:** `2.10.0`, `2.10.0-fat`, `2.9.2`, `2.9.2-fat`, `2.9.1`, `2.9.1-fat`, `2.9.0`, `2.9.0-fat`, `2.8.0`, `2.8.0-fat`

## Security

### :red_circle: High Severity

| Issue | Location | Suggestion |
| --- | --- | --- |
| Service 'directus' has hardcoded secret value | `/home/jon/docker-projects/jongriffinmusic-directus/docker-compose.yml -> SECRET` | Use ${SECRET} and add to .env file |
| Service 'directus' has hardcoded password value | `/home/jon/docker-projects/jongriffinmusic-directus/docker-compose.yml -> ADMIN_PASSWORD` | Use ${ADMIN_PASSWORD} and add to .env file |
| Service 'directus' has hardcoded secret value | `/home/jon/docker-projects/salsablanca/docker-compose.yml -> SECRET` | Use ${SECRET} and add to .env file |
| Service 'directus' has hardcoded secret value | `/home/jon/docker-projects/safira-office-directus/docker-compose.yml -> SECRET` | Use ${SECRET} and add to .env file |
| Service 'directus' has hardcoded password value | `/home/jon/docker-projects/safira-office-directus/docker-compose.yml -> ADMIN_PASSWORD` | Use ${ADMIN_PASSWORD} and add to .env file |
| Service 'vikunja' has hardcoded password value | `/home/jon/docker-projects/vikunja/docker-compose.yml -> VIKUNJA_MAILER_PASSWORD` | Use ${VIKUNJA_MAILER_PASSWORD} and add to .env file |
| Service 'directus' has hardcoded secret value | `/home/jon/docker-projects/music-assets-directus/docker-compose.yml -> SECRET` | Use ${SECRET} and add to .env file |
| Service 'directus' has hardcoded secret value | `/home/jon/docker-projects/salsablanca-directus/docker-compose.yml -> SECRET` | Use ${SECRET} and add to .env file |

