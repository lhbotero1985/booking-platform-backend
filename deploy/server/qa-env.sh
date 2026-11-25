#!/usr/bin/env bash
# Creates /opt/booking/qa/.env with fresh random secrets if it does not exist yet (T3.2).
# Secrets are generated on the server and never leave it (not in Git, not in CI logs).
# Run as deploy:  ssh deploy@HOST 'bash -s' < deploy/server/qa-env.sh
set -euo pipefail

DIR=/opt/booking/qa
ENV_FILE=$DIR/.env
DOMAIN=booking-platform-col.duckdns.org
APP_DOMAIN=booka-app-qa.duckdns.org
ADMIN_DOMAIN=booka-admin-qa.duckdns.org

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
# Frontend portals (Caddy hosts; ADR-F003 of the frontend: /api/* on each portal's origin).
APP_DOMAIN=$APP_DOMAIN
ADMIN_DOMAIN=$ADMIN_DOMAIN
# Email links (/verify-email, /reset-password). Switch to https://$APP_DOMAIN once business-web
# serves those pages (frontend phase F5).
PUBLIC_APP_URL=https://$DOMAIN
# Exact origins allowed for cookie-authenticated mutations (CSRF check) and CORS.
CORS_ALLOWED_ORIGINS=https://$DOMAIN,https://$APP_DOMAIN,https://$ADMIN_DOMAIN

# Set by each deployment (T3.5); image loaded or pulled on this host.
API_IMAGE=
EOF
chmod 600 "$ENV_FILE"
echo "created $ENV_FILE (0600)"
