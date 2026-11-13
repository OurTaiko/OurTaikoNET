#!/bin/sh
# Fills the three local source databases from last night's production backups, for tests and
# development. Needs AWS credentials that can read the private bucket (aws login).
#
#   db/dev/pull-backups.sh [fanmade db] [official db] [profile db]
#   defaults: otn_src_fanmade otn_src_database otn_src_profile
#
# Each named local database is DROPPED and restored from the newest backup of its origin.
# The dumps hold real players' data: they are kept in .data/backups (ignored by git), and
# neither they nor databases restored from them may leave this machine.
#
#   BACKUP_BUCKET (default ourtaiko-private), AWS_REGION (default ap-northeast-1)
#   PGHOST (default /tmp), PGPORT (5432)
set -eu
here="$(cd "$(dirname "$0")/../.." && pwd)"
bucket="${BACKUP_BUCKET:-ourtaiko-private}"
export AWS_REGION="${AWS_REGION:-ap-northeast-1}"
export PGHOST="${PGHOST:-/tmp}" PGPORT="${PGPORT:-5432}"
store="$here/.data/backups"
mkdir -p "$store"
chmod 700 "$store"

pull() {
  prefix="$1" origin="$2" local_db="$3"
  base="s3://$bucket/$prefix/backups/postgresql/$origin"
  # Backup folders are named by UTC time, so the last one listed is the newest. One without
  # a manifest was not finished.
  for id in $(aws s3 ls "$base/" | awk '$1 == "PRE" { sub("/", "", $2); print $2 }' | sort -r); do
    if aws s3 ls "$base/$id/manifest.json" >/dev/null 2>&1; then
      newest="$id"
      break
    fi
  done
  : "${newest:?no finished backup under $base}"
  dir="$store/$origin/$newest"
  mkdir -p "$dir"
  [ -f "$dir/database.dump" ] || aws s3 cp --only-show-errors "$base/$newest/database.dump" "$dir/database.dump"
  aws s3 cp --only-show-errors "$base/$newest/manifest.json" "$dir/manifest.json"
  want="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["sha256"])' "$dir/manifest.json")"
  have="$(shasum -a 256 "$dir/database.dump" | cut -d' ' -f1)"
  [ "$want" = "$have" ] || { echo "pull-backups: $dir/database.dump does not match its manifest" >&2; exit 1; }
  psql -X -q -v ON_ERROR_STOP=1 postgres:///postgres -c "DROP DATABASE IF EXISTS $local_db WITH (FORCE)" -c "CREATE DATABASE $local_db"
  pg_restore --no-owner --no-privileges --exit-on-error -d "postgres:///$local_db" "$dir/database.dump"
  echo "$local_db <- $origin $newest"
  unset newest
}

pull fanmade ourtaiko_fanmade "${1:-otn_src_fanmade}"
pull database ourtaiko_database "${2:-otn_src_database}"
pull profile ourtaiko_profile "${3:-otn_src_profile}"
echo "now: db/dev/test.sh ${1:-otn_src_fanmade} ${2:-otn_src_database} ${3:-otn_src_profile}"
