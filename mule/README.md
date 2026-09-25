# mule/ — the sources being converted

- `batch-contacts-csv-to-db/` — fork of the upstream batch example (Apache 2.0). `src/main/mule/global.xml`
  holds the connector configs, `src/main/mule/batch-contacts-csv-to-db-impl.xml` the three flows.
  Additive fork changes are listed in `FORK-NOTES.md`.
- `contacts-api/` — a second, original app: `global.xml` + `contacts-api-impl.xml` with the two HTTP flows.
  **Modified after the ports were generated**: its four `<ee:transform>` blocks became `<set-payload>`
  with the identical DataWeave, so that it runs on the community Mule Kernel; two non-existent
  Validation error types in one error handler were corrected (the original did not package); the
  HTTP connector and the Maven plugin were bumped to versions that work on runtime 4.12; and a bare
  `every` (a `dw::core::Arrays` function) was qualified so `POST /contacts/normalize` works at all.
  Stated in full in `contacts-api/README.md`; the ports were generated from the earlier version.

Both apps are standard Mule 4 Maven projects (`mule-application` packaging) and open in Anypoint Studio.
Both ship `properties/mule-props-sqlite.yaml` and `mule-props-postgres.yaml`, selected with `-Denv=<profile>`.

For the conversion, only the `*.xml` files, the DataWeave inside them, the template under
`src/main/resources/parse-template/` and the DDL under `database/` matter.
