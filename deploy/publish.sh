#!/usr/bin/env bash
# Runs on the server, sent by the deploy and rollback workflows: loads the image they
# uploaded and replaces the site's container with it, and the rooms' when they changed.
#
#   DEPLOY_REVISION       full commit SHA the image was built from
#   DEPLOY_IMAGE_SHA256   checksum of the uploaded archive
#   DEPLOY_ROOMS          true: replace the rooms' container as well, after the rounds under
#                         way have ended (at most DEPLOY_ROOMS_WAIT, default 20m)
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

rooms=${DEPLOY_ROOMS:-false}
compose=(sudo -n docker compose -f "$base/compose.production.yml")
# Found out before anything has changed (deploy/server/update-ci-sudoers.sh allows it).
if [ "$rooms" = true ] && ! sudo -n -l docker compose -f "$base/compose.production.yml" up -d --wait --no-deps rooms >/dev/null; then
  echo 'This server does not let the deploy account publish the rooms: run deploy/server/update-ci-sudoers.sh'
  exit 1
fi

# Lets the rounds under way end, then replaces the rooms' container.
publish_rooms() {
  if sudo -n docker container inspect ourtaiko-net-rooms --format '{{.Id}}' >/dev/null 2>&1; then
    sudo -n docker exec ourtaiko-net-rooms ourtaiko-rooms drain -wait "${DEPLOY_ROOMS_WAIT:-20m}" \
      || echo 'The rooms could not be asked to wait for their rounds; replacing them anyway'
  fi
  "${compose[@]}" up -d --wait --no-deps rooms
  sudo -n docker image ls ourtaiko-net-rooms --format '{{.Tag}}' | while read -r tag; do
    if [ "$tag" != release ] && [ "$tag" != "$revision" ] && [ "$tag" != '<none>' ]; then
      sudo -n docker image rm "ourtaiko-net-rooms:$tag" >/dev/null || true
    fi
  done
}

actual=$(sha256sum -- "$archive")
test "${actual%% *}" = "$DEPLOY_IMAGE_SHA256" || { echo 'Image archive checksum mismatch'; exit 1; }
sudo -n docker load < "$archive"
label=$(sudo -n docker image inspect "ourtaiko-net:$revision" --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}')
test "$label" = "$revision" || { echo 'Image revision mismatch'; exit 1; }
sudo -n docker tag "ourtaiko-net:$revision" ourtaiko-net:release
install -m 0644 "$incoming/compose.production.yml" "$base/compose.production.yml"

# The rooms go first: the site tells the game which protocol they speak, so it must not name
# one they do not know yet. The first time there are none, and the site gives up port 8097 to
# them.
rooms_after_site=false
if [ "$rooms" = true ]; then
  sudo -n docker tag "ourtaiko-net:$revision" "ourtaiko-net-rooms:$revision"
  sudo -n docker tag "ourtaiko-net:$revision" ourtaiko-net-rooms:release
  if sudo -n docker container inspect ourtaiko-net-rooms --format '{{.Id}}' >/dev/null 2>&1; then
    publish_rooms
  else
    rooms_after_site=true
  fi
fi
"${compose[@]}" up -d --wait --no-deps net
curl -fsS http://127.0.0.1:8096/readyz
if [ "$rooms_after_site" = true ]; then
  publish_rooms
fi
echo "Published OurTaikoNet $revision (rooms: $rooms)"

rm -f -- "$archive" "$incoming/compose.production.yml"
rmdir -- "$incoming"
# Every published image is kept in ourtaiko-private (net/images/<commit>.tar.gz) for
# rollback.yml, not here: drop every other local tag; Docker deletes an image with its last tag.
sudo -n docker image ls ourtaiko-net --format '{{.Tag}}' | while read -r tag; do
  if [ "$tag" != release ] && [ "$tag" != "$revision" ] && [ "$tag" != '<none>' ]; then
    sudo -n docker image rm "ourtaiko-net:$tag" >/dev/null || true
  fi
done
