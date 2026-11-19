# Siguiente tarea

## Última tarea completada
T1.5 — SQLC (2026-11-19), en la rama local `feature/t1.5-sqlc` (sin commit: pendiente de
autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Hotfix: `hotfix/*` → `main` → back-merge a `qa`.
Ningún merge, commit, push, PR ni despliegue sin autorización explícita del usuario.

## Acciones pendientes de autorización

1. Commit de T1.5 en `feature/t1.5-sqlc`.
2. Crear la rama `qa` desde `main` y publicarla (P24).
3. Renombrar `feat/t1.4-database` → `feature/t1.4-database` en local y remoto (P25).
4. PR `feature/t1.4-database` → `qa`; después PR `feature/t1.5-sqlc` → `qa` (rebase sobre `qa`).
5. Decidir protección de `main` y `qa` (P23: GitHub Pro en repo privado).
6. Herramienta para crear PRs: `gh` CLI o MCP de GitHub (P26).

## Próxima tarea: T1.6 — River (jobs en segundo plano)

- **Objetivo:** River sobre PostgreSQL con worker en `cmd/worker` y encolado transaccional.
- **Archivos:**
  - `internal/platform/jobs/` — cliente River (insert-only para la API, con workers para
    `cmd/worker`), registro de workers, colas (`notifications`, `maintenance`), logger slog.
  - Migraciones de River: incorporadas como migración Goose (`00002_river.sql`, generada con
    `river migrate-get`) para mantener un único sistema de migraciones.
  - `cmd/worker/main.go` — config, pool, cliente River, apagado ordenado (SIGTERM).
  - Un job de mantenimiento de ejemplo (no-op con log) para validar el flujo.
  - Compose DEV: servicio `worker`.
- **Pruebas:** integración — job insertado con `InsertTx` dentro de una transacción que hace
  commit se procesa; con rollback no se procesa; unicidad (UniqueOpts) evita duplicados;
  apagado ordenado del worker.
- **Comandos de validación:** `task check`, `task test:integration`, `task up` + logs del worker.
- **Criterios de aceptación:** los anteriores en verde y documentados.
- **Bloqueos:** autorizaciones anteriores (no bloquean el desarrollo local).
