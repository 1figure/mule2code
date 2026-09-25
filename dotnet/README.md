# dotnet/ — C# port

The two Mule applications as plain .NET 8 services, produced by prompt 1 in `../PROMPTS.md`.
Element-by-element correspondence and the deliberate deviations are in `../MAPPING.md`.

```
dotnet/
  MuleToCode.sln
  Directory.Build.props        net8.0, nullable, implicit usings, warnings as errors, XML docs
  src/Contacts.Core/           everything shared by the two hosts (see below)
  src/Contacts.Batch/          console host of batch-contacts-csv-to-db: watch inbox | --once | --file
  src/Contacts.Api/            minimal API of contacts-api: GET /contacts, POST /contacts/normalize
  tests/Contacts.Tests/        xunit, driven by ../testdata (prompt 3)
  tests/Directory.Build.props  keeps test projects out of publish/pack outputs
  reports/                     baseline coverage + complexity snapshots and the script that regenerates them
  .config/dotnet-tools.json    repo-local ReportGenerator (dotnet tool restore)
```

`Contacts.Core` by folder:

| Folder | Mule counterpart |
|---|---|
| `Settings/` | `global-property env` + `mule-props-${env}.yaml` → environment variables |
| `Csv/` | DataWeave `application/csv` reader and the `write(..., "application/csv")` line writer |
| `Model/`, `Mapping/` | the "CSV to SQL" transform and its `as Number` / `as Boolean` / `as Date` coercions |
| `Validation/` | `<validation:is-email>` |
| `Batch/` | the generic `batch:job` engine (steps, accept policies, aggregators, `maxFailedRecords`, statistics) |
| `Data/` | `<db:config>`, `<db:bulk-insert>`, `<db:select>` on Microsoft.Data.Sqlite / Npgsql via Dapper |
| `Report/` | "Extract Key Statistics", `parse-template` over the upstream HTML, the e-mail replaced by a sink |
| `Pipeline/` | `contacts-batch-process-flow`, `initialization-flow`, `send-email-report-flow` |
| `Zippo/` | `<http:request-config>` to api.zippopotam.us |
| `Api/` | the validations and DataWeave transforms of the two HTTP flows |

## Run

Prerequisite: .NET 8 SDK (`dotnet --version` → 8.0.x).

```bash
cd dotnet
dotnet build                                          # warnings are errors
mkdir -p data/inbox && cp ../testdata/contact-data-100-with-errors.csv data/inbox/
dotnet run --project src/Contacts.Batch -- --once     # process the inbox once, report on stdout
dotnet run --project src/Contacts.Api                 # http://localhost:8082
curl 'localhost:8082/contacts?city=Saint%20Louis'
curl -X POST localhost:8082/contacts/normalize -H 'content-type: application/json' \
     -d @../testdata/api/normalize-request.json
```

Batch host modes:

| Arguments | Behaviour |
|---|---|
| *(none)* | watch `<DATA_DIR>/inbox` every `POLL_SECONDS` seconds, like the SFTP listener; reports go to `<DATA_DIR>/reports/` |
| `--once` | process every `*.csv` in the inbox once, report on stdout, exit (1 if a file failed) |
| `--file <path>` | process one file, report on stdout |
| `--verbose` | log the flow's TRACE loggers (`Debug` level) |

Per file: rows are inserted, the file moves to `<DATA_DIR>/processed/<stem>.<yyyyMMddTHHmmssfff>.<ext>`,
failed rows (if any) are written next to it as `<stem>.<timestamp>.errors.json`, and a broken file
moves to `<DATA_DIR>/failed/` instead. Non-CSV files in the inbox are ignored.

## Configuration

All settings are environment variables (`Settings/AppSettings.cs`). Nothing is required; the defaults
give the SQLite quick start above.

| Variable | Default | Used by | Mule property |
|---|---|---|---|
| `DB_PROFILE` | `sqlite` | both | `env` (`sqlite` \| `postgres`) |
| `SQLITE_PATH` | `<DATA_DIR>/contacts.db` | both | `db.url` / `postgres_db.url` (sqlite profile) |
| `POSTGRES_CONNECTION_STRING` | `Host=localhost;Port=5432;Database=contacts;Username=contacts;Password=contacts` | both | `postgres_db.*` (postgres profile) |
| `DATA_DIR` | `data` | batch | `sftp.working_dir` + `new_dir` / `processed_dir` / `failed_dir` |
| `BATCH_BLOCK_SIZE` | `10000` | batch | `batch.job.block_size` |
| `BATCH_AGGREGATOR_SIZE` | `1000` | batch | `batch.aggregator.main.size` |
| `POLL_SECONDS` | `10` | batch | the listener's fixed-frequency scheduler |
| `HTTP_URL` | `http://0.0.0.0:8082` | api | `http.host` / `http.port` |
| `ZIPPOPOTAM_BASE_URL` | `https://api.zippopotam.us` | api | `zippopotam.host` / `port` / `protocol` |
| `NORMALIZE_MAX_CONTACTS` | `50` | api | `normalize.max_contacts` |
| `TIMESTAMP_PRECISION` | `date` | batch | none — `date` is Mule's `as Date` on the three timestamp columns; `instant` keeps the full timestamp (MAPPING.md deviation 4) |

Database profiles:

- **sqlite** — the file is created on first start and `../docker/sqlite/contacts.sqlite.sql` is applied
  every start (idempotent). Booleans are stored as `0/1`, dates as `YYYY-MM-DD` — including the three
  timestamp columns, which the original coerces with `as Date` — so rows are byte-identical to the Go
  port (MAPPING.md deviations 4 and 7). With `TIMESTAMP_PRECISION=instant` those three columns hold
  `2025-05-01T11:39:15.257Z`-style UTC text instead.
- **postgres** — `docker compose -f ../docker/docker-compose.yml up -d db`, then `DB_PROFILE=postgres`.
  `../docker/init/01-contacts.sql` is applied only if the `contacts` table does not exist yet.
  The tests never use this compose database; their Postgres is a Testcontainers throw-away (see *Tests*).

Relative paths (`DATA_DIR`, `SQLITE_PATH`) resolve against the working directory, i.e. `dotnet/` when
using the commands above. `data/` and `*.db` are git-ignored.

## HTTP error mapping

`{"error": "<description>"}` bodies, as `<http:error-response>` produced:

| Status | Cause |
|---|---|
| 400 | a validation from the XML fails (message copied verbatim), the body is not JSON, or a coordinate in the upstream answer is not a number (`EXPRESSION`) |
| 502 | zippopotam.us unreachable, timed out, or answered 500/502/503; the body is the description, the `Upstream error:` prefix is in the log only |
| 503 | the database is unreachable or the query fails |
| 500 | anything else |

## Tests

`tests/Contacts.Tests` reads the shared fixtures from `../testdata/` in place (nothing is copied) and
never invents expected values: every golden comes from `testdata/expected/*.json` or `testdata/api/*.json`.

| Folder | Layer |
|---|---|
| `Golden/` | every DataWeave transform against its golden file; `mm:ss.SSS` / `#,###`; e-mail validation on the fixture's addresses |
| `Batch/` | `batch:job` semantics with synthetic records; the console host's arguments and exit codes |
| `EndToEnd/` | the pipeline on SQLite in a temp directory: 100/0, 98/2 + errors file, coercion block failure, `failed/`, renaming, non-CSV ignored |
| `Data/` | the repository contract on SQLite and, opt-in, on PostgreSQL 16 via Testcontainers (`RUN_POSTGRES=1`, needs Docker; skipped otherwise) |
| `Api/` | the two flows on an in-process host, upstream stubbed from `testdata/api/zippopotam-*.json`, responses checked against `openapi.yaml` |

```bash
dotnet test                                              # SQLite only; the 12 Postgres tests report as skipped
RUN_POSTGRES=1 dotnet test                               # also the Postgres contract (pulls postgres:16-alpine once)
dotnet test --collect:"XPlat Code Coverage"              # cobertura XML under tests/Contacts.Tests/TestResults/
```

## Checks

```bash
dotnet build MuleToCode.sln                       # 0 warnings expected
dotnet format MuleToCode.sln --verify-no-changes
dotnet test MuleToCode.sln
```
