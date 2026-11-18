#!/usr/bin/env bash
set -euo pipefail
[[ "$EUID" == 0 ]] || { echo 'Install as root on the OSS mirror host.' >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
git diff --quiet -- tools/sync-distribution-mirror.py tools/distribution_mirror deploy/aliyun-mirror.json deploy/ourtaiko-mirror-sync.service deploy/ourtaiko-mirror-sync.timer
git diff --cached --quiet
/usr/bin/python3 -c 'import oss2, requests'
revision="$(git rev-parse HEAD)"
release="/opt/ourtaiko-mirror/releases/$revision"
if [[ ! -d "$release" ]]; then
  install -d -m 0755 "$release"
  git archive HEAD tools/sync-distribution-mirror.py tools/distribution_mirror deploy/aliyun-mirror.json | tar -xf - -C "$release"
fi
ln -s "$release" /opt/ourtaiko-mirror/current.next
mv -Tf /opt/ourtaiko-mirror/current.next /opt/ourtaiko-mirror/current
install -d -m 0700 /var/cache/ourtaiko-mirror /var/lib/ourtaiko-mirror
touch /run/lock/taiko-memory-qa.lock
install -m 0644 deploy/ourtaiko-mirror-sync.service deploy/ourtaiko-mirror-sync.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now ourtaiko-mirror-sync.timer
echo "Mirror synchronization installed from $revision"
