#!/usr/bin/env bash
# QA deployment entry point (T3.5). Installed on the server as /usr/local/bin/booking-deploy
# (root-owned) and bound to the CI SSH key with `restrict,command="..."` in authorized_keys,
# so the CI key can run only this script and nothing else.
#
#   deploy <ghcr image@sha256 digest> <expected version> <actor>
#
# stdin: first line = registry token (the job's ephemeral GITHUB_TOKEN, packages:read).
# The server only accepts images from this repository's GHCR package, pinned by digest, and the
# stack files (compose, Caddyfile, initdb) come from inside that image: CI cannot send
# arbitrary files or commands. See docs/operations/qa-deployment.md.
#
# Steps: pull -> pre-deploy pg_dump -> stack files -> migrations -> up -> smoke tests.
# Any failure after the stack changes rolls back to the previous image and stack files.
# Migrations are not rolled back (expand -> migrate -> contract, docs/architecture/database.md).
set -euo pipefail
umask 027

DIR=/opt/booking/qa
REPO=ghcr.io/lhbotero1985/booking-platform-backend
REGISTRY=${REPO%%/*}
ENV_FILE=$DIR/.env
LOG_FILE=$DIR/deployments.log
BACKUP_DIR=$DIR/backups
KEEP_BACKUPS=5

log() { printf '%s  %s\n' "$(date -u +%H:%M:%S)" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

# --- input -------------------------------------------------------------------------------
cmd=${SSH_ORIGINAL_COMMAND:-$*}
read -r -a args <<<"$cmd"
[[ ${#args[@]} -eq 4 && ${args[0]} == deploy ]] ||
  die "usage: deploy <image@digest> <version> <actor>"
image=${args[1]} version=${args[2]} actor=${args[3]}
[[ $image =~ ^${REPO//./\\.}@sha256:[0-9a-f]{64}$ ]] || die "image must be $REPO@sha256:<digest>"
[[ $version =~ ^[0-9a-f]{7,40}$ ]] || die "invalid version"
[[ $actor =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$ ]] || die "invalid actor"

cd "$DIR"
exec 9>"$DIR/.deploy.lock"
flock -n 9 || die "another deployment is running"

IFS= read -r -t 10 token || die "registry token expected on stdin"
[[ -n $token ]] || die "empty registry token"

env_value() { sed -n "s/^$1=//p" "$ENV_FILE" | tail -1; }
previous=$(env_value API_IMAGE)
domain=$(env_value SITE_DOMAIN)
[[ -n $previous ]] || die "API_IMAGE missing in $ENV_FILE (first deployment is manual)"
log "deploying $image (version $version, actor $actor); current: $previous"

workdir=$(mktemp -d)
trap 'chmod -R u+w "$workdir" 2>/dev/null; rm -rf "$workdir"' EXIT

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

# --- 2. stack files from the image -------------------------------------------------------
cid=$(docker create "$image")
mkdir "$workdir/stack"
docker cp --quiet "$cid:/deploy/qa/." "$workdir/stack/" || { docker rm "$cid" >/dev/null; die "no stack files in image"; }
docker rm "$cid" >/dev/null
[[ -f $workdir/stack/docker-compose.yml ]] || die "image has no /deploy/qa/docker-compose.yml"
API_IMAGE=$image docker compose --env-file "$ENV_FILE" -f "$workdir/stack/docker-compose.yml" \
  config --quiet || die "invalid compose file in image"

# Copies every file of $1 over $2 in place (same inode: file bind mounts see the new content).
copy_tree() {
  local src=$1 dst=$2 rel mode
  while IFS= read -r -d '' rel; do
    rel=${rel#./}
    mode=0644
    [[ -x $src/$rel ]] && mode=0755
    mkdir -p "$(dirname "$dst/$rel")"
    [[ -e $dst/$rel ]] && chmod u+w "$dst/$rel"
    cat "$src/$rel" >"$dst/$rel"
    chmod "$mode" "$dst/$rel"
  done < <(cd "$src" && find . -type f -print0)
}

# --- 3. backup before migrating ----------------------------------------------------------
install -d -m 700 "$BACKUP_DIR"
dump=$BACKUP_DIR/predeploy-$(date -u +%Y%m%dT%H%M%SZ)-$version.dump
docker compose exec -T postgres pg_dump -U booking_admin -d booking -Fc >"$dump" ||
  { rm -f "$dump"; die "pre-deploy backup failed"; }
chmod 600 "$dump"
find "$BACKUP_DIR" -maxdepth 1 -name 'predeploy-*.dump' -printf '%T@ %p\n' | sort -rn |
  tail -n +$((KEEP_BACKUPS + 1)) | cut -d' ' -f2- | xargs -r rm -f
log "backup $(basename "$dump") ($(du -h "$dump" | cut -f1))"

# --- 4. switch stack files (keeping the current ones for rollback) -------------------------
rm -rf "$DIR/rollback"
mkdir -p "$DIR/rollback/qa"
cp -p "$DIR/docker-compose.yml" "$DIR/rollback/"
cp -rp "$DIR/qa/." "$DIR/rollback/qa/"
old_caddyfile=$(sha256sum "$DIR/qa/Caddyfile" | cut -d' ' -f1)
copy_tree "$workdir/stack" "$DIR"

restart_caddy_if_changed() {
  if [[ $(sha256sum "$DIR/qa/Caddyfile" | cut -d' ' -f1) != "$old_caddyfile" ]]; then
    docker compose restart caddy >/dev/null
    log "Caddyfile changed: caddy restarted"
  fi
}

# Readiness through the host port and through Caddy (TLS); optional version check.
smoke() {
  local want=$1 got="" ok=false _
  for _ in $(seq 1 30); do
    if curl -fsS --max-time 3 http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
      got=$(curl -fsS --max-time 3 http://127.0.0.1:8080/healthz | jq -r .version)
      if [[ -z $want || $got == "$want" ]]; then ok=true; break; fi
    fi
    sleep 2
  done
  $ok || { log "smoke: API not ready (version '$got', want '$want')"; return 1; }
  # Caddy may have just been recreated or restarted (new Caddyfile or environment): give it time
  # to listen instead of failing on the first connection refused.
  ok=false
  for _ in $(seq 1 30); do
    if curl -fsS --max-time 5 --resolve "$domain:443:127.0.0.1" "https://$domain/readyz" >/dev/null 2>&1; then
      ok=true; break
    fi
    sleep 2
  done
  $ok || { log "smoke: not reachable through Caddy"; return 1; }
  [[ -n $(docker compose ps --status running -q worker) ]] ||
    { log "smoke: worker not running"; return 1; }
}

record() {
  printf '%s\t%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" "$version" "$image" \
    "$actor" >>"$LOG_FILE"
}

rollback() {
  log "ROLLBACK to $previous"
  copy_tree "$DIR/rollback" "$DIR"
  API_IMAGE=$previous docker compose up -d --remove-orphans
  restart_caddy_if_changed
  if smoke ""; then
    record rolled-back
    log "rolled back; the failed version is not running"
  else
    record rollback-failed
    log "ROLLBACK FAILED: manual intervention required (docs/operations/rollback.md)"
  fi
  exit 1
}

# --- 5. migrations, restart, smoke tests ---------------------------------------------------
log "migrations"
API_IMAGE=$image docker compose run --rm migrate up || rollback
log "starting $version"
API_IMAGE=$image docker compose up -d --remove-orphans || rollback
restart_caddy_if_changed
smoke "$version" || rollback

# --- 6. success: persist, record, clean up old images --------------------------------------
sed -i "s#^API_IMAGE=.*#API_IMAGE=$image#" "$ENV_FILE"
record ok
docker image ls --digests --format '{{.Repository}}@{{.Digest}} {{.ID}}' "$REPO" |
  while read -r ref id; do
    [[ $ref == "$image" || $ref == "$previous" ]] || docker image rm "$id" >/dev/null 2>&1 || true
  done
log "deployed $version"
