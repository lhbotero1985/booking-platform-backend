#!/usr/bin/env bash
# QA deployment of the frontend web portals (booking-platform-frontend, phase F2). Installed on
# the server as /usr/local/bin/booka-web-deploy (root-owned) and bound to the frontend's CI SSH
# key with `restrict,command="..."` in authorized_keys: that key can run only this script.
#
#   deploy-web <business-web|admin-web> <ghcr image@sha256 digest> <version> <actor>
#
# stdin: first line = registry token (the job's ephemeral GITHUB_TOKEN, packages:read).
# Only images of the frontend repository's GHCR packages, pinned by digest, are accepted. The
# Compose file comes from the backend stack already on the server (qa/web/docker-compose.yml),
# so CI cannot send files or commands. The backend stack (API, database) is never touched.
#
# Steps: pull -> up (only this portal) -> health check + HTTPS through Caddy -> record.
# A failure rolls back to the previous image of that portal (or stops it on a first deploy).
set -euo pipefail
umask 027

DIR=/opt/booking/qa
COMPOSE_FILE=$DIR/qa/web/docker-compose.yml
ENV_FILE=$DIR/.env
STATE_FILE=$DIR/web-images.env
LOG_FILE=$DIR/deployments.log
REPO_PREFIX=ghcr.io/lhbotero1985/booking-platform-frontend
REGISTRY=${REPO_PREFIX%%/*}

log() { printf '%s  %s\n' "$(date -u +%H:%M:%S)" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

# --- input -------------------------------------------------------------------------------
cmd=${SSH_ORIGINAL_COMMAND:-$*}
read -r -a args <<<"$cmd"
[[ ${#args[@]} -eq 5 && ${args[0]} == deploy-web ]] ||
  die "usage: deploy-web <business-web|admin-web> <image@digest> <version> <actor>"
app=${args[1]} image=${args[2]} version=${args[3]} actor=${args[4]}
case $app in
  business-web) host_var=APP_DOMAIN; default_host=booka-app-qa.duckdns.org ;;
  admin-web) host_var=ADMIN_DOMAIN; default_host=booka-admin-qa.duckdns.org ;;
  *) die "unknown portal '$app'" ;;
esac
repo=$REPO_PREFIX/$app
[[ $image =~ ^${repo//./\\.}@sha256:[0-9a-f]{64}$ ]] || die "image must be $repo@sha256:<digest>"
[[ $version =~ ^[0-9a-f]{7,40}$ ]] || die "invalid version"
[[ $actor =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$ ]] || die "invalid actor"
[[ -f $COMPOSE_FILE ]] || die "$COMPOSE_FILE missing (deploy the backend stack first)"

cd "$DIR"
exec 9>"$DIR/.web-deploy.lock"
flock -n 9 || die "another web deployment is running"

IFS= read -r -t 10 token || die "registry token expected on stdin"
[[ -n $token ]] || die "empty registry token"

env_value() {
  if [[ -f $2 ]]; then sed -n "s/^$1=//p" "$2" | tail -1; fi
}
var=$(tr 'a-z-' 'A-Z_' <<<"$app")_IMAGE # BUSINESS_WEB_IMAGE / ADMIN_WEB_IMAGE
previous=$(env_value "$var" "$STATE_FILE")
host=$(env_value "$host_var" "$ENV_FILE")
host=${host:-$default_host}
project=booka-qa-$app
log "deploying $app $image (version $version, actor $actor); current: ${previous:-none}"

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

# --- 1. pull with a throwaway registry login ------------------------------------------------
export DOCKER_CONFIG=$workdir/docker
mkdir -m 700 "$DOCKER_CONFIG"
printf '%s' "$token" | docker login "$REGISTRY" -u ci --password-stdin >/dev/null 2>&1 ||
  die "registry login failed"
unset token
docker pull --quiet "$image" >/dev/null
docker logout "$REGISTRY" >/dev/null
rm -rf "$DOCKER_CONFIG"
unset DOCKER_CONFIG
log "pulled"

compose() { WEB_APP=$app WEB_IMAGE=$1 docker compose -p "$project" -f "$COMPOSE_FILE" "${@:2}"; }
compose "$image" config --quiet || die "invalid web compose file"

# Container healthy (image HEALTHCHECK on /healthz) and reachable through Caddy over TLS.
smoke() {
  local cid status="" _
  cid=$(compose "$1" ps -q web)
  [[ -n $cid ]] || { log "smoke: no container"; return 1; }
  for _ in $(seq 1 30); do
    status=$(docker inspect --format '{{.State.Health.Status}}' "$cid" 2>/dev/null || true)
    [[ $status == healthy ]] && break
    sleep 2
  done
  [[ $status == healthy ]] || { log "smoke: container $status"; return 1; }
  curl -fsS --max-time 5 --resolve "$host:443:127.0.0.1" "https://$host/healthz" >/dev/null ||
    { log "smoke: https://$host/healthz not reachable through Caddy"; return 1; }
}

record() {
  printf '%s\t%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" "$version" "$image" \
    "$actor" >>"$LOG_FILE"
}

save_state() {
  touch "$STATE_FILE" && chmod 640 "$STATE_FILE"
  if grep -q "^$var=" "$STATE_FILE"; then
    sed -i "s#^$var=.*#$var=$1#" "$STATE_FILE"
  else
    printf '%s=%s\n' "$var" "$1" >>"$STATE_FILE"
  fi
}

rollback() {
  if [[ -z $previous ]]; then
    log "ROLLBACK: first deployment of $app failed; stopping it"
    compose "$image" down >/dev/null 2>&1 || true
    record failed-first
    exit 1
  fi
  log "ROLLBACK to $previous"
  if compose "$previous" up -d && smoke "$previous"; then
    record rolled-back
    log "rolled back; the failed version is not running"
  else
    record rollback-failed
    log "ROLLBACK FAILED: manual intervention required"
  fi
  exit 1
}

# --- 2. start this portal only, smoke test ---------------------------------------------------
compose "$image" up -d || rollback
smoke "$image" || rollback

# --- 3. success: persist, record, clean up old images of this portal --------------------------
save_state "$image"
record "ok-$app"
docker image ls --digests --format '{{.Repository}}@{{.Digest}} {{.ID}}' "$repo" |
  while read -r ref id; do
    [[ $ref == "$image" || $ref == "$previous" ]] || docker image rm "$id" >/dev/null 2>&1 || true
  done
log "deployed $app $version"
