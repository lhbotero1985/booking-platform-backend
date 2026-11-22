#!/usr/bin/env bash
# Runs once, when the PostgreSQL data directory is initialized (docker-entrypoint-initdb.d),
# as the superuser. Creates the least-privilege roles (docs/architecture/database.md):
#   app_migrator  owns the database and its schema; runs migrations (DDL).
#   app_runtime   used by api and worker; data access only (no DDL), via default privileges
#                 on everything app_migrator creates.
# Extensions are created here because postgis is not a trusted extension.
set -euo pipefail

: "${APP_MIGRATOR_PASSWORD:?missing}"
: "${APP_RUNTIME_PASSWORD:?missing}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v migrator_pw="$APP_MIGRATOR_PASSWORD" -v runtime_pw="$APP_RUNTIME_PASSWORD" <<'SQL'
CREATE ROLE app_migrator LOGIN PASSWORD :'migrator_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE app_runtime  LOGIN PASSWORD :'runtime_pw'  NOSUPERUSER NOCREATEDB NOCREATEROLE;

CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS citext;

-- The database owner also owns schema public (pg_database_owner, PostgreSQL 15+).
ALTER DATABASE booking OWNER TO app_migrator;
REVOKE ALL ON DATABASE booking FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE booking TO app_runtime;
GRANT USAGE ON SCHEMA public TO app_runtime;

-- Everything the migrator creates is usable (not alterable) by the runtime role.
ALTER DEFAULT PRIVILEGES FOR ROLE app_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE app_migrator IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO app_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE app_migrator IN SCHEMA public
  GRANT EXECUTE ON FUNCTIONS TO app_runtime;

-- Nobody else may connect to the other databases.
REVOKE CONNECT ON DATABASE postgres FROM PUBLIC;
REVOKE CONNECT ON DATABASE template1 FROM PUBLIC;
SQL

echo "app roles created"
