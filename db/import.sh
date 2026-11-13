#!/bin/sh
# Replaces the song and player data of ourtaiko_net with the current contents of the three
# databases it is built from, then checks the result. The sources are only read.
#
#   db/import.sh <target url> <fanmade db name> <official db name> <profile db name>
#
# The three sources are on one PostgreSQL server, as in production. Where it is:
#   SRC_HOST (default /tmp), SRC_PORT (5432), SRC_USER (current user), SRC_PASSWORD (empty)
# The target role must be a superuser: the import creates postgres_fdw and loads with
# triggers off.
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
target="${1:?target database url}"
fanmade="${2:?fanmade database name}"
official="${3:?official (OurTaiko Database) database name}"
profile="${4:?profile database name}"

psql -X -q -v ON_ERROR_STOP=1 \
  -v fanmade_db="$fanmade" -v official_db="$official" -v profile_db="$profile" \
  -v src_host="${SRC_HOST:-/tmp}" -v src_port="${SRC_PORT:-5432}" \
  -v src_user="${SRC_USER:-$(id -un)}" -v src_password="${SRC_PASSWORD:-}" \
  "$target" -f "$here/import/import.sql"
psql -X -q -v ON_ERROR_STOP=1 "$target" -f "$here/check.sql"
