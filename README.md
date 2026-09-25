# mule-to-code

**Two Mule 4 applications, ported to plain C# and Go services with a handful of Claude prompts.**

This repository is the proof behind a short post: the flows you run on the Mule runtime are just code.
Given the Mule XML, a model can turn them into an ordinary service in whatever language you use, and
the runtime, the connectors and the licence that come with them can go.

It is a temporary showcase. It may be taken down; the post is the durable reference.

## What is in here

| Path | What | Origin |
|---|---|---|
| `mule/batch-contacts-csv-to-db/` | A real Mule 4 batch application: SFTP → CSV → **Batch Job** (validate, aggregate, bulk insert into PostgreSQL) → failed-records file → e-mail report | Fork of [abelisle-mulesoft/mule-4-batch-job-examples](https://github.com/abelisle-mulesoft/mule-4-batch-job-examples) (Apache 2.0). Nothing in its `src/main/mule/*.xml` was changed; see `FORK-NOTES.md` |
| `mule/contacts-api/` | A small Mule 4 API on the same table: `GET /contacts?city=` (**query** pattern) and `POST /contacts/normalize` (**translate / forward** pattern, calls the free [zippopotam.us](https://api.zippopotam.us) postal-code service) | Written for this repository. **Modified after the ports were generated**: `<ee:transform>` → `<set-payload>` (same DataWeave) so it runs on the community Mule Kernel; two Validation error types that do not exist were corrected — the original never packaged; the HTTP connector and Maven plugin were bumped to versions that work on runtime 4.12; and an unqualified DataWeave `every` that broke every normalize request was fixed. Full statement in its `README.md` |
| `testdata/` | The 100-record sample CSV, a variant with two invalid e-mails, golden outputs (batch statistics, errors file, API responses), stubbed upstream answers and the OpenAPI contract. **Shared by both ports** – they must pass the same fixtures | Sample data from upstream; the rest written here |
| `docker/` | PostgreSQL (the profile the original was written against), the same schema for SQLite, an optional SFTP container for the *original* batch app, and the community Mule Kernel running `contacts-api` (no MuleSoft account needed) | Written here |
| `PARITY.md` | How to compare the ports against the Mule runtime, and what to do when they disagree | Written here |
| `dotnet/` | The C# port and its tests, as produced by prompts 1 and 3 of `PROMPTS.md`. Layout, run modes and every setting: `dotnet/README.md` | Generated |
| `go/` | The Go port, as produced by prompt 2 of `PROMPTS.md` (tests from prompt 3). Layout, run modes and every setting: `go/README.md` | Generated |
| `MAPPING.md` | Mule element → target code, and the deliberate deviations | Written here |
| `PROMPTS.md` | The literal prompts, in order | Written here |

The two Mule apps together exercise: SFTP listener + scheduler, DataWeave (CSV parsing, mapping with
type coercion, JSON output, string formatting), the Batch module (steps, accept policies, aggregators,
max failed records, on-complete statistics), the Validation module, the Database connector (bulk insert,
select with parameters, generic JDBC), parse-template, the Email connector, HTTP listener and requester,
`foreach`, `try`/`on-error-continue`, flow-level error handlers with HTTP status mapping, and
environment-specific properties files.

Worth noting: the Batch module is a **Mule Enterprise Edition** feature. The original batch app cannot
run on the community runtime at all — you need an EE trial or CloudHub. So is the Transform Message
component (`<ee:transform>`): the community Kernel ships DataWeave but not the `ee` namespace, which is
why `contacts-api` was changed to `<set-payload>` (see above) — the only way to run *any* of this on a
runtime that needs no account. The ports need `dotnet` or `go`.

## Scope

This repository shows the basic mechanic only: source XML in, prompts, code out.

Real conversion projects (many flows, shared domains, custom Java, MUnit, CI) benefit from a proper
setup — agent instructions, hooks, orchestration, review rules — and that setup is intentionally left
to the reader. It depends on your codebase and tooling, and including one here would make this look
like a template when it is an illustration.

What *is* complete are the tests. Both ports are expected to pass the same fixture-driven test suite
(see below); the project scaffolding around a conversion is not, and that is a deliberate line.

## How to reproduce

1. Open the repository in VS Code with Claude Code.
2. Run the prompts in `PROMPTS.md`, in order. Each one is self-contained; the first two produce the
   ports, the third produces the tests, the last one checks `MAPPING.md` against what was generated.
3. Run the tests (below) and compare the output rows of the three implementations against the same
   SQLite file if you want to see them agree byte for byte.

## Run

Two database profiles exist everywhere (the Mule apps, the C# port, the Go port), selected by
`DB_PROFILE` (ports) or `-Denv=` (Mule):

- **`sqlite`** (default) — zero setup. The ports create `data/contacts.db` and apply
  `docker/sqlite/contacts.sqlite.sql` on start. Used by the quick start and by the unit /
  end-to-end tests.
- **`postgres`** — the fidelity profile, what the original app was written against.
  `docker compose -f docker/docker-compose.yml up -d db`, then `DB_PROFILE=postgres`.
  Integration tests start their own Postgres through Testcontainers and are skipped without Docker.

Prerequisites: .NET 8 SDK and/or Go 1.25+ (the Go port itself needs 1.23; its Testcontainers test dependency raises the floor); Docker only for the postgres profile.

### Quick start (sqlite)

```bash
# Go
cd go && go mod tidy
mkdir -p data/inbox && cp ../testdata/contact-data-100-with-errors.csv data/inbox/
go run ./cmd/batch -once            # processes the inbox, prints the report; 98 rows land in data/contacts.db,
                                    # the file moves to data/processed/ next to <name>.<ts>.errors.json
go run ./cmd/api &                  # http://localhost:8082
curl 'localhost:8082/contacts?city=Saint%20Louis'
curl -X POST localhost:8082/contacts/normalize -H 'content-type: application/json' -d @../testdata/api/normalize-request.json

# C#
cd dotnet
mkdir -p data/inbox && cp ../testdata/contact-data-100-with-errors.csv data/inbox/
dotnet run --project src/Contacts.Batch -- --once
dotnet run --project src/Contacts.Api  # http://localhost:8082
```

Without `-once` / `--once` the batch program keeps polling `data/inbox/` like the SFTP listener did.

### Postgres profile

```bash
docker compose -f docker/docker-compose.yml up -d db
DB_PROFILE=postgres go run ./cmd/batch -once                     # Go
DB_PROFILE=postgres dotnet run --project src/Contacts.Batch -- --once   # C#
```

Connection: `postgres://contacts:contacts@localhost:5432/contacts`.

### Running the Mule apps

**`contacts-api` on the community Mule Kernel — no account, Docker only.** `docker/mule-ce/Dockerfile`
packages the app with the `mule-maven-plugin` and runs it on `org.mule.distributions:mule-standalone`
4.12.0 (the Kernel, from the public MuleSoft repository). Needs the postgres profile; the image uses
host networking so the app's own `mule-props-postgres.yaml` (`localhost:5432`) applies unchanged.

Prerequisites: Docker with Compose v2, `curl`, `jq` (for the diffs in `PARITY.md`), and internet access
during the first build — it pulls the Maven and JRE base images from Docker Hub, the connectors from
`repository.mulesoft.org` / `maven.anypoint.mulesoft.com`, and the 135 MB Kernel tarball from
`repository.mulesoft.org`; about 5 minutes and 1 GB of disk. Nothing from MuleSoft is stored in this
repository: the Dockerfile only says where to fetch it. The Kernel is CPAL 1.0 (its `LICENSE.txt` is
inside the image at `/opt/mule/LICENSE.txt`); the built image is for local use, do not publish it
to a registry.

```bash
docker compose -f docker/docker-compose.yml --profile mule-ce up -d --build   # db + mule-api; first build ≈ 5 min
docker/load-csv.sh testdata/contact-data-100.csv                             # seed the table (COPY, no batch app involved)
until curl -sf 'localhost:8082/contacts?city=x' >/dev/null; do sleep 3; done   # ready in ≈ 20–60 s (the deploy log is inside the container, not on stdout)
curl 'localhost:8082/contacts?city=Saint%20Louis'
curl -X POST localhost:8082/contacts/normalize -H 'content-type: application/json' -d @testdata/api/normalize-request.json
docker compose -f docker/docker-compose.yml --profile mule-ce down
```

The Mule API and both ports default to port 8082, so run one at a time. `PARITY.md` turns this into a
comparison against the ports.

**`batch-contacts-csv-to-db` — Anypoint Studio only.** The Batch module is EE, so this needs Studio
(30-day trial, MuleSoft account). Import it as a Maven project, set `-M-Denv=postgres` or
`-M-Denv=sqlite` in the run configuration (`env` defaults to `dev`, which has no property file), and:

- start the SFTP container (`docker compose -f docker/docker-compose.yml --profile mule up -d`), make its
  bind mounts writable for the container user (`chmod -R a+rwX docker/sftp`, uid 1001) and drop a CSV into
  `docker/sftp/new/`; the statistics are logged before the e-mail is sent, so empty SMTP credentials only
  cost you the report;
- for the sqlite profile, create the file once: `sqlite3 data/contacts.db < docker/sqlite/contacts.sqlite.sql`
  and put its **absolute** path in the properties file.

The upstream `documentation/` folder has the author's own walkthrough of the batch app.

## Tests

Both ports carry the same layers, driven by `testdata/`:

- **Unit / golden** — each DataWeave transform is a function with a golden-file test
  (`expected/first-record.json`, `expected/errors-100-with-errors.json`, the API JSON files);
  the e-mail validation; the report formatting (`mm:ss.SSS`, `#,###`).
- **Batch semantics** — the engine that reproduces `batch:job`: step order, `NO_FAILURES` /
  `ONLY_FAILURES`, aggregator block size, a failing aggregator failing its whole block,
  `maxFailedRecords`, on-complete statistics.
- **End-to-end (sqlite)** — the 100-record file (100 inserted, report says 100/0), the with-errors
  file (98/2, errors file matches the golden), a coercion error failing an aggregator block, a broken
  file landing in `failed/`, file renaming with the `uuuuMMdd'T'HHmmssSSS` timestamp.
- **Integration (postgres)** — the same repository contract against a Testcontainers Postgres; opt-in
  with `RUN_POSTGRES=1`, skipped otherwise or when Docker is absent.
- **API** — in-process test host, upstream call stubbed with `testdata/api/zippopotam-*.json`; golden
  responses; validation errors → 400; upstream outage → 502; the required fields of `openapi.yaml`.

```bash
cd go && go test ./...                 # RUN_POSTGRES=1 to include the container test (opt-in)
cd dotnet && dotnet test                 # RUN_POSTGRES=1 to include the container test (opt-in)
```

An honest caveat: the golden outputs were derived by reading the DataWeave, not by running Mule.
The API goldens can be checked against the real runtime with the Kernel container above (`PARITY.md`
has the procedure and the rule for disagreements). The batch goldens cannot without EE: if you have a
Studio trial around, running the original once and diffing its rows against the ports' would make them
stronger. `MAPPING.md` lists the two places where that could matter.

## Attribution and license

`mule/batch-contacts-csv-to-db` and `testdata/contact-data-100.csv` are © Alan Belisle, Apache License 2.0
(kept as `mule/batch-contacts-csv-to-db/LICENSE-upstream`). Everything else is Apache License 2.0 as well
(`LICENSE`, `NOTICE`). Mule, MuleSoft, Anypoint and DataWeave are trademarks of Salesforce, Inc.; this
repository is not affiliated with them.
