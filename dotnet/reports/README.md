# dotnet/reports/ — baseline quality reports

Snapshots taken right after the test suite of prompt 3 passed (2026-09-25), kept as the reference to
compare later changes against. They are outputs, not inputs: nothing reads them. Same idea as
`../../go/reports/`, with the .NET-native tools.

| File | What |
|---|---|
| `coverage-<date>.txt` | the `dotnet test` result line, then ReportGenerator's text summary: totals and line coverage per assembly and class |
| `coverage-<date>.cobertura.xml` | the raw coverlet profile the other two coverage files were derived from (line and branch hits per file) |
| `coverage-<date>.html` | the same as a one-page HTML summary (per-class bars; no line highlighting — see *Regenerate* for that) |
| `complexity-<date>.txt` | cyclomatic complexity per method in `src/`, highest first, average last |
| `generate.sh`, `complexity.py` | regenerate everything above |
| `CodeMetricsConfig.txt`, `metrics.globalconfig` | analyzer configuration used only by `generate.sh` (see below) |

Baseline figures (own code; source-generated regex runners excluded): 233 tests, line coverage
**92.4 %**, branch coverage **85.5 %**, method coverage **98.6 %** over 295 methods.
Per assembly: `Contacts.Core` 92.7 %, `Contacts.Api` 96.5 %, `Contacts.Batch` 80.8 % (the console
host's `watch` loop and the Ctrl-C / SIGTERM handlers are not exercised). Every DataWeave transform
is at 100 %. Cyclomatic complexity: **55 methods** have any branching, average **3.40**, maximum
**12** (`CsvRows.ParseLine`), then `Batch.Program.Main` 11, `NormalizeContactsAsync` 10 and
`BatchJob.RunAsync` 10; the remaining ~240 methods are straight-line (complexity 1) and are not listed.

Taken after the code review of 2026-09-25 was applied (on-complete failures recorded in the
statistics, DataWeave-style coercion messages, literal JSON escaping in the errors file, the
MAPPING.md traceability test), after the timestamp columns were aligned with Mule's `as Date`
(date only by default, deviation 4), and after the `TIMESTAMP_PRECISION` switch was added.

## Regenerate

```bash
dotnet/reports/generate.sh              # today's date; RUN_POSTGRES=0 to leave the container test out
dotnet/reports/generate.sh 2026-09-25   # explicit date
```

Needs the .NET 8 SDK, `python3`, and Docker unless `RUN_POSTGRES=0`. What it runs:

1. `dotnet test --collect:"XPlat Code Coverage"` (coverlet, already a test dependency) → cobertura XML.
2. `dotnet reportgenerator` — a repo-local tool pinned in `../.config/dotnet-tools.json`
   (`dotnet tool restore` fetches it; nothing is installed globally) → text and HTML summaries.
   For a line-highlighted report, run it yourself with `-reporttypes:HtmlInline` into `tmp/`; it is
   9 MB, which is why it is not kept here.
3. `dotnet build -p:CollectMetrics=true -p:TreatWarningsAsErrors=false` — `Directory.Build.props`
   then adds `metrics.globalconfig` (turns on the Roslyn **CA1502** analyzer) and
   `CodeMetricsConfig.txt` (sets its threshold to 1), so every branching method is reported with its
   cyclomatic complexity; `complexity.py` tabulates the warnings. This replaces
   `Microsoft.CodeAnalysis.Metrics`, whose `Metrics.exe` is Windows-only. A normal build never sees
   either file. The script ends with a plain `dotnet build` so the tree is left in its normal state.
