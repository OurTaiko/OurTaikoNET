#!/usr/bin/env bash
# Creates the account the deploy workflows sign in as: no password, one key, and sudo for
# exactly the docker commands the publish script runs.
# Run it from a directory that also holds ci.pub, the public half of the key whose private
# half is the repositories' DEPLOY_KEY secret.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
user=ourtaiko-ci
base=/opt/ourtaiko-net
getent passwd "$user" >/dev/null && { echo "$user already exists; nothing done"; exit 1; }
sudo -n test -e /etc/sudoers.d/$user && { echo "sudoers file already exists; nothing done"; exit 1; }
grep -Eq '^ssh-ed25519 [A-Za-z0-9+/=]+ ' "$here/ci.pub" || { echo "ci.pub is not an ed25519 public key"; exit 1; }

sudo -n useradd --create-home --shell /bin/bash --comment 'OurTaiko CI deploy' "$user"
# No password at all: the account is reached by its key only.
sudo -n passwd --lock "$user" >/dev/null
sudo -n install -d -m 0700 -o "$user" -g "$user" "/home/$user/.ssh"
# The key signs in and runs commands; it forwards nothing.
printf 'no-port-forwarding,no-agent-forwarding,no-X11-forwarding %s\n' "$(cat "$here/ci.pub")" > "$here/authorized_keys"
sudo -n install -m 0600 -o "$user" -g "$user" "$here/authorized_keys" "/home/$user/.ssh/authorized_keys"

# What it publishes into is its own.
sudo -n chown -R "$user:$user" "$base"

cat > "$here/sudoers" <<'SUDO'
# The deploy workflows of OurTaikoNet (deploy/publish.sh) run exactly these as root.
Cmnd_Alias OURTAIKO_NET_PUBLISH = \
    /usr/bin/docker load, \
    /usr/bin/docker image inspect ourtaiko-net\:[0-9a-f]* --format *, \
    /usr/bin/docker tag ourtaiko-net\:[0-9a-f]* ourtaiko-net\:release, \
    /usr/bin/docker compose -f /opt/ourtaiko-net/compose.production.yml up -d --wait --no-deps net, \
    /usr/bin/docker image ls ourtaiko-net --format *, \
    /usr/bin/docker image rm ourtaiko-net\:*
ourtaiko-ci ALL=(root) NOPASSWD: OURTAIKO_NET_PUBLISH
SUDO
sudo -n visudo -cf "$here/sudoers" >/dev/null || { echo "the sudoers file does not parse; not installed"; exit 1; }
sudo -n install -m 0440 -o root -g root "$here/sudoers" /etc/sudoers.d/$user
sudo -n visudo -c >/dev/null || { sudo -n rm -f /etc/sudoers.d/$user; echo "sudoers check failed; removed again"; exit 1; }

echo "created:"
getent passwd "$user" | cut -d: -f1,3,6,7
sudo -n passwd --status "$user" | awk '{print "password status:", $2}'
id "$user"
sudo -n ls -la "/home/$user/.ssh" | awk 'NR>1{print $1, $3, $9}'
ls -ld "$base" "$base"/* | awk '{print $1, $3, $4, $9}'
sudo -n -l -U "$user" | sed -n '/may run/,$p' | cut -c1-150
