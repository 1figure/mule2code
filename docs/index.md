---
title: mule2code – MuleSoft Mule 4 to Go and C#
description: Two Mule 4 apps ported to plain Go and C# with Claude, with DataWeave ports, shared test fixtures and a parity check against the Mule runtime.
---

# Migrating MuleSoft Mule 4 apps to Go and C#

Two Mule 4 applications, a batch job and an API, ported to plain Go and C#
in an afternoon: DataWeave transforms, Batch module, validation, database,
HTTP and error handling, with shared test fixtures and a parity procedure
against the Mule runtime.

**Code, prompts and every number: [github.com/1figure/mule2code](https://github.com/1figure/mule2code)**

## The claim

Converting Mule flows to ordinary code has become cheap. Given the XML, a
model turns them into a plain service in whatever language you use, in an
afternoon, and the runtime, the connectors and the licence that come with
them can go. What you get back is what every other codebase already has: a
compiler, a linter, a reviewer, coverage, dependency bots and a CVE scanner —
none of which ever saw the XML.

## What went in

- A real Mule 4 **batch job** from GitHub (Apache 2.0): SFTP listener with a
  scheduler → CSV → Batch Job (validate each record, aggregate, bulk-insert
  into PostgreSQL) → failed-records file → HTML e-mail report from a
  `parse-template`. Its XML is untouched.
- A small Mule 4 **API** on the same table, written for this exercise:
  `GET /contacts?city=` (query pattern) and `POST /contacts/normalize`
  (translate/forward pattern: `foreach` contact, `http:request` to a public
  postal-code service inside a `try`, 404 → `not_found`).
- Between them: DataWeave (CSV parsing, mapping with type coercion, JSON
  output, `mm:ss.SSS` and `#,###` formatting), the Batch module (steps, accept
  policies, aggregators, `maxFailedRecords`, on-complete statistics), the
  Validation module, the Database connector (bulk insert, select with
  parameters, generic JDBC), HTTP listener and requester, the Email connector,
  flow-level error handlers with HTTP status mapping, and per-environment
  property files. **422 lines of XML** in total.
- `MAPPING.md`: every element → where it lands in each port, plus eleven
  numbered deliberate deviations (SFTP → local directory, e-mail → file sink,
  synchronous batch, SQLite typing, and so on). `PROMPTS.md`: the five literal
  prompts. `testdata/`: the 100-record sample, a variant with two invalid
  e-mails, golden statistics, errors file and API responses, recorded
  upstream answers, an OpenAPI contract — one set of fixtures for both ports.

## What came out

Each port from one prompt, its tests from a second, in a fresh session each.

|  | Go port | C# port |
|---|---|---|
| Stack | standard library + `modernc.org/sqlite` + `pgx` | .NET 8 minimal API + console host, Dapper, Npgsql, `Microsoft.Data.Sqlite` |
| Source | 2,822 lines, 34 files | 2,807 lines, 64 files |
| Tests | 3,125 lines; 63 test functions, 149 cases, 12 packages | 2,643 lines; 233 xunit tests (12 opt-in PostgreSQL) |
| Statement/line coverage | 90.5 % | 92.4 % (branch 85.5 %) |
| Cyclomatic complexity | avg 5.1, max 19 (the batch engine's `Run`) | avg 3.4, max 12 (the CSV line parser) |
| Checks the prompt demanded | `gofmt`, `go vet` clean | build with warnings as errors, `dotnet format` clean |

Both ports have the same shape: a generic **batch engine** reproducing
`batch:job` semantics (steps run in order, `NO_FAILURES` / `ONLY_FAILURES`,
per-record processors then an aggregator with a block size, a failing
aggregator fails its whole block, `maxFailedRecords`, statistics object);
every **DataWeave transform as its own function** with a golden-file test;
the upstream **HTML e-mail template rendered unchanged**; validation and error
messages **copied verbatim** from the XML; configuration through environment
variables that map one-to-one onto the Mule property files; a batch host that
polls an inbox like the SFTP listener did (or runs once, or one file); both
database profiles, with SQLite rows byte-identical across the two languages.
Coverage and complexity reports are checked in under `reports/` in each port,
with the script that regenerates them.

The honest ratio: 422 lines of XML became about 2,800 lines per port. The XML
is dense because the runtime does the work — the batch engine, the CSV
reader, the configuration loader are now yours to read, test and change.

## Parity against the real runtime, where possible

The Batch module is Enterprise-only, so that app cannot run anywhere without
a licence; batch outputs were verified port against port. The API ran on the
free community Kernel 4.12.0 in Docker, with no MuleSoft account — after
three fixes to its XML and one to its pom:

1. A component that turned out to be EE-only: Transform Message. The `ee:` in
   its namespace is literal; the Kernel ships DataWeave but nothing that
   registers `ee:*`. Replaced by `set-payload` with the identical script.
2. Two Validation error types that do not exist (`INVALID_STRING`,
   `NOT_A_NUMBER`). The build tool refused to package the app — so the file
   the ports were generated from would not have deployed on Enterprise either.
3. One unqualified DataWeave function (`every`, which lives in
   `dw::core::Arrays`). Every normalize request failed.
4. A connector version the current runtime cannot load (HTTP connector
   1.10.3 on runtime 4.12), which dragged the Maven plugin along.

The ports had implemented the intent correctly without ever seeing any of it
fail. Nobody had ever run the input. Two of those four are compile errors in
typed code; the fourth is a dependabot pull request that never arrived,
because no dependency bot reads Anypoint Exchange.

Against the running Kernel, with the table seeded by `COPY`: both API goldens
byte-identical after JSON normalisation; each port then ran on the same
database — `GET` with and without `limit`, lower-case city, `POST` with the
live upstream from both sides, and seven 400 cases including the two-message
body of `validation:all` (joined with a newline in Mule too) — all
byte-identical to Mule's. The one difference, a non-numeric `limit` answering
500 in Mule and 400 in the ports, was already deviation 9 in `MAPPING.md`;
observed, not inferred. One golden had drifted: the upstream postal-code
service had changed four coordinates since the fixtures were recorded, which
the live run caught. The rule for such disagreements is written down in
`PARITY.md`: Mule beats the golden, the golden beats the port, listed
deviations are expected, new ones get numbered.

## Footprint, measured

|  | Mule Kernel running the API | Go port |
|---|---|---|
| Artifact | 135 MB distribution; 774 MB image with JRE 17 and the app | 20 MB static binary |
| Memory | 1 GB default heap (`wrapper.conf`) | tens of MB |
| Cold start to first response | about 18 s (20–60 s observed) | under a second |
| Build loop | about 3 min per image (Maven resolving MuleSoft repositories) | seconds |

Request latency was not benchmarked. Compute your own vCore.

## What it does not claim

- Batch outputs are verified port against port, not against Mule; the places
  a live Enterprise run could differ are named in `MAPPING.md`.
- Premium connectors, Object Store and clustering semantics, MUnit and the
  visual canvas are not exercised. Cheap conversion still means writing the
  batch engine yourself — and proving equivalence, which was most of the work.
- Corrections during generation are not recorded; "if it doesn't compile,
  say so in the same session" is part of the mechanic, and the real-project
  setup around it (agent instructions, hooks, review rules) is deliberately
  left to the reader.

## Reproduce it

1. Clone the repository and open it in VS Code with Claude Code.
2. Run the prompts in `PROMPTS.md`, one per fresh session.
3. `go test ./...` and `dotnet test`; then `PARITY.md` for the runtime check
   (Docker only, no account).

Everything in the repository is Apache 2.0; the upstream batch example is
© Alan Belisle. Mule, MuleSoft, Anypoint and DataWeave are trademarks of
Salesforce, Inc.; this project is not affiliated with them.
