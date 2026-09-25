# Prompts

The literal prompts used to produce `dotnet/` and `go/`, in order. Run them in Claude Code from the
repository root, one at a time, in a fresh session each. No agent files, hooks, or custom instructions
were used; the model only saw this repository.

Corrections and iteration are not recorded here: they belong to the "real project" setup that this
showcase leaves to the reader (see `README.md`, *Scope*). If the first output does not compile or a
test fails, say so in the same session and let it fix it; that is part of the mechanic, not a
deviation from it.

---

## 1 — C# port

```
Read mule/batch-contacts-csv-to-db/src/main/mule/*.xml and mule/contacts-api/src/main/mule/*.xml,
then MAPPING.md and testdata/openapi.yaml.

Port both Mule applications to C# (.NET 8) under dotnet/, following the layout described in
dotnet/README.md and the element-by-element mapping in MAPPING.md, including its "Deviations"
section (local directory instead of SFTP, file sink instead of e-mail, synchronous batch).

Constraints:
- Plain ASP.NET Core minimal API for the HTTP flows and a console host for the batch job. No
  integration framework, no "connector" abstraction layer; Mule connectors become ordinary
  libraries (Microsoft.Data.Sqlite, Npgsql, Dapper, HttpClient).
- Reproduce the batch:job semantics faithfully in a small generic engine: steps run in order and
  finish for all records before the next step starts; accept policies NO_FAILURES / ONLY_FAILURES /
  ALL; per-record processors, then an aggregator with a block size (0 = all records); a failing
  aggregator fails every record of its block; maxFailedRecords (-1 = unlimited); on-complete
  receives the statistics object.
- Two database profiles selected by DB_PROFILE: sqlite (default, apply
  docker/sqlite/contacts.sqlite.sql on start) and postgres (docker/init/01-contacts.sql).
  Bind values exactly as MAPPING.md deviation 7 says.
- Keep every DataWeave transform as its own small function so it can be golden-tested.
- Reuse the upstream HTML e-mail template for the report.
- Use the exact validation / error messages from the XML; they end up in the errors file and in
  HTTP error bodies.
- Configuration through environment variables with the names listed in MAPPING.md.

Code quality:
- `dotnet build` with warnings as errors and `dotnet format --verify-no-changes` clean; nullable
  enabled; async all the way with CancellationToken passed through I/O boundaries; exceptions
  carry the original as InnerException.
- XML doc comments on public types and members, otherwise comment only what is not obvious
  from the code.
- Where a method implements a specific Mule element, a one-line comment naming it, e.g.
  // <batch:step name="main-processing-step"> — this is the traceability that MAPPING.md and
  the mapping-check prompt rely on.

Do not write tests yet. When done, list every file you created with one line each.
```

## 2 — Go port

```
Read mule/batch-contacts-csv-to-db/src/main/mule/*.xml and mule/contacts-api/src/main/mule/*.xml,
then MAPPING.md and testdata/openapi.yaml.

Port both Mule applications to Go (1.23+) under go/, following the layout in go/README.md and the
element-by-element mapping in MAPPING.md, including its "Deviations" section.

Constraints:
- Standard library for HTTP, CSV, JSON, templates and logging (log/slog). Dependencies limited to
  modernc.org/sqlite, github.com/jackc/pgx/v5 (stdlib driver) and, for tests later,
  testcontainers-go. No integration framework, no "connector" abstraction layer.
- Reproduce the batch:job semantics in a small generic package (same rules as in prompt 1:
  ordered steps, accept policies, per-record processors then aggregator with block size,
  block-level failure, maxFailedRecords, on-complete statistics).
- Two database profiles selected by DB_PROFILE (sqlite default / postgres), applying the schema
  files under docker/ on start, binding values exactly as MAPPING.md deviation 7 says so the rows
  are byte-identical to the C# port on SQLite.
- Each DataWeave transform is its own function; the report reuses the upstream HTML template;
  validation and error messages are copied verbatim from the XML.
- Environment variable names as in MAPPING.md.

Code quality:
- gofmt and `go vet ./...` clean; errors wrapped with %w; context.Context passed through I/O
  boundaries; no panics outside main.
- Doc comments on exported identifiers (Go convention), otherwise comment only what is not
  obvious from the code.
- Where a function implements a specific Mule element, a one-line comment naming it, e.g.
  // <batch:step name="main-processing-step"> — this is the traceability that MAPPING.md and
  the mapping-check prompt rely on.

Do not write tests yet. When done, list every file you created with one line each.
```

## 3 — Tests (run once per port)

```
Write the test suite for the <C# | Go> port under <dotnet/tests | go/>, driven entirely by the
shared fixtures in testdata/ (never copy them; reference them by path). Both ports must pass the
same fixtures, so do not invent expected values: use testdata/expected/*.json and testdata/api/*.json.

Layers:
1. Golden tests for every DataWeave transform: the first record of testdata/contact-data-100.csv
   against testdata/expected/first-record.json; the two "Record" lines of
   testdata/expected/errors-100-with-errors.json; the report formatting (mm:ss.SSS and #,###);
   the e-mail validation with valid and invalid samples.
2. Batch engine semantics with synthetic records: step order, NO_FAILURES vs ONLY_FAILURES,
   aggregator block size, a failing aggregator failing exactly its block, a record failing in a
   step's processors never reaching that step's aggregator, maxFailedRecords stopping the job
   and -1 not stopping it, statistics, an input-phase error, cancellation.
3. End-to-end on SQLite in a temp directory: contact-data-100.csv → 100 rows and statistics equal
   to testdata/expected/batch-report-100.json; contact-data-100-with-errors.csv → 98 rows,
   statistics equal to batch-report-100-with-errors.json, and an errors file equal to
   errors-100-with-errors.json written next to the processed file with the same timestamp;
   a coercion error failing one whole aggregator block; an empty file moved to failed/ with no
   report; the renamed file matching <stem>.<yyyyMMddTHHmmssSSS>.<ext>; non-CSV files ignored.
4. Repository contract against PostgreSQL via Testcontainers, opt-in: it runs only when
   RUN_POSTGRES=1 is set and Docker is available, and reports as skipped otherwise (never as
   failed). Same assertions as the SQLite repository test.
5. API tests with an in-process host and the upstream client stubbed from
   testdata/api/zippopotam-*.json: GET /contacts?city=Saint%20Louis equals
   testdata/api/get-contacts-saint-louis.json byte-for-byte after JSON normalisation;
   case-insensitive city; limit honoured; every 400 case from the validations in the XML;
   POST /contacts/normalize with testdata/api/normalize-request.json equals
   normalize-response.json; validation happens before any upstream call; upstream failure → 502;
   the required properties of testdata/openapi.yaml are present in the responses.

Run the tests and fix the port until they pass. Report the final test count and the line
coverage (`dotnet test --collect:"XPlat Code Coverage"` / `go test -cover ./...`), and name
any package or transform the fixtures leave uncovered.
```

## 4 — Mapping check

```
Compare dotnet/ and go/ against MAPPING.md. For every row of the tables, confirm where the element
landed in each port (file and symbol). For every numbered deviation, confirm the port behaves that
way. List anything that differs, and anything present in the ports that MAPPING.md does not
describe. Do not change code; produce the list only.
```

## 5 — (optional) Parity run

```
Using the sqlite profile, run the Go batch and the C# batch on a fresh copy of
testdata/contact-data-100-with-errors.csv each, into two separate database files, then dump
`SELECT * FROM contacts ORDER BY external_id` from both and diff them. Report the diff. Then diff
the two errors files. Report the diff.

For the API, follow PARITY.md: start the Mule application on the community Kernel
(docker compose --profile mule-ce), seed the table with docker/load-csv.sh, record its responses,
then run each port against the same database and diff. Apply the rule in PARITY.md if anything
disagrees, and report what you changed.
```
