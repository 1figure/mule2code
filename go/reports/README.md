# go/reports/ — baseline quality reports

Snapshots taken right after the test suite of prompt 3 first passed (2026-09-25), kept as the
reference to compare later changes against. They are outputs, not inputs: nothing reads them.

| File | What |
|---|---|
| `coverage-<date>.txt` | `go test -cover ./...` per package, followed by `go tool cover -func` per function; the last line is the statement total |
| `coverage-<date>.prof` | the raw profile the two other coverage files were derived from |
| `coverage-<date>.html` | the same profile as browsable, line-highlighted source |
| `complexity-<date>.txt` | cyclomatic complexity per function (`gocyclo`, non-test files, highest first, average last) |

Baseline figures: statement coverage **90.5 %** over all packages (`internal/testutil` has no
tests and counts as 0 %; every other package is between 86.5 % and 100 %);
average cyclomatic complexity **5.1**, maximum 19 (`batch.(*Job).Run`). Taken after the
TIMESTAMP_PRECISION (date/instant) change that matched the C# port (2026-09-25).

Regenerate with `reports/generate.sh [date]` (`RUN_POSTGRES=1` with Docker running to include the
PostgreSQL test); it runs the commands below from `go/`:

```bash
go test -count=1 -cover -coverprofile=reports/coverage-$D.prof ./... > reports/coverage-$D.txt 2>&1
{ echo; echo "=== per function ==="; go tool cover -func=reports/coverage-$D.prof; } >> reports/coverage-$D.txt
go tool cover -html=reports/coverage-$D.prof -o reports/coverage-$D.html
go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -avg . | grep -v _test.go > reports/complexity-$D.txt
```

`gocyclo` is run through `go run …@latest`, so it is not a dependency of the module.
