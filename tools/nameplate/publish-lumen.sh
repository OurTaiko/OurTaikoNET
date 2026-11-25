#!/bin/sh
# Uploads the layered nameplate set (lumen.py) to the public bucket under s3://ourtaiko-public/
# nameplate/. The site and the game (which does not carry the set) both read
# https://distrib.ourtaiko.org/nameplate/lumen.json and download the files it names. The .otanim
# files go first and lumen.json last, so it never names a file that is not there yet. Every
# file's name carries its hash, so nothing published is replaced (the mirror keeps the first copy
# it reads) and the files may be cached for good; lumen.json for five minutes, and the upload ends
# by invalidating it on CloudFront. catalog.json and its PNGs, which released game versions read,
# are not touched.
# Needs `aws login` with write access to the bucket and to that distribution.
#
# Usage: tools/nameplate/publish-lumen.sh <OurTaikoPlay>/Tools/nameplate-samples/lumen/set
set -eu
src=${1:?usage: publish-lumen.sh <OurTaikoPlay>/Tools/nameplate-samples/lumen/set}
here=$(cd "$(dirname "$0")" && pwd)
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
python3 -I "$here/lumen.py" "$src" "$dir"
dest=s3://ourtaiko-public/nameplate
aws s3 sync "$dir/lumen" "$dest/lumen" --size-only --exclude '.*' \
  --content-type application/octet-stream --cache-control 'public, max-age=31536000, immutable'
aws s3 cp "$dir/lumen.json" "$dest/lumen.json" \
  --content-type application/json --cache-control 'public, max-age=300'
# distrib.ourtaiko.org
aws cloudfront create-invalidation --distribution-id E29SNWDH0GYCHC --paths '/nameplate/lumen.json' \
  --query 'Invalidation.{Id:Id,Status:Status}' --output text
echo "published: https://distrib.ourtaiko.org/nameplate/lumen.json"
