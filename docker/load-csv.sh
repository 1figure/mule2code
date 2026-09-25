#!/bin/sh
# Load a contacts CSV straight into the "db" container with COPY, bypassing any batch implementation.
# Used to seed the table for API parity runs. Truncates the table first.
#   docker/load-csv.sh testdata/contact-data-100.csv
set -eu
csv=${1:?usage: load-csv.sh <file.csv>}
compose="docker compose -f $(dirname "$0")/docker-compose.yml"
columns=$(head -1 "$csv")
$compose exec -T db psql -q -U contacts -d contacts -c "TRUNCATE contacts RESTART IDENTITY"
$compose exec -T db psql -q -U contacts -d contacts -c "\copy contacts($columns) FROM STDIN WITH (FORMAT csv, HEADER true)" < "$csv"
$compose exec -T db psql -Atc "SELECT count(*) || ' rows in contacts' FROM contacts" -U contacts -d contacts
