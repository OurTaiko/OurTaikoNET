#!/usr/bin/env bash
# Runs on the server, sent by the deploy and rollback workflows: loads the image they
# uploaded and replaces the running container with it.
#
#   DEPLOY_REVISION       full commit SHA the image was built from
#   DEPLOY_IMAGE_SHA256   checksum of the uploaded archive
#   DEPLOY_BASE_PATH      default /opt/ourtaiko-net
#
# The archive and the Compose file are in $base/incoming/$revision/.
set -euo pipefail
base=${DEPLOY_BASE_PATH:-/opt/ourtaiko-net}
revision=${DEPLOY_REVISION:-}
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || { echo 'DEPLOY_REVISION must be a full Git commit SHA'; exit 1; }
[[ "${DEPLOY_IMAGE_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] || { echo 'DEPLOY_IMAGE_SHA256 must be a SHA-256'; exit 1; }
incoming="$base/incoming/$revision"
archive="$incoming/net-image.tar.gz"

# One publish at a time, automatic or by hand.
exec 9>"$base/publish.lock"
flock -n 9 || { echo 'Another OurTaikoNet deployment is running'; exit 1; }

actual=$(sha256sum -- "$archive")
test "${actual%% *}" = "$DEPLOY_IMAGE_SHA256" || { echo 'Image archive checksum mismatch'; exit 1; }
sudo -n docker load < "$archive"
label=$(sudo -n docker image inspect "ourtaiko-net:$revision" --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}')
test "$label" = "$revision" || { echo 'Image revision mismatch'; exit 1; }
sudo -n docker tag "ourtaiko-net:$revision" ourtaiko-net:release
install -m 0644 "$incoming/compose.production.yml" "$base/compose.production.yml"

sudo -n docker compose -f "$base/compose.production.yml" up -d --wait --no-deps net
curl -fsS http://127.0.0.1:8096/readyz
echo "Published OurTaikoNet $revision"

rm -f -- "$archive" "$incoming/compose.production.yml"
rmdir -- "$incoming"
# Every published image is kept in ourtaiko-private (net/images/<commit>.tar.gz) for
# rollback.yml, not here: drop every other local tag; Docker deletes an image with its last tag.
sudo -n docker image ls ourtaiko-net --format '{{.Tag}}' | while read -r tag; do
  if [ "$tag" != release ] && [ "$tag" != "$revision" ] && [ "$tag" != '<none>' ]; then
    sudo -n docker image rm "ourtaiko-net:$tag" >/dev/null || true
  fi
done
