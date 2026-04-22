#!/bin/bash
# Docker Scanner — weekly scan with HTML email report
# Add to crontab: 0 8 * * 1 /home/jon/docker-scanner/docker-scan-email.sh

set -a
source /home/jon/docker-scanner/.env
set +a

REPORT="/tmp/docker-scan-report.html"
SUBJECT="Docker Security Report - $(date +%Y-%m-%d)"

# Run the scan
${SCANNER} -dir "${SCAN_DIR}" -format "${FORMAT}" -output "${REPORT}" -safe-days "${SAFE_DAYS}"

# Send HTML email via mail
mail -a "Content-Type: text/html; charset=utf-8" \
     -s "${SUBJECT}" \
     "${MAIL_TO}" < "${REPORT}"

# Cleanup
rm -f "${REPORT}"