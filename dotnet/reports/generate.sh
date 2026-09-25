#!/usr/bin/env bash
# Regenerates the baseline quality reports under dotnet/reports/ (see README.md there).
# Usage, from anywhere:  dotnet/reports/generate.sh [YYYY-MM-DD]
# Needs the .NET 8 SDK, python3, and Docker unless RUN_POSTGRES=0.
set -euo pipefail
cd "$(dirname "$0")/.."
D=${1:-$(date +%F)}
OUT=reports
COV_DIR=tests/Contacts.Tests/TestResults

# 1. Tests with coverlet's cobertura output (the Postgres contract included unless RUN_POSTGRES=0).
rm -rf "$COV_DIR"
RUN_POSTGRES=${RUN_POSTGRES:-1} dotnet test MuleToCode.sln --collect:"XPlat Code Coverage" > "$OUT/test-$D.log" 2>&1 || {
  cat "$OUT/test-$D.log"; exit 1; }
COBERTURA=$(ls "$COV_DIR"/*/coverage.cobertura.xml)
cp "$COBERTURA" "$OUT/coverage-$D.cobertura.xml"

# 2. ReportGenerator (repo-local tool, .config/dotnet-tools.json): one-page HTML summary + text summary.
#    Source-generated regex code is excluded; it is not ours. A line-highlighted report is 9 MB, so it is
#    not kept here: `dotnet reportgenerator -reports:reports/coverage-<date>.cobertura.xml -targetdir:tmp/cov
#    -reporttypes:HtmlInline` produces it locally from the raw file.
WORK=$(mktemp -d)
dotnet tool restore > /dev/null
dotnet reportgenerator "-reports:$COBERTURA" "-targetdir:$WORK" \
  "-reporttypes:HtmlSummary;TextSummary" "-classfilters:-*RegexGenerator*" -verbosity:Error \
  | grep -v 'does not exist (any more)' || true
mv "$WORK/summary.html" "$OUT/coverage-$D.html"
{
  grep -E 'Passed!|Failed!' "$OUT/test-$D.log"
  echo
  cat "$WORK/Summary.txt"
} > "$OUT/coverage-$D.txt"
rm -rf "$WORK" "$OUT/test-$D.log"

# 3. Cyclomatic complexity per method, from the CA1502 analyzer with its threshold at 1
#    (reports/CodeMetricsConfig.txt + reports/metrics.globalconfig, wired in by Directory.Build.props).
dotnet build MuleToCode.sln --no-incremental -nologo -v:q \
  -p:CollectMetrics=true -p:TreatWarningsAsErrors=false 2>&1 \
  | python3 reports/complexity.py "$D" > "$OUT/complexity-$D.txt"
# Leave a normal build behind so the next `dotnet test --no-build` does not see analyzer-config inputs.
dotnet build MuleToCode.sln -nologo -v:q > /dev/null

echo "reports written: $OUT/coverage-$D.txt, $OUT/coverage-$D.html, $OUT/coverage-$D.cobertura.xml, $OUT/complexity-$D.txt"
