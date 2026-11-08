# Automated scanning with email

[`scripts/security-report/`](../scripts/security-report/) holds `security-report`, a daily security email for an Ubuntu server, with docker-scanner covering its Docker images. Its [README](../scripts/security-report/README.md) covers what the email contains, its settings and how to install it. `config.example` and `cve-watchlist.example` in the same folder are starting points for `/etc/security-report/config` and `cve-watchlist`.

Not on Ubuntu: run docker-scanner from cron with `-format html -output <file>` and mail the file.
