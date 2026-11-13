#!/bin/sh
# Builds the empty schema of ourtaiko_net in an existing, empty database from the schemas of
# the two song databases, then writes it to baseline.sql.
#
#   db/build.sh <target url> <fanmade url> <official url>
#
# The sources are only read (pg_dump --schema-only). The order matters: the first source's
# users and sign-in tables become net's.
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
target="${1:?target database url}"
fanmade="${2:?fanmade database url}"
official="${3:?official (OurTaiko Database) database url}"

run() { psql -X -q -v ON_ERROR_STOP=1 "$target" "$@"; }

if [ "$(run -Atc "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname NOT LIKE 'pg\_%' AND n.nspname <> 'information_schema'")" != 0 ]; then
  echo "build.sh: the target database is not empty" >&2
  exit 1
fi

run -f "$here/build/adopt.sql"
for pair in "fanmade|$fanmade" "official|$official"; do
  src="${pair%%|*}"
  url="${pair#*|}"
  # pg_dump 18 wraps its output in \restrict lines with a random key; they are not needed here.
  pg_dump --schema-only --no-owner --no-privileges -n public -n auth -n internal "$url" \
    | grep -v -e '^\\restrict ' -e '^\\unrestrict ' -e '^CREATE SCHEMA public;$' -e '^COMMENT ON SCHEMA public ' \
    | run -f - >/dev/null
  run -c "CALL net_build.adopt('$src')"
done
run -f "$here/build/alike.sql"
run -f "$here/build/net.sql" >/dev/null
run -c "DROP SCHEMA net_build CASCADE" -c "DROP SCHEMA public CASCADE" -c "CREATE SCHEMA public"
run -f "$here/check.sql"

pg_dump --schema-only --no-owner --no-privileges -n fanmade -n official -n net "$target" \
  | grep -v -e '^\\restrict ' -e '^\\unrestrict ' > "$here/baseline.sql"
# A schema dump carries no rows; the one row that says which schema this is goes after it.
cat >> "$here/baseline.sql" <<'SQL'
-- Rows of the baseline
INSERT INTO net.schema_migrations (version) VALUES (1);
SQL
echo "wrote $here/baseline.sql ($(wc -l < "$here/baseline.sql" | tr -d ' ') lines)"
