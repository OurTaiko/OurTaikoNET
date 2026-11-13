#!/bin/sh
# Runs the three old sites (the two song sites and the players' site) and OurTaikoNet side by
# side on the same data, on this machine, and compares their answers
# (tools/compare-answers.py).
#
#   tools/compare-local.sh <fanmade db> <official db> <profile db>
#
# The three names are local databases in the old layout, for example the ones
# db/dev/pull-backups.sh restores. They are only copied. The old sites are built from their
# own repositories, which are not changed: FANMADE_REPO, DATABASE_REPO and PROFILE_REPO
# (default: next to this repository). A stand-in account center gives every player a name and lets nobody sign
# in, so only public answers are compared.
#
# Needs TJAPARSER_BIN (TJAParser v0.13.0) and ffmpeg on PATH, as the servers do.
# Ports used on 127.0.0.1: 18999, 18080, 18081, 18083, 18090. CHARTS limits how many charts are asked
# about one by one (default 0 = all); SHOW how many differences are listed (default 40).
set -eu
here="$(cd "$(dirname "$0")/.." && pwd)"
fanmade_repo="${FANMADE_REPO:-$here/../Fanmade}"
database_repo="${DATABASE_REPO:-$here/../Database}"
profile_repo="${PROFILE_REPO:-$here/../OurTaikoProfile}"
export PGHOST="${PGHOST:-/tmp}" PGPORT="${PGPORT:-5432}"
src_fanmade="${1:?fanmade database name}"
src_official="${2:?official database name}"
src_profile="${3:?profile database name}"
: "${TJAPARSER_BIN:?set TJAPARSER_BIN to a build of TJAParser v0.13.0}"
export TJAPARSER_BIN
work="$(mktemp -d)"
pids=""
admin() { psql -X -q -v ON_ERROR_STOP=1 postgres:///postgres "$@"; }
cleanup() {
  for pid in $pids; do kill "$pid" 2>/dev/null || true; done
  sleep 1
  for d in otn_cmp_fanmade otn_cmp_official otn_cmp_profile otn_cmp_net; do
    admin -c "DROP DATABASE IF EXISTS $d WITH (FORCE)" 2>/dev/null || true
  done
  rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM

echo "== build"
# Each old server embeds its built website; a comparison needs only the API, so that one file
# is replaced for this build without touching the repository.
stub() {
  mkdir -p "$work/stub-$1"
  printf 'package %s\n\nimport "io/fs"\n\nfunc Site() fs.FS { return nil }\n' "$1" > "$work/stub-$1/site.go"
  printf '{"Replace":{"%s/site.go":"%s/stub-%s/site.go"}}' "$2" "$work" "$1" > "$work/overlay-$1.json"
}
fanmade_repo="$(cd "$fanmade_repo" && pwd)"
database_repo="$(cd "$database_repo" && pwd)"
profile_repo="$(cd "$profile_repo" && pwd)"
stub fanmade "$fanmade_repo"
stub webapp "$database_repo"
stub profile "$profile_repo"
(cd "$fanmade_repo" && go build -overlay "$work/overlay-fanmade.json" -o "$work/old-fanmade" ./cmd/server)
(cd "$database_repo" && go build -overlay "$work/overlay-webapp.json" -o "$work/old-official" ./cmd/server)
(cd "$profile_repo" && go build -overlay "$work/overlay-profile.json" -o "$work/old-profile" ./cmd/server)
(cd "$here" && go build -o "$work/net" ./cmd/server)

echo "== data"
admin -c "DROP DATABASE IF EXISTS otn_cmp_fanmade WITH (FORCE)" -c "DROP DATABASE IF EXISTS otn_cmp_official WITH (FORCE)" \
      -c "DROP DATABASE IF EXISTS otn_cmp_profile WITH (FORCE)" -c "DROP DATABASE IF EXISTS otn_cmp_net WITH (FORCE)" \
      -c "CREATE DATABASE otn_cmp_fanmade TEMPLATE $src_fanmade" -c "CREATE DATABASE otn_cmp_official TEMPLATE $src_official" \
      -c "CREATE DATABASE otn_cmp_profile TEMPLATE $src_profile" -c "CREATE DATABASE otn_cmp_net"
DATABASE_URL="postgres:///otn_cmp_net" "$work/net" -init
SRC_HOST="$PGHOST" SRC_PORT="$PGPORT" "$here/db/import.sh" postgres:///otn_cmp_net otn_cmp_fanmade otn_cmp_official otn_cmp_profile >"$work/import.log" 2>&1 || { cat "$work/import.log" >&2; exit 1; }
users_fanmade="$(psql -X -At postgres:///otn_cmp_fanmade -c "SELECT string_agg('--user ' || id, ' ') FROM users")"
users_official="$(psql -X -At postgres:///otn_cmp_official -c "SELECT string_agg('--user ' || id, ' ') FROM users")"
users_all="$(psql -X -At postgres:///otn_cmp_net -c "SELECT string_agg('--user ' || id, ' ') FROM net.users")"

echo "== servers"
for port in 18999 18080 18081 18083 18090; do
  if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$port/"; then
    echo "port $port is already in use; a server from an earlier run may still be running" >&2
    exit 1
  fi
done
export SSO_ISSUER=http://127.0.0.1:18999 SSO_SERVICE_ID=compare SSO_CLIENT_ID=compare SSO_CLIENT_SECRET=compare
export SSO_SERVICE_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export SESSION_ENCRYPTION_KEY=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export SSO_REDIRECT_URL=http://127.0.0.1:18090/callback APP_ORIGIN=http://127.0.0.1:18090
python3 "$here/tools/fake-sso.py" 18999 & pids="$pids $!"
# Each server runs in a folder of its own so that no .env is read.
start() {
  name="$1"; shift
  mkdir -p "$work/run-$name"
  # exec: the process started here is the server itself, so stopping it at the end stops the server.
  (cd "$work/run-$name" && exec env "$@" STORAGE_DIR="$work/run-$name/files" "$work/$name" >"$work/$name.log" 2>&1) & pids="$pids $!"
}
start old-fanmade DATABASE_URL=postgres:///otn_cmp_fanmade LISTEN_ADDR=127.0.0.1:18080
start old-official DATABASE_URL=postgres:///otn_cmp_official LISTEN_ADDR=127.0.0.1:18081
# The old players' site reads the two old song sites; it is told to call OurTaiko Database
# "official", the name that source has in OurTaikoNet, so that the answers can be compared.
start old-profile DATABASE_URL=postgres:///otn_cmp_profile LISTEN_ADDR=127.0.0.1:18083 \
  'SCORE_SOURCES=[{"id":"fanmade","name":"OurTaiko Fanmade","api":"http://127.0.0.1:18080","site":"https://fanmade.ourtaiko.org"},{"id":"official","name":"OurTaiko Database","api":"http://127.0.0.1:18081","site":"https://database.ourtaiko.org"}]'
start net DATABASE_URL=postgres:///otn_cmp_net LISTEN_ADDR=127.0.0.1:18090 LEGACY_FANMADE_HOST=fanmade.ourtaiko.org LEGACY_OFFICIAL_HOST=database.ourtaiko.org \
  LEGACY_PROFILE_HOST=profile.ourtaiko.org FANMADE_SITE=https://fanmade.ourtaiko.org OFFICIAL_SITE=https://database.ourtaiko.org
for port in 18080 18081 18083 18090; do
  n=0
  until curl -s -o /dev/null "http://127.0.0.1:$port/healthz"; do
    n=$((n + 1))
    [ "$n" -lt 60 ] || { echo "the server on port $port did not start:" >&2; tail -5 "$work"/*.log >&2; exit 1; }
    sleep 0.5
  done
done

echo "== compare"
status=0
# Different on purpose: the old sites each remembered when a player first signed in and was
# last active there. OurTaikoNet has one set of players, so it shows the earliest first
# sign-in and the latest activity across both, and the player list, which is ordered by
# activity, comes in another order. Set STRICT=1 to compare these too.
expected="--ignore firstLoginAt --ignore lastActiveAt --skip /users"
# The system playlists are described in the same words on both sources now ("songs"); the old
# official site said "charts".
official_wording="--ignore-in /playlists/system:description"
[ -z "${STRICT:-}" ] || expected=""
# shellcheck disable=SC2086
python3 "$here/tools/compare-answers.py" --old http://127.0.0.1:18080 --new http://127.0.0.1:18090 --legacy-host fanmade.ourtaiko.org --source fanmade --charts "${CHARTS:-0}" --show "${SHOW:-40}" $expected $users_fanmade || status=1
# shellcheck disable=SC2086
python3 "$here/tools/compare-answers.py" --old http://127.0.0.1:18081 --new http://127.0.0.1:18090 --legacy-host database.ourtaiko.org --source official --charts "${CHARTS:-0}" --show "${SHOW:-40}" $expected $official_wording $users_official || status=1
# shellcheck disable=SC2086
python3 "$here/tools/compare-answers.py" --old http://127.0.0.1:18083 --new http://127.0.0.1:18090 --legacy-host profile.ourtaiko.org --players --show "${SHOW:-40}" $expected $users_all || status=1
exit "$status"
