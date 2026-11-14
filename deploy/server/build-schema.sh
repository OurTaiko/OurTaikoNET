#!/usr/bin/env bash
# Builds the schema in the empty database, as the application's role so that it owns it.
# Run it from a directory that also holds db/baseline.sql. (`ourtaiko-net -init` does the
# same from the image.)
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
app() { sudo -n docker exec ourtaiko-postgres psql -U ourtaiko_net -d ourtaiko_net -X -v ON_ERROR_STOP=1 "$@"; }
tables=$(app -At -c "select count(*) from pg_class c join pg_namespace n on n.oid=c.relnamespace where c.relkind in ('r','p') and n.nspname not like 'pg\_%' and n.nspname<>'information_schema'")
[ "$tables" = 0 ] || { echo "the database is not empty ($tables tables); nothing done"; exit 1; }
trap 'sudo -n docker exec ourtaiko-postgres rm -f /tmp/otn-baseline.sql' EXIT
sudo -n docker cp -q "$here/baseline.sql" ourtaiko-postgres:/tmp/otn-baseline.sql
app -q --single-transaction -f /tmp/otn-baseline.sql
echo "schema built:"
app -At -F ' | ' -c "select n.nspname, count(*) filter (where c.relkind in ('r','p')) as tables, count(distinct pg_get_userbyid(c.relowner)) as owners, min(pg_get_userbyid(c.relowner)) as owner from pg_class c join pg_namespace n on n.oid=c.relnamespace where n.nspname in ('fanmade','official','net') group by 1 order by 1"
