#!/bin/sh
# Uploads the game's nameplate art to the public bucket (s3://ourtaiko-public/nameplate/, read at
# https://distrib.ourtaiko.org/nameplate/), as tools/3dchara/publish.sh does for the 3D Don: the
# PNGs unchanged first, the catalog last, so the catalog never names a file that is not there yet.
# Files may be cached for a day, the catalog for five minutes; the website asks for each file with
# "?v=<hash>" from the catalog, and the upload ends by invalidating /nameplate/* on CloudFront
# (distrib.ourtaiko.org keys its cache on the path alone). Needs `aws login` with write access to
# the bucket and to that distribution.
#
# The folder is the game's Assets/OurTaiko/Art/global/nameplate (its .meta files are left out).
#
# Usage: tools/nameplate/publish.sh <folder>
set -eu
src=${1:?usage: publish.sh <folder>}
here=$(cd "$(dirname "$0")" && pwd)
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
cp "$src"/*.png "$dir"/
python3 -I "$here/catalog.py" "$dir"
dest=s3://ourtaiko-public/nameplate
aws s3 sync "$dir" "$dest" --exclude catalog.json --exclude '.*' \
  --content-type image/png --cache-control 'public, max-age=86400'
aws s3 cp "$dir/catalog.json" "$dest/catalog.json" \
  --content-type application/json --cache-control 'public, max-age=300'
# distrib.ourtaiko.org
aws cloudfront create-invalidation --distribution-id E29SNWDH0GYCHC --paths '/nameplate/*' \
  --query 'Invalidation.{Id:Id,Status:Status}' --output text
echo "published: https://distrib.ourtaiko.org/nameplate/catalog.json"
