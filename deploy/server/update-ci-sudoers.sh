#!/usr/bin/env bash
# Installs what the deploy account (ourtaiko-ci) may run as root: exactly the docker commands
# deploy/publish.sh runs. Run it on the server, as an administrator, when that list changes;
# create-ci-user.sh runs it for a new account.
set -euo pipefail
user=ourtaiko-ci
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cat > "$work/sudoers" <<'SUDO'
# The deploy workflows of OurTaikoNet (deploy/publish.sh) run exactly these as root.
Cmnd_Alias OURTAIKO_NET_PUBLISH = \
    /usr/bin/docker load, \
    /usr/bin/docker image inspect ourtaiko-net\:[0-9a-f]* --format *, \
    /usr/bin/docker tag ourtaiko-net\:[0-9a-f]* ourtaiko-net\:release, \
    /usr/bin/docker compose -f /opt/ourtaiko-net/compose.production.yml up -d --wait --no-deps net, \
    /usr/bin/docker image ls ourtaiko-net --format *, \
    /usr/bin/docker image rm ourtaiko-net\:*, \
    /usr/bin/docker container inspect ourtaiko-net-rooms --format *, \
    /usr/bin/docker exec ourtaiko-net-rooms ourtaiko-rooms drain -wait *, \
    /usr/bin/docker tag ourtaiko-net\:[0-9a-f]* ourtaiko-net-rooms\:[0-9a-f]*, \
    /usr/bin/docker tag ourtaiko-net\:[0-9a-f]* ourtaiko-net-rooms\:release, \
    /usr/bin/docker compose -f /opt/ourtaiko-net/compose.production.yml up -d --wait --no-deps rooms, \
    /usr/bin/docker image ls ourtaiko-net-rooms --format *, \
    /usr/bin/docker image rm ourtaiko-net-rooms\:*
ourtaiko-ci ALL=(root) NOPASSWD: OURTAIKO_NET_PUBLISH
SUDO
sudo -n visudo -cf "$work/sudoers" >/dev/null || { echo "the sudoers file does not parse; not installed"; exit 1; }
previous="$work/previous"
sudo -n test -e "/etc/sudoers.d/$user" && sudo -n cp "/etc/sudoers.d/$user" "$previous"
sudo -n install -m 0440 -o root -g root "$work/sudoers" "/etc/sudoers.d/$user"
if ! sudo -n visudo -c >/dev/null; then
  if [ -e "$previous" ]; then sudo -n install -m 0440 -o root -g root "$previous" "/etc/sudoers.d/$user"; else sudo -n rm -f "/etc/sudoers.d/$user"; fi
  echo "sudoers check failed; put back as it was"
  exit 1
fi
echo "installed /etc/sudoers.d/$user"
