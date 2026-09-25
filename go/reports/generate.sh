#!/usr/bin/env bash
# Regenerates the baseline reports: coverage (text, profile, HTML) and cyclomatic complexity.
# Run from anywhere; set RUN_POSTGRES=1 (with Docker) to include the PostgreSQL container test.
set -euo pipefail
cd "$(dirname "$0")/.."
D=${1:-$(date +%F)}
go test -count=1 -cover -coverprofile="reports/coverage-$D.prof" ./... > "reports/coverage-$D.txt" 2>&1
{ echo; echo "=== per function ==="; go tool cover -func="reports/coverage-$D.prof"; } >> "reports/coverage-$D.txt"
go tool cover -html="reports/coverage-$D.prof" -o "reports/coverage-$D.html"
# gocyclo runs through `go run …@latest`, so it is not a dependency of the module.
go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -avg . | grep -v _test.go > "reports/complexity-$D.txt"
tail -1 "reports/coverage-$D.txt"; tail -1 "reports/complexity-$D.txt"
