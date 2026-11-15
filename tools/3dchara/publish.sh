#!/bin/sh
# Uploads a 3dchara folder to the public bucket (s3://ourtaiko-public/3dchara/, read at
# https://distrib.ourtaiko.org/3dchara/): the files first, the catalog last, so the catalog never
# names a file that is not there yet. Files may be cached for a day, the catalog for five minutes;
# CloudFront (distrib.ourtaiko.org) keys its cache on the path alone (not "?v="), so the upload
# ends by invalidating /3dchara/* there: a replaced file reaches the game and the website at once
# (the game checks every file's hash and would refuse an old copy). Needs `aws login` with write
# access to the bucket and to that distribution.
#
# Usage: tools/3dchara/publish.sh <folder>
set -eu
dir=${1:?usage: publish.sh <folder>}
here=$(cd "$(dirname "$0")" && pwd)
python3 -I "$here/catalog.py" "$dir"
dest=s3://ourtaiko-public/3dchara
aws s3 sync "$dir" "$dest" --exclude catalog.json --exclude '.*' --exclude '*/.*' \
  --cache-control 'public, max-age=86400'
aws s3 cp "$dir/catalog.json" "$dest/catalog.json" \
  --content-type application/json --cache-control 'public, max-age=300'
# distrib.ourtaiko.org
aws cloudfront create-invalidation --distribution-id E29SNWDH0GYCHC --paths '/3dchara/*' \
  --query 'Invalidation.{Id:Id,Status:Status}' --output text
echo "published: https://distrib.ourtaiko.org/3dchara/catalog.json"
