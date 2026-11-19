# Siguiente tarea

## Última tarea completada
T0.3 — Documentación de arquitectura de la Fase 0 (2026-11-18).

## Próxima tarea: T0.4 — Revisión y aprobación de la Fase 0

- **Objetivo:** que el usuario revise la arquitectura, responda `pending-questions.md` y apruebe
  (o pida cambios). Al aprobar, los ADR pasan de `Propuesto` a `Aceptado`.
- **Archivos:** `docs/plan/pending-questions.md`, `docs/plan/decisions.md`, `docs/adr/*`,
  `docs/plan/roadmap.md`, `docs/plan/progress.md`.
- **Dependencias:** respuestas del usuario.
- **Bloqueos:** ninguno técnico.
- **Criterios de aceptación:**
  - Preguntas bloqueantes (P1–P6) respondidas o con valor por defecto aceptado.
  - Aprobación explícita: "Fase 0 aprobada".

## Después de la aprobación: T1.1 — Toolchain y esqueleto del proyecto

- **Requisito previo del usuario:** instalar Go (versión estable vigente) o autorizar su
  instalación; autorizar instalación de `golangci-lint`, `govulncheck`, `sqlc`, `goose`, `task`
  y `pre-commit` (o usar sus imágenes Docker).
- **Archivos:** `go.mod`, `Taskfile.yml`, `cmd/api/main.go`, `cmd/worker/main.go`,
  `internal/platform/...` (vacío mínimo), `.golangci.yml`.
- **Comandos de validación:** `go build ./...`, `go vet ./...`, `golangci-lint run`,
  `govulncheck ./...`, `go test ./...`.
- **Criterios de aceptación:** compila; lint y vet sin errores; estructura coincide con
  `docs/architecture/overview.md` §5.
