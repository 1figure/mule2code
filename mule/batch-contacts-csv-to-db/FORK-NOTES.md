# Fork notes (mule-to-code)

This directory is a copy of `batch-contacts-csv-to-db` from
https://github.com/abelisle-mulesoft/mule-4-batch-job-examples (Apache License 2.0, © Alan Belisle).
The upstream license is kept as `LICENSE-upstream`.

Changes made in this fork, all additive:

- `pom.xml`: added the `org.xerial:sqlite-jdbc` driver (dependency + shared library) so the app can run
  against the same SQLite file the C# and Go ports use.
- `src/main/resources/properties/mule-props-sqlite.yaml` and `mule-props-postgres.yaml`: two ready-made
  profiles matching `docker/` in the repo root. Upstream deliberately ships only a template.
- `documentation/assets/` (screenshots) removed to keep the showcase small; see upstream for them.

Nothing in `src/main/mule/*.xml` was changed. The XML is the input to the conversion.

Note that the Batch module used by this app is a Mule Enterprise Edition feature: the app needs an EE
runtime (Anypoint Studio trial, CloudHub) to run. The SFTP listener and SMTP report also need real servers;
`docker/docker-compose.yml` provides an SFTP container for the `new/processed/failed` directories.
