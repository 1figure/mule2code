# Parity: the ports against the Mule runtime

What can be compared with a real Mule run, how, and the rule when they disagree. For the people (or
sessions) working on `dotnet/` and `go/`.

## What the reference is

| App | Real Mule run possible? | Reference for the ports |
|---|---|---|
| `contacts-api` | **Yes** — community Kernel in Docker, no account (`docker/mule-ce/`) | The running Mule app, then `testdata/api/*.json` |
| `batch-contacts-csv-to-db` | Only with EE (Anypoint Studio trial, MuleSoft account) | `testdata/expected/*.json`, derived by reading the DataWeave; port vs port (`PROMPTS.md` prompt 5) |

The API app was modified after the ports were generated (`<ee:transform>` → `<set-payload>`, same
DataWeave — `mule/contacts-api/README.md`). That changes nothing the ports implement; the reference
behaviour is the DataWeave, and the DataWeave is byte-identical.

## API: Mule first, then your port

Everything runs against the same Postgres (`docker/`), seeded with the same file by `COPY`, so the only
variable is the implementation answering on port 8082.

```bash
# 1. Mule
docker compose -f docker/docker-compose.yml --profile mule-ce up -d --build
docker/load-csv.sh testdata/contact-data-100.csv
until curl -sf 'localhost:8082/contacts?city=x' >/dev/null; do sleep 3; done   # ready in ≈ 20–60 s

norm() { jq -S . ; }
curl -s 'localhost:8082/contacts?city=Saint%20Louis'                                    | norm > /tmp/mule-get.json
curl -s 'localhost:8082/contacts?city=saint%20louis&limit=2'                            | norm > /tmp/mule-get-limit.json
curl -s -X POST localhost:8082/contacts/normalize -H 'content-type: application/json' \
     -d @testdata/api/normalize-request.json                                            | norm > /tmp/mule-post.json
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8082/contacts'                      # 400
curl -s -X POST localhost:8082/contacts/normalize -H 'content-type: application/json' \
     -d @testdata/api/normalize-request-invalid.json -w '\n%{http_code}\n'              # 400 + {"error": ...}

diff /tmp/mule-get.json  <(jq -S . testdata/api/get-contacts-saint-louis.json)
diff /tmp/mule-post.json <(jq -S . testdata/api/normalize-response.json)
docker compose -f docker/docker-compose.yml --profile mule-ce stop mule-api            # frees 8082

# 2. Your port, same database, same seed
DB_PROFILE=postgres go run ./cmd/api &                     # or: dotnet run --project src/Contacts.Api
# repeat the curls into /tmp/port-*.json, then:
diff /tmp/mule-get.json  /tmp/port-get.json
diff /tmp/mule-post.json /tmp/port-post.json
```

`POST /contacts/normalize` calls the live api.zippopotam.us from both Mule and the port; the golden was
recorded from it too. If that service changes an answer, Mule and the port move together but the
stubbed tests do not: refresh `testdata/api/zippopotam-us-*.json` **and** `normalize-response.json`
from the live answers in the same change, and record it in `testdata/README.md` (done once, 2026-09-25).

## The rule when something disagrees

1. **Mule vs golden differ → the golden was wrong.** Mule is the reference. Update the file in
   `testdata/api/`, record it in `testdata/README.md` (what changed, Kernel version, date), then fix the
   port until the tests pass again. Never the other way around, and never adjust the port to a golden
   you have not confirmed.
2. **Port vs Mule differ, golden agrees with Mule → fix the port.**
3. **The difference is listed in `MAPPING.md` "Deviations"** (for the API: deviation 9, non-numeric
   `limit` answers 500 in Mule and 400 in the ports) → expected; leave it, cite the deviation.
4. **A difference not listed there** that you decide to keep → add it as a numbered deviation in
   `MAPPING.md` with the reason, before you move on.

Response bytes are compared after `jq -S` normalisation (key order, whitespace). Number formatting
(`38.6` vs `38.60`), `null` vs missing key and string/number type of a field are real differences, not
normalisation noise: DataWeave is precise about these and the goldens follow it.

## What the first run found (2026-09-25, Kernel 4.12.0)

Running the published `contacts-api` for the first time took four changes to the app before a single
request went through; all are stated in `mule/contacts-api/README.md`. In order of discovery:

1. `<ee:transform>` is EE-only → `<set-payload>` with the same DataWeave (the planned change).
2. `VALIDATION:INVALID_STRING` / `VALIDATION:NOT_A_NUMBER` do not exist → the app never packaged.
3. HTTP connector 1.10.3 does not start on runtime 4.12 → 1.12.1, which needs `mule-maven-plugin` ≥ 4.10.
4. Bare `every` is not a core DataWeave function → every normalize request failed.

Then: `GET /contacts` matched its golden byte for byte; `POST /contacts/normalize` matched except for
four coordinates that api.zippopotam.us has since changed (stubs and golden refreshed); deviation 9
was observed as written. The ports had implemented the *intent* of 2 and 4 correctly without ever
seeing them fail. Draw your own conclusion about the XML as a specification.

The C# port had one test (`ApiTransformTests.ToPlaceParsesCoordinatesAsNumbers`) asserting the old
coordinates as literals instead of reading `testdata/api/zippopotam-us-63131.json` — against prompt
3's "never copy them; reference them by path". Fixed on 2026-09-25 by reading the golden
(`normalize-response.json`); the two synthetic wires in other tests no longer carry fixture-looking
numbers either. Both ports are green on the refreshed fixtures.

C# port parity run (2026-09-25, Kernel 4.12.0, `DB_PROFILE=postgres`, same `COPY` seed, captures under
`tmp/parity/api/`): `GET /contacts` (with and without `limit`), `POST /contacts/normalize`, and the six
400 cases (`city` missing, `limit=0`, the invalid fixture, `{}`, both contact fields missing) are
byte-identical to Mule's after `jq -S`; the `validation:all` body joins its two messages with a
newline in Mule too. The only difference is `limit=abc`: Mule 500 with DataWeave's multi-line
description, the port 400 — deviation 9, as written.

Go port parity run (2026-09-25, Kernel 4.12.0 rebuilt from `docker/mule-ce/`, `DB_PROFILE=postgres`,
same `COPY` seed, captures under `tmp/parity/api-go/`): Mule first — `GET /contacts` and
`POST /contacts/normalize` matched the refreshed goldens byte for byte — then `go run ./cmd/api` on the
same database. `GET /contacts` (with and without `limit`, lower-case city), `POST /contacts/normalize`
(live api.zippopotam.us from both) and seven 400 cases (`city` missing, `city` blank, `limit=0`,
`limit=501`, the invalid fixture, `{}`, both contact fields missing, 51 contacts) are byte-identical to
Mule's after `jq -S`. `limit=abc`: Mule 500, the port 400 — deviation 9, rule case 3. No rule-case-4
additions were needed.

## Batch: what you can and cannot claim

- Prompt 5 in `PROMPTS.md` compares the Go and C# batch on SQLite; extend it to Postgres with
  `DB_PROFILE=postgres`, `TRUNCATE contacts` between runs, and
  `docker compose -f docker/docker-compose.yml exec -T db psql -U contacts -d contacts -c "\copy (SELECT * FROM contacts ORDER BY external_id) TO STDOUT CSV HEADER"`
  from each. That proves the two ports agree, not that either agrees with Mule.
- The batch goldens (`testdata/expected/`) remain derived, not observed. `MAPPING.md` deviations 4 and 5
  name the two places a real EE run could differ. A third candidate, observed on the API run: Mule's
  `error.description` for a failed DataWeave coercion is multi-line — `Cannot coerce String (abc) to
  Number`, a blank line, the script excerpt with a caret, and a `Trace:` block. The ports emit the first
  line only. If `Batch::getFirstException().message` carries the same text on EE, the `Error` value of
  a coercion entry in the errors file differs from the ports' after that first line. Anyone with a Studio trial can settle them: the README
  section *Running the Mule apps* has the procedure; record the outcome in `testdata/README.md`.
