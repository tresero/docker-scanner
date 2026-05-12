# Automated scanning with email

docker-scanner is designed to run periodically and email results. The HTML format produces clean emails that work in any mail client.

## Configuration

Create a `.env` file in the docker-scanner directory:

```
SCANNER=/home/you/docker-scanner/docker-scanner
SCAN_DIR=/home/you/docker-projects
MAIL_TO=you@example.com
SAFE_DAYS=3
FORMAT=html
```

## Wrapper script

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

## Crontab

```bash
chmod +x docker-scan-email.sh
crontab -e

# Weekly on Monday at 8am
0 8 * * 1 /home/you/docker-scanner/docker-scan-email.sh

# Or daily
0 8 * * * /home/you/docker-scanner/docker-scan-email.sh
```

## Requirements

- `mailutils` package (Ubuntu/Debian) or equivalent
- A working MTA on the host: `postfix`, `msmtp`, `ssmtp`, etc.

If you don't have an MTA configured, `msmtp` is the simplest option for sending through an existing SMTP account (Gmail, Fastmail, etc.). See your distro's docs for setup.

## Tips

- **GitHub rate limit:** if you have many images covered by overrides, set `GITHUB_TOKEN` in the `.env` file so the wrapper script exports it for the scanner.
- **Suppression:** there's no "only email if issues found" flag yet — every run produces a report and sends mail. Track the [feature request](https://github.com/tresero/docker-scanner/issues) for that.
- **Multiple environments:** run separate cron jobs with different `.env` files if you scan multiple hosts.