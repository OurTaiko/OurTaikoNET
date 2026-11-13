#!/bin/sh
# For each file named: try giving one package the other's text; keep it if everything still
# compiles and both packages' tests pass, otherwise put the file back and say why.
#   tools/try-same-text.sh <o2f|f2o> file...   (o2f: fanmade takes the official text)
# Needs what scripts/test.sh needs. Part of merging the two song packages; see
# tools/compare-sources.py for what is left.
cd "$(dirname "$0")/.." || exit 1
export DATABASE_TEST_URL="${DATABASE_TEST_URL:-postgres:///postgres?host=/tmp}"
dir="$1"; shift
F=internal/fanmade/httpapi; O=internal/official/httpapi
for f in "$@"; do
  if cmp -s $F/$f $O/$f; then echo "SAME   $f"; continue; fi
  if [ "$dir" = o2f ]; then from=$O/$f; to=$F/$f; else from=$F/$f; to=$O/$f; fi
  cp $to "${TMPDIR:-/tmp}/try-same-text-backup.go"; cp $from $to
  if ! out=$(go vet ./internal/fanmade/... ./internal/official/... 2>&1); then
    cp "${TMPDIR:-/tmp}/try-same-text-backup.go" $to
    echo "NOBUILD $f ($dir): $(echo "$out" | grep -E '\.go:[0-9]+' | head -3 | cut -c1-170 | tr '\n' '|')"
    continue
  fi
  if out=$(go test -count=1 ./internal/fanmade/httpapi/ ./internal/official/httpapi/ 2>&1); then
    echo "KEPT   $f ($dir)"
  else
    cp "${TMPDIR:-/tmp}/try-same-text-backup.go" $to
    echo "FAILS  $f ($dir): $(echo "$out" | grep -E '^--- FAIL|_test.go:[0-9]+' | head -4 | cut -c1-200 | tr '\n' '|')"
  fi
done
