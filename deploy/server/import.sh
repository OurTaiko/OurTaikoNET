#!/usr/bin/env bash
# Fills ourtaiko_net from the three old databases (only read), in one transaction, and
# checks the result. Run it from a directory that also holds db/import/import.sql and
# db/check.sql of the commit that is deployed. It can be repeated: each run replaces the
# song and player data, keeps sessions and players' groups. As the superuser: the import makes a foreign-data wrapper and loads
# with triggers off. The old databases are reached over the server's own socket.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
pg() { sudo -n docker exec ourtaiko-postgres psql -U postgres -d ourtaiko_net -X -v ON_ERROR_STOP=1 "$@"; }
trap 'sudo -n docker exec ourtaiko-postgres rm -f /tmp/otn-import.sql /tmp/otn-check.sql' EXIT
sudo -n docker cp -q "$here/import.sql" ourtaiko-postgres:/tmp/otn-import.sql
sudo -n docker cp -q "$here/check.sql" ourtaiko-postgres:/tmp/otn-check.sql
start=$(date +%s)
pg -q -v fanmade_db=ourtaiko_fanmade -v official_db=ourtaiko_database -v profile_db=ourtaiko_profile \
   -v src_host=/var/run/postgresql -v src_port=5432 -v src_user=postgres -v src_password= \
   -f /tmp/otn-import.sql
echo "import finished in $(( $(date +%s) - start )) s"
pg -q -f /tmp/otn-check.sql
echo "check.sql passed"
pg -At -F ' | ' -c "select 'tables ' || jsonb_array_length(report), 'same content ' || (select count(*) from jsonb_array_elements(report) r where (r->>'same_content')::boolean), 'counts differ ' || (select count(*) from jsonb_array_elements(report) r where (r->>'origin_rows') is distinct from (r->>'target_rows')), 'rows ' || (select sum((r->>'target_rows')::bigint) from jsonb_array_elements(report) r) from net.import_runs order by finished_at desc limit 1"
echo "leftovers:"; pg -At -c "select 'servers', count(*) from pg_foreign_server union all select 'import schemas', count(*) from pg_namespace where nspname in ('net_import','old_fanmade','old_official','old_profile')"
