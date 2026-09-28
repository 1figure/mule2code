# Repository structure

```
mule-to-code/
├── README.md                 intent, scope, how to run, tests, attribution
├── MAPPING.md                Mule element -> C# / Go, deliberate deviations
├── PROMPTS.md                the literal prompts, in order
├── STRUCTURE.md              this file
├── LICENSE / NOTICE          Apache 2.0; upstream attribution
├── mule/
│   ├── README.md
│   ├── batch-contacts-csv-to-db/     upstream Mule 4 batch app (fork, additive changes only)
│   │   ├── FORK-NOTES.md
│   │   ├── LICENSE-upstream
│   │   ├── pom.xml                   + sqlite-jdbc
│   │   ├── database/postgres-contacts-table.sql
│   │   ├── documentation/            upstream walkthrough (screenshots removed)
│   │   └── src/main/
│   │       ├── mule/global.xml, batch-contacts-csv-to-db-impl.xml     <- the conversion input
│   │       └── resources/
│   │           ├── parse-template/contacts-batch-report-email.template
│   │           ├── properties/mule-props-{sqlite,postgres}.yaml (+ upstream template)
│   │           └── examples/         upstream sample statistics objects
│   └── contacts-api/                 original Mule 4 API app
│       ├── README.md                 incl. the statement of the post-generation modification
│       ├── pom.xml, mule-artifact.json
│       └── src/main/
│           ├── mule/global.xml, contacts-api-impl.xml                  <- the conversion input;
│           │                             impl.xml MODIFIED after generation: ee:transform -> set-payload
│           └── resources/properties/mule-props-{sqlite,postgres}.yaml
├── PARITY.md                 comparing the ports with the Mule runtime; what to do on disagreement
├── docs/                     GitHub Pages landing page (index.md + _config.yml), published from main:/docs
├── .dockerignore             build context of docker/mule-ce (only mule/contacts-api)
├── testdata/                 shared fixtures + OpenAPI contract (see testdata/README.md)
├── docker/
│   ├── docker-compose.yml    postgres (db) + optional sftp (profile "mule") + mule-api (profile "mule-ce")
│   ├── mule-ce/Dockerfile    community Mule Kernel 4.12.0 running contacts-api (no account needed)
│   ├── load-csv.sh           COPY a contacts CSV into the db container (API parity seed)
│   ├── init/01-contacts.sql  postgres schema (upstream DDL)
│   ├── sqlite/contacts.sqlite.sql
│   └── sftp/{new,processed,failed}/
├── dotnet/                   C# port, produced by PROMPTS.md prompt 1 (layout, run, settings in dotnet/README.md)
│   ├── MuleToCode.sln, Directory.Build.props, .editorconfig
│   ├── src/Contacts.Core/    settings, CSV, mapping, validation, batch engine, repository, report, pipeline, zippo, api
│   ├── src/Contacts.Batch/   console host: watch inbox | --once | --file
│   ├── src/Contacts.Api/     minimal API: GET /contacts, POST /contacts/normalize
│   ├── tests/Contacts.Tests/ xunit: golden, batch engine, end-to-end (sqlite), repository (sqlite + postgres), API
│   └── reports/              baseline coverage + cyclomatic complexity (generate.sh), like go/reports/
└── go/                       Go port, produced by PROMPTS.md prompt 2 (layout, run, settings in go/README.md)
    ├── go.mod, go.sum        module muletocode; deps: modernc.org/sqlite, pgx/v5, testcontainers-go (tests)
    ├── cmd/batch/            host of batch-contacts-csv-to-db: watch inbox | -once | -file
    ├── cmd/api/              host of contacts-api: GET /contacts, POST /contacts/normalize
    ├── internal/
    │   ├── config/           global-property env + mule-props-${env}.yaml -> environment variables
    │   ├── contacts/         model, CSV reader, "CSV to SQL" transform + coercions, e-mail validation
    │   ├── batch/            batch:job engine (steps, accept policies, aggregators, maxFailedRecords, statistics)
    │   ├── db/               Database connector: sqlite (modernc) + postgres (pgx/stdlib)
    │   ├── report/           "Extract Key Statistics", parse-template over the upstream HTML, report sink
    │   ├── pipeline/         contacts-batch-process-flow, initialization-flow, send-email-report-flow
    │   ├── zippo/            http:request to api.zippopotam.us
    │   ├── api/              the two HTTP flows: validations, transforms, handlers
    │   ├── resources/        locates ../docker schemas and the upstream e-mail template at run time
    │   ├── mapping/          traceability test: every Mule doc:name must appear in the source
    │   └── testutil/         fixture access for the tests (paths into ../testdata)
    └── reports/              baseline coverage + cyclomatic complexity (generate.sh), like dotnet/reports/
```
