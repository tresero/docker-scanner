# Automated scanning with email

[`scripts/security-report/`](../scripts/security-report/) holds `security-report`, a daily security email for one Ubuntu server. docker-scanner covers its Docker section. The email starts with a **WHAT TO DO** list, with commands, and then covers:

- **Updates:** pending apt updates, security ones first. It also checks that the host's `-security` and `-updates` package lists aren't behind `archive.ubuntu.com`, because a stalled mirror still looks like a clean `apt update`.
- **Security notices:** new Ubuntu Security Notices (USNs) for installed packages, and older ones that are still not installed.
- **CVE watchlist:** each CVE in `cve-watchlist`, checked against Ubuntu's CVE feed and the installed version. FIX INSTALLED means Ubuntu names a fixed version (status `pending`, no USN yet) and that version is installed.
- **Kernel:** the running kernel compared with the newest installed one (is a reboot needed?), and Livepatch status.
- **Docker images:** a table from docker-scanner's JSON report with Status, App, Image, Running, Set tag to and Notes. Each security issue is listed once: on its service, or on a row of its own for an issue about the whole compose file, such as a missing `.env`.

The subject line starts with ATTENTION, WORTH A LOOK or ALL CLEAR. The email has a plain-text part and an HTML part.

`security-report.timer` runs it every day at 08:00 America/Denver. With `Persistent=true`, a run missed while the server was off happens at the next boot.

## Files

| Repo file | Installed as | What it is |
|---|---|---|
| `security-report` | `/usr/local/sbin/security-report` | The script (bash) |
| `config.example` | `/etc/security-report/config` | Settings (shell variables, see below) |
| `cve-watchlist.example` | `/etc/security-report/cve-watchlist` | One CVE ID per line. Text after the ID is a note shown in the report. |
| `security-report.service` | `/etc/systemd/system/security-report.service` | Oneshot unit that runs the script |
| `security-report.timer` | `/etc/systemd/system/security-report.timer` | The daily schedule |
| | `/var/lib/security-report/` | State: USNs already seen, still-open USNs, the last update list and the watchlist status. Changes are reported against this state. The first run creates it. |

## Settings (`config`)

| Variable | Default | Meaning |
|---|---|---|
| `MAILTO` | `root` | Recipient |
| `MAILFROM` | `root` | Sender |
| `WATCHLIST` | `/etc/security-report/cve-watchlist` | CVE watchlist file |
| `STATE` | `/var/lib/security-report` | State directory |
| `DOCKER_SCANNER_ENV` | `/root/docker-scanner/.env` | docker-scanner's settings file. The script reads `SCANNER` (path to the binary), `SCAN_DIR` (folder with the compose apps) and `SAFE_DAYS` (default 3). Set it to empty (`DOCKER_SCANNER_ENV=`) to leave out the Docker section. |
| `MAX_LIST_LAG_HOURS` | `24` | Warn when the local package lists are this many hours behind Ubuntu's |
| `UBUNTU_ARCHIVE` | `http://archive.ubuntu.com/ubuntu` | Where to compare the package lists against |

docker-scanner's `.env`, for example:

```
SCANNER=/root/docker-scanner/docker-scanner-linux-amd64
SCAN_DIR=/srv/docker
SAFE_DAYS=3
```

## Install

It needs Ubuntu: it reads the release codename from `/etc/os-release` and uses Ubuntu's security API. Run it as root.

1. **Install the dependencies.** You need `jq`, `curl`, and a mail server at `/usr/sbin/sendmail` that can deliver to `MAILTO`, such as Postfix.
   ```
   apt install jq curl
   ```
   Livepatch is optional. Without it (no Ubuntu Pro), the report says Livepatch isn't running.
2. **Install docker-scanner** from the [releases](https://github.com/tresero/docker-scanner/releases), check it against `checksums.txt`, and write its `.env` (above).
3. **Install the files** from `scripts/security-report/`:
   ```
   install -m 755 security-report /usr/local/sbin/security-report
   install -d /etc/security-report
   install -m 644 config.example /etc/security-report/config
   install -m 644 cve-watchlist.example /etc/security-report/cve-watchlist
   install -m 644 security-report.service security-report.timer /etc/systemd/system/
   ```
4. **Edit `/etc/security-report/config`:** set `MAILTO` and `MAILFROM`. Add `DOCKER_SCANNER_ENV=` if the machine has no docker-scanner.
5. **Check the timer's time zone.** `OnCalendar=*-*-* 08:00:00 America/Denver` is in `security-report.timer`; change it there if needed.
6. **Test without mailing:**
   ```
   security-report --stdout   # the text version; mails nothing, saves no state
   security-report --html     # the HTML version; mails nothing, saves no state
   ```
   Every mode runs `apt-get update` first. Read the whole output: problems the script hit, such as a failed fetch or a failed docker-scanner run, are listed in the report.
7. **Turn it on and send one real email:**
   ```
   systemctl daemon-reload
   systemctl enable --now security-report.timer
   systemctl start security-report.service
   systemctl list-timers security-report.timer
   ```
   The first real run lists the last 30 days of security notices that still apply, then saves its state. From the next run on, it reports only what has changed and what is still open.

## Not on Ubuntu

Run docker-scanner from cron with `-format html -output <file>` and mail the file.
