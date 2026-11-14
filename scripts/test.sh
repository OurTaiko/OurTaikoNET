#!/bin/sh
# Runs every Go test of the module against a local PostgreSQL 18.
#
#   DATABASE_TEST_URL  a role that may create databases (default: postgres:///postgres?host=/tmp)
#   TJAPARSER_BIN      tjaparser built from TJAParser v0.13.0 (default: tjaparser on PATH)
#
# ffmpeg, ffprobe and cwebp must be on PATH. Each test gets its own database, copied from a
# template named otn_template_<hash of db/baseline.sql>; templates of older baselines can be
# dropped by hand.
set -eu
cd "$(dirname "$0")/.."
export DATABASE_TEST_URL="${DATABASE_TEST_URL:-postgres:///postgres?host=/tmp}"
if ! "${TJAPARSER_BIN:-tjaparser}" version >/dev/null 2>&1; then
  echo "test.sh: tjaparser not found; set TJAPARSER_BIN to a build of TJAParser v0.13.0" >&2
  exit 1
fi
for tool in ffmpeg ffprobe cwebp; do
  command -v "$tool" >/dev/null || { echo "test.sh: $tool is not on PATH" >&2; exit 1; }
done
test -z "$(gofmt -l cmd db internal tools)" || { echo "test.sh: gofmt would change:" >&2; gofmt -l cmd db internal tools >&2; exit 1; }
go vet ./...
go test -count=1 ./... "$@"
