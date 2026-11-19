# Siguiente tarea

## Última tarea completada
T1.3 — Docker y Docker Compose DEV (2026-11-19).

## Repositorio remoto

`main` publicado en https://github.com/lhbotero1985/booking-platform-backend (2026-11-19).
Cada `git push` requiere aprobación explícita del usuario (regla `ask` en
`.claude/settings.local.json`). Pendiente: verificar que el workflow `security` pase en Actions.

## Próxima tarea: T1.4 — Base de datos, migraciones y readiness

- **Objetivo:** conexión a PostgreSQL con pgx, migraciones Goose embebidas, comando de
  migración explícito y `/readyz`.
- **Archivos:**
  - `internal/platform/db/` — `pgxpool` (config por `DATABASE_URL`, timeouts, tamaño de pool),
    helper `WithTx`, mapeo de errores PostgreSQL (`23505`, `23P01`, …).
  - `db/migrations/00001_extensions.sql` — `postgis`, `btree_gist`, `citext`.
  - `db/migrations/embed.go` — `embed.FS`.
  - `cmd/api` — subcomando `migrate up|down|status` (usa `DATABASE_MIGRATOR_URL`).
  - `GET /readyz` — 200 si la BD responde y no hay migraciones pendientes; 503 problem+json si no.
  - `internal/platform/config` — `DATABASE_URL`, `DATABASE_MIGRATOR_URL`, pool.
  - Compose DEV y `.env.example` — variables de BD; `task migrate`.
  - `Taskfile.yml` — `task test:integration` (build tag `integration`, testcontainers-go).
- **Dependencias nuevas:** `github.com/jackc/pgx/v5`, `github.com/pressly/goose/v3`,
  `github.com/testcontainers/testcontainers-go` (+ módulo postgres).
- **Pruebas:**
  - Unitarias: config de BD, mapeo de errores.
  - Integración (PostgreSQL real): migraciones up → down → up; extensiones instaladas;
    `/readyz` 200 con BD y migraciones al día, 503 con BD caída o migración pendiente;
    `WithTx` hace rollback ante error y ante pánico.
  - Seguridad: `DATABASE_URL` nunca aparece en logs; usuario runtime sin permisos DDL (si se
    crean roles en esta tarea).
- **Comandos de validación:** `task check`, `task test:integration`, `task up`, `task migrate`.
- **Criterios de aceptación:** los anteriores en verde y documentados en `test-results.md`.
- **Bloqueos:** DNS intermitente puede requerir reintentos al descargar dependencias.
