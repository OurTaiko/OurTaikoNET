#!/bin/sh
# Installs the daily backup of ourtaiko_net, as root on the server:  sh install-backup.sh
#
# The program is cmd/backup of this repository, built for the server and put beside this
# script as `ourtaiko-net-backup`:
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ourtaiko-net-backup ./cmd/backup
# It dumps the database, restores the dump into a scratch database to prove it, and uploads it
# to the private bucket with the key in /etc/ourtaiko-private/aws.env.
set -eu
umask 077
here=$(cd "$(dirname "$0")" && pwd)
program=/usr/local/libexec/ourtaiko-net-backup
[ -f "$here/ourtaiko-net-backup" ] && install -m 0755 -o root -g root "$here/ourtaiko-net-backup" "$program"
[ -x "$program" ] || { echo "$program is not installed" >&2; exit 1; }
[ -f /etc/ourtaiko-private/aws.env ] || { echo "/etc/ourtaiko-private/aws.env is missing" >&2; exit 1; }

install -d -m 0700 -o root -g root /etc/ourtaiko-net
install -d -m 0755 -o root -g root /var/backups/ourtaiko-net
install -d -m 0700 -o root -g root /var/backups/ourtaiko-net/daily
cat > /etc/ourtaiko-net/backup.env <<'ENV'
# Root-only; AWS credentials are read from /etc/ourtaiko-private/aws.env.
BACKUP_DATABASE=ourtaiko_net
BACKUP_POSTGRES_CONTAINER=ourtaiko-postgres
BACKUP_POSTGRES_USER=postgres
BACKUP_DIRECTORY=/var/backups/ourtaiko-net/daily
BACKUP_RETENTION_DAYS=30
BACKUP_S3_ENABLED=true
BACKUP_S3_PREFIX=net/backups/postgresql
AWS_REGION=ap-northeast-1
S3_BUCKET=ourtaiko-private
ENV
chmod 600 /etc/ourtaiko-net/backup.env

cat > /etc/systemd/system/ourtaiko-net-backup.service <<'UNIT'
[Unit]
Description=Validated OurTaikoNet PostgreSQL backup to private S3
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target
StartLimitIntervalSec=3h
StartLimitBurst=3

[Service]
Type=oneshot
User=root
Group=root
UMask=0077
EnvironmentFile=/etc/ourtaiko-net/backup.env
EnvironmentFile=/etc/ourtaiko-private/aws.env
ExecStart=/usr/local/libexec/ourtaiko-net-backup
TimeoutStartSec=2h
Restart=on-failure
RestartSec=15min
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/backups/ourtaiko-net
UNIT

cat > /etc/systemd/system/ourtaiko-net-backup.timer <<'UNIT'
[Unit]
Description=Daily OurTaikoNet database backup at 04:30 Asia/Tokyo

[Timer]
OnCalendar=*-*-* 04:30:00 Asia/Tokyo
Persistent=true
AccuracySec=1min
Unit=ourtaiko-net-backup.service

[Install]
WantedBy=timers.target
UNIT

systemctl daemon-reload
systemctl enable --now ourtaiko-net-backup.timer
echo "installed; the first backup runs now"
systemctl start ourtaiko-net-backup.service && echo "backup finished" || { echo "the backup failed:" >&2; journalctl -u ourtaiko-net-backup.service -n 15 --no-pager >&2; exit 1; }
journalctl -u ourtaiko-net-backup.service -n 8 --no-pager | cut -c1-220
ls -la /var/backups/ourtaiko-net/daily | tail -3 | awk '{print $5, $6, $7, $8, $9}'
systemctl list-timers ourtaiko-net-backup.timer --no-pager | head -3
