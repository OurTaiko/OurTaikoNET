#!/usr/bin/env bash
# Runs on the server, sent by the deploy and rollback workflows: makes one build of the
# website the current one. The OurTaikoNet server serves $base/web/current (its WEB_DIR) and
# reads it on every request, so nothing is restarted.
#
#   DEPLOY_REVISION     full commit SHA of the build
#   DEPLOY_WEB_SHA256   checksum of the uploaded archive; empty to go back to a release
#                       that is still on the server (rollback)
#   DEPLOY_BASE_PATH    default /opt/ourtaiko-net
set -euo pipefail
base=${DEPLOY_BASE_PATH:-/opt/ourtaiko-net}
revision=${DEPLOY_REVISION:-}
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || { echo 'DEPLOY_REVISION must be a full Git commit SHA'; exit 1; }
web="$base/web"
release="$web/releases/$revision"
mkdir -p "$web/releases"

# One publish at a time, automatic or by hand.
exec 9>"$base/publish-web.lock"
flock -n 9 || { echo 'Another website deployment is running'; exit 1; }

if [ -n "${DEPLOY_WEB_SHA256:-}" ]; then
  [[ "$DEPLOY_WEB_SHA256" =~ ^[0-9a-f]{64}$ ]] || { echo 'DEPLOY_WEB_SHA256 must be a SHA-256'; exit 1; }
  incoming="$base/incoming-web/$revision"
  archive="$incoming/web.tar.gz"
  actual=$(sha256sum -- "$archive")
  test "${actual%% *}" = "$DEPLOY_WEB_SHA256" || { echo 'Website archive checksum mismatch'; exit 1; }
  rm -rf -- "$release.tmp"
  mkdir -- "$release.tmp"
  tar -xzf "$archive" -C "$release.tmp" --no-same-owner
  test -f "$release.tmp/index.html" || { echo 'The archive has no index.html'; exit 1; }
  mkdir -p -- "$release.tmp/assets"
  # A page opened before this release still asks for the scripts and styles it named, and
  # they are named by their content: keep those of the last two weeks beside the new ones.
  if [ -d "$web/current/assets" ]; then
    find -L "$web/current/assets" -maxdepth 1 -type f -mtime -14 | while read -r old; do
      [ -e "$release.tmp/assets/$(basename -- "$old")" ] || cp -p -- "$old" "$release.tmp/assets/"
    done
  fi
  # The server reads the files as another user.
  chmod -R a+rX -- "$release.tmp"
  rm -rf -- "$release"
  mv -- "$release.tmp" "$release"
  rm -f -- "$archive"
  rmdir -- "$incoming"
else
  test -f "$release/index.html" || { echo "No release $revision on the server; publish it again instead"; exit 1; }
fi

# A new link moved over the old one: readers see the old release or the new, never neither.
ln -sfn "releases/$revision" "$web/current.new"
mv -Tf -- "$web/current.new" "$web/current"
echo "Published the website $revision"

# Keep the five newest releases for rollback; the current one is always among them.
ls -1dt -- "$web"/releases/*/ | tail -n +6 | while read -r old; do
  [ "$(basename -- "$old")" = "$revision" ] || rm -rf -- "$old"
done
