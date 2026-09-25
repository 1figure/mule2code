# go/ — Go port

The two Mule applications as plain Go services (standard library plus a SQLite and a Postgres
driver), produced by prompt 2 in `../PROMPTS.md`. Element-by-element correspondence and the
deliberate deviations are in `../MAPPING.md`.

```
go/
  go.mod                      module muletocode (Go 1.25+, the floor set by testcontainers-go)
  cmd/batch/main.go           CSV -> DB batch job: watch inbox | -once | -file <path>
  cmd/api/main.go             GET /contacts, POST /contacts/normalize
  internal/batch/             batch:job semantics (steps, accept policies, aggregator, maxFailedRecords, statistics)
  internal/contacts/          model, CSV reader, "CSV to SQL" transform + coercions, CSV line writer, e-mail validation
  internal/config/            global-property env + mule-props-${env}.yaml -> environment variables
  internal/db/                Database connector: sqlite (modernc.org/sqlite) + postgres (pgx stdlib)
  internal/report/            "Extract Key Statistics", parse-template over the upstream HTML, report sink
  internal/pipeline/          contacts-batch-process-flow, initialization-flow, send-email-report-flow
  internal/zippo/             http:request to api.zippopotam.us
  internal/api/               the two HTTP flows: validations, DataWeave transforms, handlers
  internal/resources/         locates the schema files under ../docker and the upstream e-mail template
  internal/testutil/          fixture access for the tests (paths into ../testdata, JSON canonicalisation)
  internal/*/*_test.go        the test suite of prompt 3, driven by ../testdata (see "Tests")
```

The schema scripts and the e-mail template are read from the repository at run time
(`internal/resources` walks up from the working directory, or the executable, to the directory that
contains `docker/sqlite/contacts.sqlite.sql`), so run the commands from inside the checkout.

## Run

Prerequisite: Go 1.25+ (the port itself needs only 1.23; the Testcontainers test dependency raises the floor).

```bash
cd go
go build ./... && go vet ./...
mkdir -p data/inbox && cp ../testdata/contact-data-100-with-errors.csv data/inbox/
go run ./cmd/batch -once                  # process the inbox once, report on stdout
go run ./cmd/api                          # http://localhost:8082
curl 'localhost:8082/contacts?city=Saint%20Louis'
curl -X POST localhost:8082/contacts/normalize -H 'content-type: application/json' \
     -d @../testdata/api/normalize-request.json
```

Batch host modes:

| Arguments | Behaviour |
|---|---|
| *(none)* | watch `<DATA_DIR>/inbox` every `POLL_SECONDS` seconds, like the SFTP listener; reports go to `<DATA_DIR>/reports/` |
| `-once` | process every `*.csv` in the inbox once, report on stdout, exit (1 if a file failed) |
| `-file <path>` | process one file, report on stdout |
| `-verbose` | log the flow's TRACE loggers (debug level) |

Per file: rows are inserted, the file moves to `<DATA_DIR>/processed/<stem>.<yyyyMMddTHHmmssSSS>.<ext>`,
failed rows (if any) are written next to it as `<stem>.<timestamp>.errors.json`, and a broken file
moves to `<DATA_DIR>/failed/` instead. Non-CSV files in the inbox are ignored.

## Configuration

All settings are environment variables (`internal/config`). Nothing is required; the defaults give
the SQLite quick start above.

| Variable | Default | Used by | Mule property |
|---|---|---|---|
| `DB_PROFILE` | `sqlite` | both | `env` (`sqlite` \| `postgres`) |
| `SQLITE_PATH` | `<DATA_DIR>/contacts.db` | both | `db.url` / `postgres_db.url` (sqlite profile) |
| `POSTGRES_DSN` | `postgres://contacts:contacts@localhost:5432/contacts` | both | `postgres_db.*` (postgres profile) |
| `DATA_DIR` | `data` | batch | `sftp.working_dir` + `new_dir` / `processed_dir` / `failed_dir` |
| `BATCH_BLOCK_SIZE` | `10000` | batch | `batch.job.block_size` |
| `BATCH_AGGREGATOR_SIZE` | `1000` | batch | `batch.aggregator.main.size` |
| `POLL_SECONDS` | `10` | batch | the listener's fixed-frequency scheduler |
| `HTTP_ADDR` | `0.0.0.0:8082` | api | `http.host` / `http.port` |
| `ZIPPOPOTAM_BASE_URL` | `https://api.zippopotam.us` | api | `zippopotam.host` / `port` / `protocol` |
| `NORMALIZE_MAX_CONTACTS` | `50` | api | `normalize.max_contacts` |
| `TIMESTAMP_PRECISION` | `date` | batch | how `created_date`, `last_modified_date`, `system_mod_stamp` are coerced: `date` = Mule's `as Date` (calendar date as written), `instant` = the full timestamp in UTC (MAPPING.md deviation 4) |

Database profiles:

- **sqlite** — the file is created on first start and `../docker/sqlite/contacts.sqlite.sql` is applied
  every start (idempotent). Booleans are stored as `0/1` and dates as `YYYY-MM-DD` — including the
  three timestamp columns under the default `TIMESTAMP_PRECISION=date`, because the original coerces
  them with `as Date`, which drops the time part; with `instant` they are stored as
  `yyyy-MM-ddTHH:mm:ss.fffZ` (MAPPING.md deviation 4). Both ports bind the same representations so
  the rows are byte-identical (deviation 7).
- **postgres** — `docker compose -f ../docker/docker-compose.yml up -d db`, then `DB_PROFILE=postgres`.
  `../docker/init/01-contacts.sql` is applied only if the `contacts` table does not exist yet.

Relative paths (`DATA_DIR`, `SQLITE_PATH`) resolve against the working directory, i.e. `go/` when
using the commands above. `data/` and `*.db` are git-ignored.

## HTTP error mapping

`{"error": "<description>"}` bodies, as `<http:error-response>` produced:

| Status | Cause |
|---|---|
| 400 | a validation from the XML fails (message copied verbatim), an unparsable or non-JSON body, or a failed `as Number` in "Zippopotam to result" (an `EXPRESSION` error in Mule) |
| 502 | zippopotam.us unreachable, timed out, or answered 500/502/503; the body is the bare description, the `Upstream error:` prefix is in the log only |
| 503 | the database is unreachable or the query fails |
| 500 | anything else |

## Tests

Every expected value comes from `../testdata`; the fixtures are referenced by path, never copied
into this tree (a test that hands a file to the pipeline copies it into its own temp directory,
because the pipeline moves it).

| Layer | Where | What |
|---|---|---|
| Golden transforms | `internal/contacts`, `internal/zippo`, `internal/api` (`TestTransforms`) | "CSV to SQL" on the first record vs `expected/first-record.json`; the two `Record` lines of `expected/errors-100-with-errors.json`; "Zippopotam to result" / "Not found result" vs `api/normalize-response.json`; the e-mail validation on the samples |
| Report formatting | `internal/report` | `mm:ss.SSS`, `#,###`, "Extract Key Statistics", the upstream template rendered, both sinks |
| Batch engine | `internal/batch` | synthetic records: step order, accept policies, aggregator block size, block-level failure, processor failure vs aggregator, `maxFailedRecords`, statistics, input-phase error, cancellation |
| End-to-end (SQLite) | `internal/pipeline` | both sample files vs `expected/batch-report-*.json` and the errors file, a coercion error failing one block, empty file → `failed/`, the renamed file's timestamp, non-CSV files ignored, the watcher |
| Repository (PostgreSQL) | `internal/db/postgres_test.go` | the same contract as the SQLite test against a Testcontainers `postgres:16-alpine`; opt-in: runs only with `RUN_POSTGRES=1` and Docker, skipped otherwise |
| API | `internal/api` | in-process host, upstream stubbed from `api/zippopotam-*.json`: golden responses, every 400 case, validation before any upstream call, 502/503/500, the required properties of `openapi.yaml` |

```bash
gofmt -l .                        # no output expected
go vet ./...
go test -cover ./...              # RUN_POSTGRES=1 also runs the PostgreSQL container test
```

`cmd/api` and `cmd/batch` expose `run(ctx, args, …)` so their flags, exit codes and shutdown are
tested in-process too (`cmd/*/main_test.go`).

Baseline coverage and cyclomatic-complexity reports from the first passing run are kept under
[`reports/`](reports/README.md), with the commands to regenerate them.
