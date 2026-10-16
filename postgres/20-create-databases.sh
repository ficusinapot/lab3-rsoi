#!/usr/bin/env bash
set -euo pipefail
VARIANT=v2
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
    -f "/docker-entrypoint-initdb.d/scripts/db-$VARIANT.sql"
