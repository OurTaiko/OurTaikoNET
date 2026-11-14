#!/usr/bin/env bash
# Creates the role and the empty database ourtaiko_net, and writes the container's settings.
# The password is made here and written only to the root-only settings file.
set -euo pipefail
umask 077
pg() { sudo -n docker exec ourtaiko-postgres psql -U postgres -X -v ON_ERROR_STOP=1 "$@"; }

db=$(pg -At -c "select count(*) from pg_database where datname='ourtaiko_net'")
role=$(pg -At -c "select count(*) from pg_roles where rolname='ourtaiko_net'")
if [ "$db" != 0 ] || [ "$role" != 0 ]; then echo "ourtaiko_net already exists (database $db, role $role); nothing done"; exit 1; fi
if sudo -n test -e /etc/ourtaiko-net/docker/api.env; then echo "api.env already exists; nothing done"; exit 1; fi

work=$(mktemp -d)
trap 'rm -rf "$work"; sudo -n docker exec ourtaiko-postgres rm -f /tmp/otn-role.sql' EXIT
pw=$(openssl rand -hex 24)
printf "CREATE ROLE ourtaiko_net LOGIN PASSWORD '%s';\n" "$pw" > "$work/role.sql"
sudo -n docker cp -q "$work/role.sql" ourtaiko-postgres:/tmp/otn-role.sql
# Output discarded: an error message would repeat the statement.
pg -q -f /tmp/otn-role.sql >/dev/null 2>&1 || { echo "creating the role failed"; exit 1; }
pg -q -c "CREATE DATABASE ourtaiko_net OWNER ourtaiko_net"
# As the other databases here: nobody but the owner (and the superuser) connects.
pg -q -c "REVOKE CONNECT ON DATABASE ourtaiko_net FROM PUBLIC"

cat > "$work/api.env" <<ENV
DATABASE_URL=postgres://ourtaiko_net:${pw}@postgres:5432/ourtaiko_net?sslmode=disable
APP_ORIGIN=https://net.ourtaiko.org
COOKIE_SECURE=true
TRUSTED_PROXY_CIDRS=172.20.0.1/32
ENV
printf 'STORAGE_BACKEND=local\n' > "$work/storage.env"
sudo -n install -m 0600 -o root -g root "$work/api.env" /etc/ourtaiko-net/docker/api.env
sudo -n install -m 0600 -o root -g root "$work/storage.env" /etc/ourtaiko-net/docker/storage.env
unset pw

echo "created:"
pg -At -c "select datname, pg_get_userbyid(datdba) from pg_database where datname='ourtaiko_net'"
pg -At -c "select rolname, rolsuper, rolcreatedb, rolcreaterole from pg_roles where rolname='ourtaiko_net'"
sudo -n ls -l /etc/ourtaiko-net/docker | awk 'NR>1{print $1, $3, $4, $9}'
sudo -n sed -E 's#(DATABASE_URL=postgres://[^:]+:)[^@]+@#\1[hidden]@#' /etc/ourtaiko-net/docker/api.env
