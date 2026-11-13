#!/bin/sh
# The whole database path on local copies: build the schema, import, import again, check
# behaviour, and prove baseline.sql rebuilds the same schema.
#
#   db/dev/test.sh <fanmade db> <official db> <profile db>
#
# The three names are local databases in the old layout (see db/README.md for making them).
# They are only used as templates: the test works on copies it seeds and removes afterwards.
# Server: PGHOST (default /tmp), PGPORT (5432). Rewrites db/baseline.sql from the sources' schemas.
set -eu
here="$(cd "$(dirname "$0")/.." && pwd)"
export PGHOST="${PGHOST:-/tmp}" PGPORT="${PGPORT:-5432}"
fanmade="${1:?fanmade database name}"
official="${2:?official database name}"
profile="${3:?profile database name}"
url() { echo "postgres:///$1"; }
admin() { psql -X -q -v ON_ERROR_STOP=1 "$(url postgres)" "$@"; }
# Runs a command, shows its output through a filter, and stops the test if the command failed
# (a plain pipe would report the filter's success instead).
log="$(mktemp)"
shown() {
  filter="$1"
  shift
  if "$@" >"$log" 2>&1; then
    sh -c "$filter" <"$log" || true
  else
    cat "$log" >&2
    echo "FAILED: $*" >&2
    exit 1
  fi
}
quiet="grep -v -e 'does not exist, skipping' -e 'drop cascades' -e '^DETAIL' -e '^drop ' -e '^\$'"
notices="sed -n 's/^.*NOTICE:  //p'"

cleanup() {
  rm -f "$log" "$log.schema"
  for d in otn_test_fanmade otn_test_official otn_test_profile otn_test_baseline otn_test_net; do
    admin -c "DROP DATABASE IF EXISTS $d" 2>/dev/null || true
  done
}
trap cleanup EXIT HUP INT TERM
cleanup

echo "== copies of the sources, with test rows"
admin -c "CREATE DATABASE otn_test_fanmade TEMPLATE $fanmade" \
      -c "CREATE DATABASE otn_test_official TEMPLATE $official" \
      -c "CREATE DATABASE otn_test_profile TEMPLATE $profile"
psql -X -q -v ON_ERROR_STOP=1 -v tag=f "$(url otn_test_fanmade)" -f "$here/dev/seed-source.sql"
psql -X -q -v ON_ERROR_STOP=1 -v tag=d "$(url otn_test_official)" -f "$here/dev/seed-source.sql"
psql -X -q -v ON_ERROR_STOP=1 "$(url otn_test_profile)" -f "$here/dev/seed-profile.sql"

echo "== build"
admin -c "CREATE DATABASE otn_test_net"
shown "$quiet" "$here/build.sh" "$(url otn_test_net)" "$(url otn_test_fanmade)" "$(url otn_test_official)"

echo "== import"
export SRC_HOST="$PGHOST" SRC_PORT="$PGPORT"
shown "$quiet" "$here/import.sh" "$(url otn_test_net)" otn_test_fanmade otn_test_official otn_test_profile

echo "== import again (must give the same result, and keep players' groups)"
psql -X -q -v ON_ERROR_STOP=1 "$(url otn_test_net)" -c "UPDATE net.users SET groups = 5 WHERE id = repeat('a', 32)"
shown true "$here/import.sh" "$(url otn_test_net)" otn_test_fanmade otn_test_official otn_test_profile
shown "$notices" psql -X -q -v ON_ERROR_STOP=1 "$(url otn_test_net)" <<EOF
DO \$\$
BEGIN
  IF (SELECT count(*) FROM net.import_runs) <> 2 THEN RAISE EXCEPTION 'expected two import runs'; END IF;
  IF (SELECT groups FROM net.users WHERE id = repeat('a', 32)) <> 5 THEN RAISE EXCEPTION 'the second import lost a player''s groups'; END IF;
  IF (SELECT report FROM net.import_runs ORDER BY finished_at LIMIT 1) <> (SELECT report FROM net.import_runs ORDER BY finished_at DESC LIMIT 1) THEN
    RAISE EXCEPTION 'the second import gave a different report';
  END IF;
  RAISE NOTICE 'ok: second import matches the first, groups kept';
END \$\$;
EOF

echo "== behaviour"
shown "$notices" psql -X -q -v ON_ERROR_STOP=1 "$(url otn_test_net)" -f "$here/test/behaviour.sql"

echo "== baseline.sql rebuilds the same schema"
admin -c "CREATE DATABASE otn_test_baseline"
psql -X -q -v ON_ERROR_STOP=1 "$(url otn_test_baseline)" -c "DROP SCHEMA public" -f "$here/baseline.sql" -c "CREATE SCHEMA public" >/dev/null
shown "$notices" psql -X -q -v ON_ERROR_STOP=1 "$(url otn_test_baseline)" -f "$here/check.sql"
dump() { pg_dump --schema-only --no-owner --no-privileges -n fanmade -n official -n net "$(url "$1")" | grep -v -e '^\\restrict ' -e '^\\unrestrict '; }
# The imported database also holds net.import_runs, which the importer adds; leave it out.
if dump otn_test_baseline | cmp -s - "$(sed '/^-- Rows of the baseline$/,$d' "$here/baseline.sql" > "$log.schema" && echo "$log.schema")"; then
  echo "ok: a database made from baseline.sql dumps back to baseline.sql"
else
  echo "FAILED: baseline.sql does not reproduce itself" >&2
  exit 1
fi
echo "== all passed"
