#!/usr/bin/env bash
# Creates /opt/booking/qa/.env with fresh random secrets if it does not exist yet (T3.2).
# Secrets are generated on the server and never leave it (not in Git, not in CI logs).
# Run as deploy:  ssh deploy@HOST 'bash -s' < deploy/server/qa-env.sh
set -euo pipefail

DIR=/opt/booking/qa
ENV_FILE=$DIR/.env
DOMAIN=booking-platform-col.duckdns.org

install -d -m 750 "$DIR"
if [[ -f "$ENV_FILE" ]]; then
  echo "$ENV_FILE already exists; not touching secrets"
  exit 0
fi

# Hex secrets: no characters that need escaping in a postgres:// URL.
secret() { openssl rand -hex 24; }

umask 077
cat >"$ENV_FILE" <<EOF
# booking-platform QA — generated $(date -u +%Y-%m-%dT%H:%M:%SZ) by deploy/server/qa-env.sh
# Secrets: do not copy outside this server.
POSTGRES_ADMIN_PASSWORD=$(secret)
APP_MIGRATOR_PASSWORD=$(secret)
APP_RUNTIME_PASSWORD=$(secret)

SITE_DOMAIN=$DOMAIN
PUBLIC_APP_URL=https://$DOMAIN
CORS_ALLOWED_ORIGINS=https://$DOMAIN

# Set by each deployment (T3.5); image loaded or pulled on this host.
API_IMAGE=
EOF
chmod 600 "$ENV_FILE"
echo "created $ENV_FILE (0600)"
