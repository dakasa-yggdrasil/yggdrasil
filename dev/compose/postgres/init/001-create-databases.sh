#!/bin/sh
set -eu

for database in "${POSTGRES_CORE_DB}"; do
  psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER}" --dbname "${POSTGRES_DB}" <<EOSQL
SELECT 'CREATE DATABASE "${database}"'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${database}')\gexec
EOSQL
done
