#!/usr/bin/env bash
# Runs the Go tests with coverage and fails when total coverage is below the
# minimum. Extra arguments go to `go test` (CI passes -race).
#
#   scripts/coverage.sh [go test flags...]      MIN_COVERAGE=80 by default
set -euo pipefail

min="${MIN_COVERAGE:-80}"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

go test "$@" -covermode=atomic -coverprofile=coverage.out ./...
# internal/testrepo is a test helper with no tests of its own; don't count it.
grep -v '/internal/testrepo/' coverage.out > coverage.filtered.out

go tool cover -func=coverage.filtered.out | awk '/^total:/ || $3+0 < 60'
total="$(go tool cover -func=coverage.filtered.out | awk '/^total:/ { sub("%", "", $3); print $3 }')"
echo "Total coverage: ${total}% (minimum ${min}%)"
if awk -v t="$total" -v m="$min" 'BEGIN { exit !(t + 0 < m + 0) }'; then
  echo "::error::Go coverage ${total}% is below the ${min}% minimum"
  exit 1
fi
