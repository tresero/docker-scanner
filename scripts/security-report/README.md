# security-report

A daily security email for one Ubuntu server. It starts with a **WHAT TO DO** list, with commands, and then covers:

- **Updates:** pending apt updates, security ones first. It also checks that this host's `noble-security` and `noble-updates` lists aren't behind `archive.ubuntu.com`, because a stalled mirror still looks like a clean `apt update`.
- **Security notices:** new Ubuntu Security Notices (USNs) for installed packages, and older ones that are still not installed.
- **CVE watchlist:** each CVE in `cve-watchlist`, checked against Ubuntu's CVE feed and the installed version. FIX INSTALLED means Ubuntu names a fixed version (status `pending`, no USN yet) and that version is installed.
- **Kernel:** the running kernel compared with the newest installed one (is a reboot needed?), and Livepatch status.
- **Docker images:** a table from docker-scanner with Status, App, Container (running, exited with its code, not created), Image, Running (the version), Set tag to and Notes. Images with nothing to report are listed under the table, and it says when they are all running. Each security issue is listed once: on its service, or on a row of its own for an issue about the whole compose file, such as a missing `.env`.

The subject line starts with ATTENTION, WORTH A LOOK or ALL CLEAR. The email has a plain-text part and an HTML part.

It isn't in cron. `security-report.timer` runs it every day at 08:00 America/Denver. With `Persistent=true`, a run missed while the server was off happens at the next boot.

## Files

| Path | What it is |
|---|---|
| `/usr/local/sbin/security-report` | The script (bash) |
| `/etc/security-report/config` | Settings (shell variables, see below) |
| `/etc/security-report/cve-watchlist` | One CVE ID per line. Text after the ID is a note shown in the report. |
| `/etc/systemd/system/security-report.service` | Oneshot unit that runs the script |
| `/etc/systemd/system/security-report.timer` | The daily schedule |
| `/var/lib/security-report/` | State: USNs already seen, still-open USNs, the last update list and the watchlist status. Changes are reported against this state. |

## Manual runs

```
security-report            # build the report, mail it, save state (what the timer does)
security-report --stdout   # print the text version; mails nothing, saves no state
security-report --html     # print the HTML version; mails nothing, saves no state
systemctl start security-report.service   # a real run, the same as the timer's
```

Run it as root. Every mode runs `apt-get update` first.

## Settings (`config`)

| Variable | Default | Meaning |
|---|---|---|
| `MAILTO` | `root` | Recipient |
| `MAILFROM` | `root` | Sender. |
| `WATCHLIST` | `/etc/security-report/cve-watchlist` | CVE watchlist file |
| `STATE` | `/var/lib/security-report` | State directory |
| `DOCKER_SCANNER_ENV` | `/root/docker-scanner/.env` | docker-scanner's settings file. The script reads `SCANNER` (path to the binary), `SCAN_DIR` (folder with the compose apps) and `SAFE_DAYS` (default 3). Set it to empty (`DOCKER_SCANNER_ENV=`) to leave out the Docker section. |
| `MAX_LIST_LAG_HOURS` | `24` | Warn when the local package lists are this many hours behind Ubuntu's |
| `UBUNTU_ARCHIVE` | `http://archive.ubuntu.com/ubuntu` | Where to compare the package lists against |

## Install on a new machine

It needs Ubuntu: it reads the release codename from `/etc/os-release` and uses Ubuntu's security API.

1. **Install the dependencies.** You need `jq`, `curl`, and a mail server at `/usr/sbin/sendmail` that can deliver to `MAILTO`. On this server that's Postfix relaying to Amazon SES.
   ```
   apt install jq curl
   ```
   Livepatch is optional. Without it (no Ubuntu Pro), the report says Livepatch isn't running.
2. **Copy the files from this server** (paths in the table above): the script, `config`, `cve-watchlist` and both unit files. Don't copy `/var/lib/security-report/`; the first run creates it.
   ```
   install -m 755 security-report /usr/local/sbin/security-report
   install -d /etc/security-report
   install -m 644 config cve-watchlist /etc/security-report/
   install -m 644 security-report.service security-report.timer /etc/systemd/system/
   ```
3. **Edit `config`:** set `MAILTO` and `MAILFROM` for the new machine. Add `DOCKER_SCANNER_ENV=` if it has no docker-scanner. If it does, docker-scanner is installed separately (it's in `/root/docker-scanner/` here) and its `.env` must set `SCANNER` and `SCAN_DIR`.
4. **Check the timer's time zone.** `OnCalendar=*-*-* 08:00:00 America/Denver` is in `security-report.timer`; change it there if needed.
5. **Test without mailing:**
   ```
   security-report --stdout
   ```
   Read the whole output. Problems the script hit (a failed fetch, a failed docker-scanner run) are listed in the report.
6. **Turn it on and send one real email:**
   ```
   systemctl daemon-reload
   systemctl enable --now security-report.timer
   systemctl start security-report.service
   systemctl list-timers security-report.timer
   ```
   The first real run lists the last 30 days of security notices that still apply, then saves its state. From the next run on, it reports only what has changed and what is still open.

## Related

- docker-scanner also sends its own Docker report email, from root's crontab (`/root/docker-scanner/docker-scanner-report.sh`, 08:00). That is separate from this report, and both are kept on purpose.
- Server notes are in `/etc/AGENTS.md`, and the change history, with backup paths, is in `/etc/AGENTS-history.md`.
