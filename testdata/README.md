# testdata/ — shared fixtures

Both ports test against these files; nothing here is language-specific.

| File | Used for |
|---|---|
| `contact-data-100.csv` | Upstream sample (100 fictitious Salesforce-style contacts, all valid). Happy-path batch run, DB seeding for API tests |
| `contact-data-100-with-errors.csv` | Same file with rows 3 and 7 given an invalid / empty e-mail → 98 successful, 2 failed |
| `expected/first-record.json` | Golden for the "CSV to SQL" mapping of the first row (typed values) |
| `expected/batch-report-100.json`, `expected/batch-report-100-with-errors.json` | Expected on-complete statistics |
| `expected/errors-100-with-errors.json` | Expected content of the `<stem>.<ts>.errors.json` file (error message + row re-serialised as CSV) |
| `api/get-contacts-saint-louis.json` | Expected `GET /contacts?city=Saint%20Louis` after loading `contact-data-100.csv` |
| `api/normalize-request.json`, `api/normalize-response.json` | Expected `POST /contacts/normalize` round trip (two hits, one 404) |
| `api/normalize-request-invalid.json` | A 400 case |
| `api/zippopotam-us-63131.json`, `api/zippopotam-us-91499.json` | Real answers of api.zippopotam.us, used to stub the upstream call in tests |
| `openapi.yaml` | The contract shared by the Mule app and both ports |

Golden values were derived by reading the DataWeave in the Mule XML, not by running Mule.
See `../MAPPING.md` deviations 4 and 5 for the two places a live run could differ.

## Verified against the Mule runtime

`contacts-api` on the community Mule Kernel 4.12.0 (`docker/mule-ce/`, procedure in `../PARITY.md`),
table seeded with `contact-data-100.csv` by `docker/load-csv.sh`, 2026-09-25:

| Golden | Result |
|---|---|
| `api/get-contacts-saint-louis.json` | identical after `jq -S` normalisation |
| `api/normalize-response.json` | identical — after refreshing the coordinates, see below |
| `api/normalize-request-invalid.json` → 400 | 400, `Every contact needs 'mailing_postal_code'` |
| `{"contacts": []}` → 400 | 400, `Body must contain a non-empty 'contacts' array` |
| `limit=abc` (MAPPING.md deviation 9) | 500 from Mule, as the deviation states |

**Refreshed 2026-09-25:** the latitude/longitude in `api/zippopotam-us-63131.json`,
`api/zippopotam-us-91499.json` and `api/normalize-response.json` were changed to what
api.zippopotam.us answers today (63131: 38.6171/-90.4504, was 38.6187/-90.4426; 91499:
33.7866/-118.2987, was 34.1913/-118.4531). Mule's live response matched the service and differed from
the golden only in those four numbers; everything the flows compute matched. The stubs and the golden
move together so that the ports' stubbed tests stay consistent with the recorded response.

The batch goldens (`expected/`) remain unverified: the batch app needs an Enterprise Edition runtime.

`README-upstream-sample-data.md` is the upstream description of the sample data (Apache 2.0, © Alan Belisle).
