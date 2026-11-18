#!/usr/bin/env bash
# Creates the account the deploy workflows sign in as: no password, one key, and sudo for
# exactly the docker commands the publish script runs.
# Run it from a directory that also holds ci.pub, the public half of the key whose private
# half is the repositories' DEPLOY_KEY secret, and update-ci-sudoers.sh.
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

"$here/update-ci-sudoers.sh"

echo "created:"
getent passwd "$user" | cut -d: -f1,3,6,7
sudo -n passwd --status "$user" | awk '{print "password status:", $2}'
id "$user"
sudo -n ls -la "/home/$user/.ssh" | awk 'NR>1{print $1, $3, $9}'
ls -ld "$base" "$base"/* | awk '{print $1, $3, $4, $9}'
sudo -n -l -U "$user" | sed -n '/may run/,$p' | cut -c1-150
