# Siguiente tarea

## Última tarea completada
T1.4 — Base de datos, migraciones y readiness (2026-11-19), en la rama `feat/t1.4-database`.

## Repositorio remoto y flujo

- `main` publicado en https://github.com/lhbotero1985/booking-platform-backend.
- Cada tarea se trabaja en una rama `feat/…` y entra a `main` por Pull Request (ADR-0013).
  QA es un ambiente que despliega desde `main` (Fase 3), no una rama.
- Cada `git push` requiere aprobación del usuario (regla `ask` en `.claude/settings.local.json`).
- Pendiente: abrir PR de `feat/t1.4-database` → `main` y verificar el workflow `security`.

## Próxima tarea: T1.5 — SQLC

- **Objetivo:** generación de código tipado desde SQL, integrada al flujo y verificada en CI.
- **Archivos:**
  - `sqlc.yaml` — motor postgresql, esquema desde `db/migrations`, consultas en
    `db/queries/<modulo>/`, salida por módulo en `internal/modules/<modulo>/infrastructure/sqlcgen`
    (o `internal/platform/...` para consultas técnicas), driver `pgx/v5`, `emit_interface`,
    tipos `uuid`/`timestamptz` mapeados.
  - Una consulta mínima de ejemplo que valide la configuración (sin lógica de negocio).
  - `Taskfile.yml` — `task sqlc` (generate) y `task sqlc:check` (`sqlc diff` / `sqlc vet`).
- **Pruebas:** `sqlc generate` reproducible; `sqlc diff` sin cambios; código generado compila y
  pasa lint; prueba de integración que ejecute la consulta de ejemplo.
- **Comandos de validación:** `task sqlc`, `task check`, `task test:integration`.
- **Criterios de aceptación:** los anteriores en verde y documentados en `test-results.md`.
- **Bloqueos:** ninguno.
