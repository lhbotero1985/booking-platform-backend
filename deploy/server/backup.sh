#!/usr/bin/env bash
# PostgreSQL backups for the QA stack (T3.6). Installed as /usr/local/bin/booking-backup and run
# daily by booking-backup.timer as the deploy user. See docs/operations/backup-restore.md.
#
#   booking-backup                      daily dump (+ weekly copy and restore check on Sundays)
#   booking-backup verify [FILE]        restore FILE (default: newest) into a temporary database
#                                       and compare tables, row counts and migration version
#   booking-backup restore FILE --yes   replace the live database with FILE (stops api/worker;
#                                       takes a safety dump first)
set -euo pipefail
umask 077

DIR=/opt/booking/qa
BACKUP_DIR=$DIR/backups
LOCK=$DIR/.deploy.lock # shared with booking-deploy: never dump during a migration
DB=booking
ADMIN=booking_admin
VERIFY_DB=booking_verify
KEEP_DAILY=7
KEEP_WEEKLY=4
MIN_FREE_MB=1024

log() { printf '%s  %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

cd "$DIR"
install -d -m 700 "$BACKUP_DIR"
pg() { docker compose exec -T postgres "$@"; }
psql_at() { pg psql -U "$ADMIN" -d "$1" -v ON_ERROR_STOP=1 -At -c "$2"; }

# Keeps the newest $2 files matching glob $1 (and their .sha256).
prune() {
  find "$BACKUP_DIR" -maxdepth 1 -name "$1" -printf '%T@ %p\n' | sort -rn |
    tail -n +$(($2 + 1)) | cut -d' ' -f2- |
    while IFS= read -r f; do rm -f "$f" "$f.sha256"; done
}

newest() { find "$BACKUP_DIR" -maxdepth 1 -name 'daily-*.dump' -printf '%T@ %p\n' | sort -rn | head -1 | cut -d' ' -f2-; }

# One line per table: "<table> <rows>", plus the applied migration version.
snapshot() {
  psql_at "$1" "SELECT table_name || ' ' || (xpath('/row/c/text()', query_to_xml(
      format('SELECT count(*) AS c FROM public.%I', table_name), false, true, '')))[1]::text
    FROM information_schema.tables
    WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name"
  echo "goose_version $(psql_at "$1" "SELECT max(version_id) FROM goose_db_version")"
}

dump() { # $1 = output file
  local tmp=$1.partial
  pg pg_dump -U "$ADMIN" -d "$DB" -Fc >"$tmp" || { rm -f "$tmp"; die "pg_dump failed"; }
  # A truncated or corrupt archive fails here.
  pg pg_restore --list <"$tmp" >/dev/null || { rm -f "$tmp"; die "archive is not readable"; }
  mv "$tmp" "$1"
  (cd "$BACKUP_DIR" && sha256sum "$(basename "$1")" >"$1.sha256")
}

drop_verify() { psql_at postgres "DROP DATABASE IF EXISTS $VERIFY_DB WITH (FORCE)" >/dev/null 2>&1 || true; }

cmd_verify() {
  local file=${1:-$(newest)} src got
  [[ -f $file ]] || die "no backup to verify"
  [[ ! -f $file.sha256 ]] || (cd "$BACKUP_DIR" && sha256sum --quiet -c "$(basename "$file").sha256") ||
    die "checksum mismatch: $file"
  log "verify: restoring $(basename "$file") into $VERIFY_DB"
  drop_verify
  trap drop_verify EXIT # also on failure: never leave a second copy of the data around
  psql_at postgres "CREATE DATABASE $VERIFY_DB OWNER app_migrator" >/dev/null
  pg pg_restore -U "$ADMIN" -d "$VERIFY_DB" --exit-on-error <"$file" || die "restore failed"
  src=$(snapshot "$DB")
  got=$(snapshot "$VERIFY_DB")
  if [[ $(cut -d' ' -f1 <<<"$src") != "$(cut -d' ' -f1 <<<"$got")" ]]; then
    diff <(echo "$src") <(echo "$got") || true
    die "verify: restored tables differ from the live database"
  fi
  [[ $(tail -1 <<<"$src") == "$(tail -1 <<<"$got")" ]] || die "verify: migration version differs"
  # Row counts may legitimately differ (the live database moved on since the dump).
  diff <(echo "$src") <(echo "$got") >/dev/null ||
    { log "verify: row counts changed since the dump (live vs restored):"; diff <(echo "$src") <(echo "$got") || true; }
  log "verify OK: $(($(wc -l <<<"$got") - 1)) tables, $(tail -1 <<<"$got")"
}

cmd_restore() {
  local file=${1:-} safety
  [[ -f $file && ${2:-} == --yes ]] || die "usage: booking-backup restore FILE --yes"
  [[ ! -f $file.sha256 ]] || (cd "$BACKUP_DIR" && sha256sum --quiet -c "$(basename "$file").sha256") ||
    die "checksum mismatch: $file"
  safety=$BACKUP_DIR/prerestore-$(date -u +%Y%m%dT%H%M%SZ).dump
  log "restore: safety dump $(basename "$safety")"
  dump "$safety"
  log "restore: stopping api and worker"
  docker compose stop api worker
  log "restore: replacing database $DB with $(basename "$file")"
  if ! pg pg_restore -U "$ADMIN" -d postgres --clean --if-exists --create --exit-on-error <"$file"; then
    log "restore FAILED; the safety dump is $safety"
    docker compose start api worker || true
    exit 1
  fi
  docker compose start api worker
  for _ in $(seq 1 30); do
    curl -fsS --max-time 3 http://127.0.0.1:8080/readyz >/dev/null 2>&1 && { log "restore OK; API ready"; return; }
    sleep 2
  done
  die "restore done but the API is not ready"
}

cmd_daily() {
  local free file stamp
  free=$(df -Pm "$BACKUP_DIR" | awk 'NR==2 {print $4}')
  ((free >= MIN_FREE_MB)) || die "only ${free} MB free"
  exec 9>"$LOCK"
  flock -w 900 9 || die "a deployment holds the lock"
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  file=$BACKUP_DIR/daily-$stamp.dump
  dump "$file"
  log "backup $(basename "$file") ($(du -h "$file" | cut -f1))"
  if [[ $(date -u +%u) == 7 ]]; then
    cp "$file" "$BACKUP_DIR/weekly-$stamp.dump"
    (cd "$BACKUP_DIR" && sha256sum "weekly-$stamp.dump" >"weekly-$stamp.dump.sha256")
    flock -u 9
    cmd_verify "$file"
  fi
  prune 'daily-*.dump' "$KEEP_DAILY"
  prune 'weekly-*.dump' "$KEEP_WEEKLY"
  prune 'prerestore-*.dump' 3
  printf '%s ok %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(basename "$file")" >"$BACKUP_DIR/last-backup"
}

case ${1:-daily} in
  daily) cmd_daily ;;
  verify) cmd_verify "${2:-}" ;;
  restore) cmd_restore "${2:-}" "${3:-}" ;;
  *) die "usage: booking-backup [daily | verify [FILE] | restore FILE --yes]" ;;
esac
