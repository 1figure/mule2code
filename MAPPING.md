# Mule → code mapping

How each element of the two Mule applications is expected to land in the C# and Go ports, and the
places where the ports deliberately deviate. The last prompt in `PROMPTS.md` asks Claude Code to check
the generated code against this table and report differences.

## Configuration

| Mule | C# | Go |
|---|---|---|
| `<global-property name="env">` + `properties/mule-props-${env}.yaml` | `AppSettings.FromEnvironment()`: `DB_PROFILE`, `SQLITE_PATH`, `POSTGRES_CONNECTION_STRING`, `DATA_DIR`, `BATCH_BLOCK_SIZE`, `BATCH_AGGREGATOR_SIZE`, `POLL_SECONDS`, `HTTP_URL`, `ZIPPOPOTAM_BASE_URL`, `NORMALIZE_MAX_CONTACTS`, `TIMESTAMP_PRECISION` (deviation 4) | `config.FromEnv()` with the same variable names (`POSTGRES_DSN`, `HTTP_ADDR`) |
| `${batch.job.block_size}` / `${batch.aggregator.main.size}` | `BlockSize` / `AggregatorSize` on the pipeline | same |
| `<db:config><db:generic-connection url= driverClassName=>` | `SqlContactRepository.OpenAsync(profile, connectionString)`; Microsoft.Data.Sqlite or Npgsql | `db.Open(ctx, profile, dsn)`; `modernc.org/sqlite` or `pgx/stdlib` |
| `<sftp:config>` | not ported — see *Deviations* | same |
| `<email:smtp-config>` | `IReportSink` (`FileReportSink`, `ConsoleReportSink`) — see *Deviations* | `report.Sink` (`FileSink`, `StdoutSink`) |
| `<http:listener-config>` | Kestrel (`HTTP_URL`) | `net/http` (`HTTP_ADDR`) |
| `<http:request-config>` | `HttpClient` with `BaseAddress` | `http.Client` + base URL |
| `<validation:config>` | static functions | same |

## `batch-contacts-csv-to-db-impl.xml`

| Mule | C# | Go |
|---|---|---|
| `<sftp:listener directory=new autoDelete moveToDirectory=processed renameTo=#[vars.newFilename]>` with fixed-frequency scheduler | `ContactsPipeline.WatchAsync` polling `<data>/inbox`; successful files are moved to `<data>/processed/<stem>.<ts>.<ext>` | `Pipeline.Watch` / `ProcessInbox` |
| `initialization-flow` (`startTime`, `currentFilename`, `timestamp`, `newFilename`, `errorsFilename`) | `Initialization()` → `RunVars`; timestamp format `yyyyMMdd'T'HHmmssfff` | `initialization()` → `runVars`; `20060102T150405.000` with the dot removed |
| `<ee:transform>` "CSV to Java": `payload as Iterator` (CSV with header) | `CsvRows.Read(TextReader)` → lazy `IEnumerable<Dictionary<string,string>>` | `contacts.ReadRows(io.Reader)` → `iter.Seq2[Row, error]` |
| `<batch:job maxFailedRecords="-1" blockSize=...>` | `BatchJob { MaxFailedRecords = -1, BlockSize }` — a small generic engine (`Batch/BatchJob.cs`) | `batch.Job` (`internal/batch/engine.go`) |
| `<batch:step name="main-processing-step">` (default `NO_FAILURES`) | `BatchStep { Accept = NoFailures }` | `batch.Step{Accept: NoFailures}` |
| `<validation:is-email email="#[payload.email]" message="Missing or invalid email">` (per-record processor) | `EmailValidation.ValidateEmail(row)` throws `InvalidEmailException("Missing or invalid email")` | `contacts.ValidateEmail(row)` returns `ErrInvalidEmail` |
| `<batch:aggregator size=${...}>` + "CSV to SQL" transform + `<db:bulk-insert>` | `Aggregate`: `ContactMapper.FromRow` for each record of the block, then `IContactRepository.BulkInsertAsync` (one transaction, one insert per row) | `contacts.FromRow` + `Repository.BulkInsert` |
| DataWeave `as Number` / `as Boolean` / `as Date` | `Coerce.ToInt/ToBool/ToDate` (`ToDate` also on the three timestamp columns, deviation 4); a failure throws `CoercionException` with DataWeave's message (`Cannot coerce String (abc) to Number`) and the field as a property | `parseInt/parseBool/parseDate` |
| `<batch:step acceptPolicy="ONLY_FAILURES">` | `Accept = OnlyFailures` | `Accept: OnlyFailures` |
| "Create Error Record": `{Error: Batch::getFirstException().message, Record: write(payload,"application/csv",{header:false,lineSeparator:""})}` | `ErrorRecord(r.Error.Message, CsvRows.ToLine(row, Contact.Columns))` | `ErrorRecord{Error: r.Err.Error(), Record: contacts.RowToCSVLine(row)}` |
| `<batch:aggregator streaming="true">` + `<sftp:write path=processed/${errorsFilename}>` (JSON) | streaming aggregator (`AggregatorSize = 0`) writing `<data>/processed/<stem>.<ts>.errors.json` | same |
| `<batch:on-complete>` → log statistics, `send-email-report-flow` | `OnComplete` → `SendReportAsync`; a failure there sets `failedOnCompletePhase` (deviation 3) | `OnComplete` → `sendReport` |
| `send-email-report-flow`: "Extract Key Statistics" DataWeave | `BatchReport.KeyStatistics(filename, elapsed, stats)` → 5 label/value rows | `report.KeyStatistics` |
| `as String {format: "mm:ss.SSS"}` / `as String {format: "#,###"}` | `FormatMmSs`, `GroupThousands` | `formatMMSS`, `groupThousands` |
| `<parse-template location="contacts-batch-report-email.template">` | `BatchReport.Render(items, now)` — the same HTML with the two DataWeave expressions evaluated | `report.Render` (`text/template`; values verbatim, no HTML escaping, as parse-template) |
| `<email:send>` | `IReportSink.SendAsync(subject, html)` | `report.Sink.Send` |
| flow-level `<error-handler><on-error-propagate type="ANY">` → log + `<sftp:move targetPath=failed/>` | `catch` in `ProcessFileAsync`: log, `File.Move` to `<data>/failed/`, rethrow `PipelineException` | error path in `ProcessFile`: log, `os.Rename` to `failed/`, return error |
| TRACE loggers | `ILogger.LogDebug` | `slog.Debug` |

## `contacts-api-impl.xml`

The four DataWeave transforms named below by their `doc:name` were `<ee:transform>` blocks when the
ports were generated; they are now `<set-payload value="#[%dw 2.0 …]">` with the identical scripts, so
that the app runs on the community Kernel (`mule/contacts-api/README.md`, *Modification*). At the same
time the 400 handler's `VALIDATION:INVALID_STRING, VALIDATION:NOT_A_NUMBER` — error types the
Validation module does not define, so the original never packaged — became `VALIDATION:BLANK_STRING,
VALIDATION:INVALID_NUMBER`, the types the two validators raise; and the bare `every` in the two
`validation:is-true` expressions (not a core DataWeave function) became `dw::core::Arrays::every(…)`.
No row of this table is affected by any of these: the rows describe the intent (validation → 400,
each contact checked before any upstream call), which is what the ports implement.

| Mule | C# | Go |
|---|---|---|
| `<http:listener path="/contacts" allowedMethods="GET">` | `app.MapGet("/contacts", ...)` | `mux.HandleFunc("GET /contacts", ...)` |
| `<validation:is-not-blank-string>` / `<validation:is-number minValue=1 maxValue=500>` | inline checks → `400 {"error": ...}` with the same messages | same |
| `<db:select>` with `:city`, `:limit` input parameters | `QueryByCityAsync(city, limit)` (`LOWER(mailing_city) = LOWER(@city) ... LIMIT @limit`) | `QueryByCity` |
| "Rows to JSON" DataWeave | `contactsByCity` / `contactView` DTOs, `snake_case` JSON names | structs with `json:"..."` tags |
| `<http:listener path="/contacts/normalize" allowedMethods="POST">` | `app.MapPost` | `"POST /contacts/normalize"` |
| `<validation:is-true>` ×2 + `<validation:all>` | same messages, checked **before** any upstream call | same |
| `<foreach collection="#[vars.contacts]">` | `foreach` | `for range` |
| `<try>` + `<http:request method="GET" path="/{country}/{postalCode}">` | `IZippoClient.LookupAsync(country, postalCode)`; country lower-cased in the path | `zippo.Client.Lookup` |
| "Zippopotam to result" (`places[0]`, `latitude as Number`) | `ZippoWire.ToPlace()` | `zippo.FromWire` |
| `<on-error-continue type="HTTP:NOT_FOUND">` → `status: "not_found", place: null` | `PostalCodeNotFoundException` → result with `Status = "not_found"` | `zippo.ErrNotFound` |
| `<on-error-propagate type="HTTP:CONNECTIVITY, HTTP:TIMEOUT, HTTP:5xx">` → 502 | `UpstreamException` → `502 {"error": error.description}`; the `Upstream error:` prefix is in the log only, as in the XML `<logger>` | `*zippo.UpstreamError` → 502 |
| `EXPRESSION` from `latitude as Number` in "Zippopotam to result" → 400 | `CoercionException` → 400 | same |
| `<on-error-propagate type="DB:CONNECTIVITY, DB:QUERY_EXECUTION">` → 503 | repository exception → 503 | same |
| `<http:error-response>` body `{error: error.description}` | same shape | same |

## Deviations (deliberate)

1. **SFTP → local directory.** The ports read from `<data>/inbox` and write to `<data>/processed`,
   `<data>/failed`. A transport swap, not a logic change; adding real SFTP is one library
   (`SSH.NET` / `pkg/sftp`) behind the same three operations (list, move+rename, write). The showcase
   keeps it local so it runs with no servers. The listener has no file matcher, so Mule would pick up
   every file in `new/` (and fail a non-CSV one into `failed/`); the ports only pick up `*.csv` and
   leave other files in the inbox untouched.
2. **E-mail → file.** The report is rendered from the same template but written to `<data>/reports/`
   (or stdout in one-shot mode). SMTP is one call with `MailKit` / `net/smtp` behind `IReportSink` /
   `report.Sink`.
3. **Synchronous batch.** Mule runs `batch:job` asynchronously: the flow logs "Batch job successfully
   started", ends, and the listener moves the source file to `processed/` while the job is still
   running. The ports run the job inline and move the file *after* it completes. Same outputs, simpler
   to reason about, and the timestamp on the renamed file is still the run's start time. One
   consequence kept faithful: `<batch:on-complete>` runs after the flow has ended in Mule, so a failing
   report (`send-email-report-flow`) is logged and never reaches the flow's `on-error-propagate`; the
   file stays in `processed/`, the rows stay committed.
4. **`as Date` on timestamps.** The original coerces `created_date`, `last_modified_date` and
   `system_mod_stamp` with `as Date`, into `TIMESTAMP WITH TIME ZONE` columns, so a value like
   `2025-05-01T11:39:15.257Z` loses its time part and zone: only the calendar date, as written,
   survives. The ports do the same (`Coerce.ToDate` / `parseDate` on all three columns; golden
   `expected/first-record.json` has `"2025-05-01"`). What lands in the column is midnight of that date:
   SQLite stores the `YYYY-MM-DD` text, Postgres midnight **UTC**, bound explicitly — a JDBC
   `LocalDate` → `timestamptz` cast would instead use the session time zone, which is the one place a
   real Mule run against a non-UTC Postgres could differ. A failed coercion reads
   `Cannot coerce String (x) to Date`, DataWeave's wording. Both ports also accept
   `TIMESTAMP_PRECISION=instant` to keep the full timestamp instead (UTC, milliseconds:
   `2025-05-01T11:39:15.257Z`; a plain date in the input stays a date) for callers who value the data
   over parity with the original; `date` is the default and what the goldens and the parity run use.
5. **Error-file CSV quoting.** `Record` in the errors file is the row re-serialised as one CSV line
   with minimal quoting. DataWeave's CSV writer quotes in the same situations (separator, quote,
   newline in a value); exotic cases (leading/trailing spaces are *not* quoted by either) were not
   verified against a live run. Second place a diff could differ.
6. **Empty text cells stay `""`.** DataWeave passes empty CSV cells through as empty strings and the
   original inserts them as such (not `NULL`). The ports do the same, so the rows compare equal.
7. **SQLite typing.** SQLite has no `BOOLEAN`/`DATE`/`TIMESTAMPTZ`; the shared schema stores booleans
   as `0/1` and both the date and the timestamp columns as `YYYY-MM-DD` text (the latter carry dates
   only, deviation 4). Both ports bind exactly those representations so the rows are byte-identical
   across languages. Postgres keeps native types.
8. **Statistics object.** The `batchJobInstanceId`, `totalRecords`, `loadedRecords`,
   `processedRecords`, `successfulRecords`, `failedRecords`, `elapsedTimeInMillis`,
   `failedOnInputPhase` and `failedOnCompletePhase` fields are reproduced; the `*PhaseException`
   fields are not.
9. **Non-numeric `limit` → 400.** `(attributes.queryParams.limit default '50') as Number` on `abc`
   raises an `EXPRESSION` error in the `<set-variable>`, which the flow's 400 handler does not list,
   so Mule answers 500 (and before the `city` check). The ports answer 400 with the range message.
   *Observed* on the community Kernel 4.12.0 (2026-09-25): `limit=abc` → 500 with
   `Cannot coerce String (abc) to Number` in the body.
10. **`as Number` is integer-only.** DataWeave's `as Number` on `active_tracker_count` accepts
    decimals, exponents and values beyond 32 bits; the ports parse a 32-bit integer (the column is
    `INTEGER`) and fail the block on anything else.
11. **Empty input file → `failed/`.** DataWeave yields no records for an empty CSV, so Mule would
    run a zero-record job and move the file to `processed/`. The ports treat a file without a header
    row as unreadable: it goes to `failed/` and no report is sent (this is what the test prompt
    specifies).

## What ports cleanly, what needs a human

Ports cleanly: HTTP listener/requester, Database select/insert/bulk, DataWeave mappings and coercions,
`choice`/`foreach`/`try`/error handlers, Validation, batch steps/aggregators, parse-template, properties.

Needs attention in a real project (not exercised here): Object Store and watermarks (semantics depend on
persistence and clustering), connector-specific behaviours (Salesforce bulk limits, SAP idocs), XA
transactions, MUnit tests (their mocks encode assumptions worth reading before porting), secure
properties, custom Java classes, and anything that relies on Mule's streaming / repeatable-stream
memory model for very large payloads.
