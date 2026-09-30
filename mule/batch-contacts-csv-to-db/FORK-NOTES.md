# Fork notes (mule-to-code)

This directory is a copy of `batch-contacts-csv-to-db` from
https://github.com/abelisle-mulesoft/mule-4-batch-job-examples (Apache License 2.0, © Alan Belisle),
taken 2026-09-25T06:46Z. The upstream commit current at that time was `776c9c30b7`
(2026-09-24T23:16Z); `src/main/mule/*.xml` and `properties/mule-props.template.yaml` are byte-identical
to it (verified 2026-09-30). Upstream changed these files later the same day — `789b6ea919`
(2026-09-25T20:56Z): `app-props-*.yaml`, aggregator size hard-coded; `8b3b0ee973` (2026-09-25T21:47Z):
`sftp.archive_dir` — so a diff against upstream `main` shows upstream's changes, not this fork's.
This copy is intentionally frozen at that commit: it is the conversion input, and the ports were
generated from it. Do not sync it with upstream. The upstream license is kept as `LICENSE-upstream`.

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
