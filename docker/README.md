# docker/

- `docker compose up -d db` — Postgres 16 with the upstream schema (`init/01-contacts.sql`). User/password/db: `contacts`.
- `docker compose --profile mule-ce up -d --build` — additionally builds and starts `mule-api`: the community
  Mule Kernel 4.12.0 (`mule-ce/Dockerfile`) running `../mule/contacts-api` on http://localhost:8082, against
  `db`. Host networking, so the app's own `mule-props-postgres.yaml` applies unchanged. No MuleSoft account
  is needed; the Kernel comes from the public `repository.mulesoft.org` releases repository, at build time.
  `mule-ce/` contains only the Dockerfile — no MuleSoft software is stored in this repository. The Kernel
  is licensed under CPAL 1.0; keep the built image local (do not push it to a registry).
  The batch app cannot be run this way: its Batch module is Enterprise Edition only.
- `docker compose --profile mule up -d` — additionally starts an SFTP server (user `mule`, password `mule`, port 2222)
  with `new/`, `processed/`, `failed/` directories, for running the original batch application from Anypoint
  Studio. The container writes as uid 1001: `chmod -R a+rwX sftp` first.
- `load-csv.sh <file.csv>` — truncates `contacts` and loads the CSV with `COPY`, column list taken from the
  header. Seeds the table for API parity runs without involving any batch implementation.
- `sqlite/contacts.sqlite.sql` — the same schema for SQLite. The ports apply it automatically on start-up;
  the Mule apps need it applied once: `sqlite3 data/contacts.db < docker/sqlite/contacts.sqlite.sql`.
