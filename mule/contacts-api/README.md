# contacts-api (Mule 4)

Two flows on the `contacts` table that `batch-contacts-csv-to-db` fills:

- `GET /contacts?city=<name>&limit=<1..500>` — HTTP listener → Validation → `db:select` → DataWeave → JSON.
- `POST /contacts/normalize` — HTTP listener → Validation → `foreach` contact: `http:request`
  `GET https://api.zippopotam.us/{country}/{postal-code}` inside a `try` (404 → `not_found`) → DataWeave → JSON.

Error handling maps Validation errors to 400, DB errors to 503 and upstream connectivity/5xx to 502,
with a `{"error": "..."}` body. Contract: `../../testdata/openapi.yaml`.

Profiles: `-Denv=sqlite` (default) or `-Denv=postgres`.

## Modification — this is not the file the ports were generated from

`src/main/mule/contacts-api-impl.xml` was changed on 2026-09-25, **after** prompts 1–3 of `../../PROMPTS.md`
had produced the C# and Go ports from it:

| | Before (what the ports saw) | Now |
|---|---|---|
| "Rows to JSON", "Zippopotam to result", "Not found result", "Results to JSON" | `<ee:transform>` (Transform Message) | `<set-payload value="#[%dw 2.0 …]">` |
| `get-contacts-by-city-flow`, 400 handler | `type="VALIDATION:INVALID_STRING, VALIDATION:INVALID_NUMBER, VALIDATION:NOT_A_NUMBER"` | `type="VALIDATION:BLANK_STRING, VALIDATION:INVALID_NUMBER"` |
| `pom.xml`: `mule-http-connector` | `1.10.3` | `1.12.1` |
| `pom.xml`: `mule-maven-plugin` | `4.4.0` | `4.10.1` |
| `normalize-contacts-flow`, the two `validation:is-true` inside `validation:all` | `#[vars.contacts every ((c) -> …)]` | `#[dw::core::Arrays::every(vars.contacts, (c) -> …)]` |

**Change 1.** The DataWeave scripts inside are unchanged, byte for byte. The reason: `<ee:transform>`
lives in the `http://www.mulesoft.org/schema/mule/ee/core` namespace, which only the Enterprise
Edition runtime provides — the community Mule Kernel 4.12.0 ships the DataWeave *service* but no
provider for that namespace, so the original file did not deploy on it (checked by grepping every jar
of the distribution). `set-payload` is a core component and runs the same script through the same
DataWeave service, which makes the app runnable on the free Kernel:
`docker compose -f ../../docker/docker-compose.yml --profile mule-ce up`.

**Change 2 — a bug in the original.** `VALIDATION:INVALID_STRING` and `VALIDATION:NOT_A_NUMBER` are
not error types of the Validation module (2.0.11; the enum in its jar defines `BLANK_STRING`,
`INVALID_NUMBER`, `INVALID_BOOLEAN`, `MULTIPLE`, … and none of those two). The `mule-maven-plugin`
therefore refuses to package the app (`Could not find error 'VALIDATION:INVALID_STRING'`), on EE as much
as on CE: the file the ports were generated from would not have deployed anywhere. The handler now
names the types `is-not-blank-string` and `is-number` actually raise. The intent (those two validations
→ 400) is what the ports implemented, and `../../MAPPING.md` describes it in those terms, so the ports
are unaffected; the finding is recorded here because a reader comparing the XML with the ports
deserves to know the input was never executable as published.

**Change 3 — build versions.** HTTP connector 1.10.3 fails to initialise its listener on runtime
4.12.0 (`NoClassDefFoundError: org.mule.runtime.core.api.util.ClassUtils`, a class the runtime no longer
has); 1.12.1 is the current release and is built for it. Packaging 1.12.1 in turn needs a
`mule-maven-plugin` newer than 4.4.0 (that one does not know the connector's declared `JAVA_25`
support), hence 4.10.1. Dependency versions only; no flow semantics are involved. The DB connector
(1.16.4) and the Validation module (2.0.11) were already the latest.

**Change 4 — a second bug in the original.** `every` is not a core DataWeave function; it lives in
`dw::core::Arrays` and needs an import or a qualified name. Bare `vars.contacts every (…)` made the
runtime reject *every* `POST /contacts/normalize` with `Unable to resolve reference of: every` (as a
400, since the flow's 400 handler lists `EXPRESSION`). The two expressions now call
`dw::core::Arrays::every(vars.contacts, …)` with the lambdas unchanged. As with change 2, the ports
implemented the intent (each contact needs both fields, checked before any upstream call), so they are
unaffected; the golden `testdata/api/normalize-response.json` was, again, right by reading.

Nothing else was changed: `global.xml`, the property files, `mule-artifact.json` and the rest of
`pom.xml` are as the ports saw them.

Found by running it: changes 2–4 are what a first deployment of the published file surfaced. The
sequence is recorded in `../../PARITY.md`. The previous wording of this README ("no EE-only features") was wrong and is
withdrawn. `../../MAPPING.md` refers to the four transforms by their `doc:name`, which did not change.

`batch-contacts-csv-to-db` was **not** modified in this way: its Batch module is EE-only regardless,
and the fork promises its XML is untouched (`../batch-contacts-csv-to-db/FORK-NOTES.md`).
