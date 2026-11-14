#!/bin/sh
# Runs the backend for local development: http://127.0.0.1:8090, configured by .env (see
# README, "Local development"). The website is web/: `pnpm dev` there serves
# http://127.0.0.1:5173 and passes /api on to this server.
#
# Starts the local PostgreSQL first when it is not running and its data directory is where
# Homebrew puts it (PGDATA_DIR to say otherwise).
set -eu
cd "$(dirname "$0")/.."
[ -f .env ] || { echo "dev.sh: no .env here; see README, \"Local development\"" >&2; exit 1; }
# Uploads decode audio, images and video.
for dir in /opt/homebrew/opt/ffmpeg-full/bin; do
  [ -d "$dir" ] && PATH="$dir:$PATH"
done
export PATH
for tool in ffmpeg ffprobe cwebp; do
  command -v "$tool" >/dev/null || echo "dev.sh: $tool is not on PATH; uploads will fail" >&2
done
if ! pg_isready -q -h /tmp; then
  data="${PGDATA_DIR:-/opt/homebrew/var/postgresql@18}"
  [ -d "$data" ] || { echo "dev.sh: PostgreSQL is not running and $data does not exist; start it, or set PGDATA_DIR" >&2; exit 1; }
  echo "dev.sh: starting PostgreSQL in $data (stop it with: pg_ctl -D $data stop)"
  LC_ALL=en_US.UTF-8 pg_ctl -D "$data" -l "$data/server.log" -w start >/dev/null
fi
exec go run ./cmd/server
