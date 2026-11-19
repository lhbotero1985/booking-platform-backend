# Progreso

Registro cronológico. Una entrada por tarea. No borrar entradas anteriores.

---

## 2026-11-18 · Fase 0 · T0.1 — Diagnóstico del repositorio

- **Estado:** ✅ Completada
- **Hallazgos:**
  - `booking-platform/` contiene solo `booking-platform-backend/` y `booking-platform-frontend/`,
    ambas vacías. No existen `infrastructure/`, `docs/` raíz ni `booking-platform.code-workspace`.
  - No había repositorio Git.
  - Herramientas: Git 2.30.1, Docker 27.2.0, Docker Compose v2.29.2. **Go no está instalado.**
    golangci-lint, govulncheck, sqlc, goose, task, pre-commit: no verificados / no instalados.
  - No hay trabajo previo que preservar.
- **Archivos modificados:** ninguno.
- **Pruebas:** no aplica.

## 2026-11-18 · Fase 0 · T0.2 — Git y base DevSecOps

- **Estado:** ✅ Completada
- **Archivos:** `.gitignore`, `.gitattributes`, `.editorconfig`, `.gitleaks.toml`,
  `.pre-commit-config.yaml`, `.github/workflows/secret-scan.yml`, `.github/dependabot.yml`,
  `.github/pull_request_template.md`, `SECURITY.md`.
- **Acciones:** `git init -b main` en `booking-platform-backend/`; commit inicial local
  (sin remoto, sin push).
- **Pruebas:** gitleaks **no se pudo ejecutar** (Docker Desktop apagado); revisión manual por grep
  sin hallazgos. Ver `docs/testing/test-results.md`.
- **Problemas:** Git local 2.30.1 es antiguo (2021); se recomienda actualizarlo.

## 2026-11-18 · Fase 0 · T0.3 — Documentación de arquitectura

- **Estado:** ✅ Completada (pendiente de aprobación)
- **Archivos:** `CLAUDE.md`, `README.md`, `docs/architecture/*` (11 documentos),
  `docs/adr/*` (15 ADR + índice), `docs/plan/*`, `docs/testing/*`, `docs/operations/*`.
- **Pruebas:** no aplica (sin código).
- **Próximos pasos:** T0.4 — revisión del usuario y respuesta a preguntas pendientes.

## 2026-11-18 · Fase 0 · T0.4 — Respuestas del usuario (parcial)

- **Estado:** 🟡 En curso (falta aprobación explícita de la Fase 0)
- **Respuestas:** P1 monorepo · P2 privado · P3 instalación autorizada.
- **Acciones:**
  - `.git` movido de `booking-platform-backend/` a `booking-platform/` (historial conservado).
  - Archivos transversales movidos a la raíz: `.github/`, `.gitignore` (ampliado para Node/Expo/
    Terraform), `.gitattributes`, `.editorconfig`, `.gitleaks.toml`, `.pre-commit-config.yaml`,
    `SECURITY.md`. Nuevos: `README.md` raíz, `booking-platform.code-workspace`.
  - ADR-0014 reescrito y **Aceptado** (monorepo).
  - Herramientas instaladas: Go 1.27.4 (winget), golangci-lint 2.15.0 (winget), Task 3.54.0
    (winget), govulncheck v1.10.0, goose v3.30.1, sqlc v1.31.1 (`go install`).
- **No instalado:** `pre-commit` (requiere Python; pendiente de autorización). Git sigue en 2.30.1.
- **Problemas:** primer `go install` de govulncheck falló por DNS (`lookup proxy.golang.org`);
  el reintento funcionó. Nota: abrir una terminal nueva para que el PATH incluya Go.

## 2026-11-18 · Fase 0 · T0.4 — Aprobación

- **Estado:** ✅ Fase 0 aprobada por el usuario.
- **Aclaración del usuario:** workspace local único, pero **dos repos en GitHub** (backend y
  frontend). Se devolvió `.git` y la configuración DevSecOps a `booking-platform-backend/`
  (historial conservado). ADR-0016 reemplaza a ADR-0014; ADR 0001–0015 aceptados.
- `README.md` y `booking-platform.code-workspace` quedan en la raíz del workspace sin versionar.

## 2026-11-18 · Fase 1 · T1.1 — Toolchain y esqueleto del proyecto

- **Estado:** ✅ Completada
- **Archivos:** `go.mod`, `Taskfile.yml`, `.golangci.yml`, `cmd/api/main.go`,
  `cmd/worker/main.go`, `internal/platform/buildinfo/{buildinfo.go,buildinfo_test.go}`,
  `.github/dependabot.yml` (gomod), `CLAUDE.md` (comandos).
- **Decisiones:** módulo `booking-platform-backend` (provisional); golangci-lint v2 con gosec,
  depguard (pureza del dominio), forbidigo (`fmt.Print` → slog) y goimports con prefijo local;
  solo se crean carpetas con contenido.
- **Pruebas:** `task check` completo en verde; regla depguard verificada con violación
  intencional (archivo temporal eliminado). Ver `test-results.md`.
- **Problemas:** `go test -race` no es ejecutable localmente (sin gcc/cgo en Windows); se
  ejecutará en CI (T1.8). gitleaks sigue pendiente de Docker Desktop.
- **Próximos pasos:** T1.2.

## 2026-11-18 · Fase 1 · Ruta del módulo y push

- Ruta del módulo cambiada a `github.com/lhbotero1985/booking-platform-backend`.
- gitleaks ejecutado con Docker activo: historial y directorio sin fugas.
- Remoto `origin` = `https://github.com/lhbotero1985/booking-platform-backend.git` (autorizado).
- **Push pendiente:** el primer `git push` quedó esperando credenciales (Git Credential Manager)
  y no completó; el reintento fue bloqueado por la política de permisos de Claude Code.
  El usuario debe ejecutar el push manualmente (ver next-task.md).

## 2026-11-18 · Fase 1 · T1.2 — Plataforma HTTP base

- **Estado:** ✅ Completada
- **Archivos:** `internal/platform/config/`, `internal/platform/logging/`,
  `internal/platform/httpx/` (problem, requestid, middleware, json, router + pruebas),
  `internal/app/http.go` (+ pruebas), `cmd/api/main.go` (servidor con timeouts y apagado ordenado).
- **Implementación:** config por env con validación agregada (todos los errores a la vez);
  slog JSON con redacción de claves sensibles; RFC 9457 con `code` y `requestId`; middleware
  request-id, cabeceras de seguridad, access log (sin query string), recover (stack solo en log),
  CORS por lista exacta (preflight denegado → 403), límite de body (413); router stdlib con
  404/405 como problem+json; `GET /healthz`.
- **Pruebas:** ver `test-results.md` (lint 0, pruebas ok, cobertura ≥ 90 %, smoke real).
- **Riesgos:** `-race` y SIGTERM pendientes de verificar en Linux (T1.3/T1.8).
- **Próximos pasos:** T1.3 (Docker + Compose).

## 2026-11-19 · Fase 1 · T1.3 — Docker y Docker Compose (DEV)

- **Estado:** ✅ Completada
- **Archivos:** `deploy/docker/Dockerfile`, `deploy/compose/docker-compose.dev.yml`, `.env.example`,
  `.dockerignore`, `Taskfile.yml` (docker:build, up, down, logs, trivy), `.github/dependabot.yml`
  (docker, docker-compose), `cmd/api` y `cmd/worker` (`time/tzdata` embebido), `CLAUDE.md`.
- **Decisiones:** imagen final distroless/static nonroot; contenedor read-only, sin capabilities,
  no-new-privileges, límites de memoria/pids; puertos solo en 127.0.0.1; PostGIS 17-3.5;
  puerto DEV 55432 (5432 ocupado por PostgreSQL nativo); Trivy 0.58.1 con DB desde GHCR.
- **Pruebas:** ver `test-results.md` (stack arriba, healthz, nonroot, SIGTERM limpio, Trivy 0 HIGH/CRITICAL).
- **Problemas:** DNS intermitente en la máquina (Docker y Go); se mitiga con reintentos.
  Push a GitHub aún pendiente (lo debe ejecutar el usuario).
- **Próximos pasos:** T1.4.

## 2026-11-19 · Fase 1 · Primer push a GitHub

- Regla `ask` para `Bash(git push *)` en `.claude/settings.local.json` (local, ignorado por Git):
  cada push muestra una confirmación al usuario.
- `git push -u origin main` ✅ — `main` publicado en GitHub con 7 commits (hasta T1.3).
- Pendiente: revisar el resultado del workflow `security` (gitleaks) en GitHub Actions.

## 2026-11-19 · Fase 1 · T1.4 — Base de datos, migraciones y readiness

- **Estado:** ✅ Completada (rama `feat/t1.4-database`)
- **Flujo Git:** desde esta tarea, cada tarea en rama `feat/…` → PR a `main` (ADR-0013).
  QA es un ambiente que despliega desde `main`, no una rama (explicado al usuario).
- **Archivos:** `internal/platform/db/` (pool, WithTx, errores PG), `internal/platform/migrate/`,
  `internal/platform/pgtest/` (testcontainers, solo tag `integration`), `db/migrations/`
  (`embed.go`, `00001_extensions.sql`), `internal/platform/config` (DATABASE_URL,
  DATABASE_MIGRATOR_URL, DB_MAX_CONNS, DB_CONNECT_TIMEOUT), `internal/app/http.go` (`/readyz`,
  `Deps`), `cmd/api/main.go` (subcomandos serve / migrate up|down|status), Dockerfile (COPY db),
  compose (DATABASE_URL), `.env.example`, Taskfile (`migrate`, `test:integration`),
  `.golangci.yml` (build tag integration, exclusión G101 en tests).
- **Dependencias nuevas:** pgx v5.13.0, goose v3.30.1, testcontainers-go v0.45.0 (+ postgres).
- **Decisiones:** la API arranca sin BD y `/readyz` lo reporta; `migrate down` prohibido en QA/PROD;
  el Down de 00001 no elimina postgis; timeouts de sesión (statement 15 s, idle in tx 30 s);
  roles `app_migrator`/`app_runtime` diferidos a Fase 3.
- **Pruebas:** ver `test-results.md` (unitarias, integración con PG real, E2E compose).
- **Problemas:** DNS intermitente en Docker y Go (reintentos).
- **Próximos pasos:** PR de la rama a `main`; T1.5 (SQLC).

## 2026-11-19 · Estrategia de ramas (ADR-0017)

- **Estado:** ✅ Documentada (sin cambios en GitHub)
- El usuario definió `main` / `qa` / `feature/*` / `hotfix/*`; reemplaza el flujo trunk-based de
  ADR-0013. Archivos: `docs/adr/0017-branching-strategy-main-qa.md`, `docs/adr/README.md`,
  `docs/adr/0013-…` (nota), `docs/architecture/devsecops.md` §1, `CLAUDE.md`, `decisions.md`,
  `pending-questions.md` (P23–P26).
- Nueva regla: ningún merge, commit, push, PR ni despliegue sin autorización explícita.
- Pendiente de autorización: crear `qa` desde `main`, renombrar `feat/t1.4-database`,
  protección de ramas (requiere GitHub Pro en repo privado), PR de T1.4 → `qa`.

## 2026-11-19 · Fase 1 · T1.5 — SQLC

- **Estado:** ✅ Implementada y probada en `feature/t1.5-sqlc` (creada desde `feat/t1.4-database`).
  **Sin commit**: pendiente de autorización del usuario.
- **Archivos:** `sqlc.yaml`, `db/queries/platform/extensions.sql`,
  `internal/platform/db/platformq/` (generado), `internal/platform/db/db_integration_test.go`,
  `Taskfile.yml` (`sqlc`, `sqlc:check`), `CLAUDE.md`.
- **Decisiones:** esquema leído de las migraciones Goose; driver pgx/v5; overrides globales
  uuid → google/uuid, timestamptz → time.Time (punteros si nullable); un paquete generado por
  módulo.
- **Pruebas:** ver `test-results.md`.
- **Próximos pasos:** autorización de commit/push/PR; luego T1.6 (River).

## 2026-11-19 · Integración en `qa` y ajustes de GitHub

- Rama `qa` creada desde `main`; `feat/t1.4-database` renombrada a `feature/t1.4-database`.
- GitHub CLI 2.105.0 instalado y autenticado.
- PR #3 (T1.4) → `qa`: gitleaks ✅, squash merge `e8b33e6`.
- PR #4 (T1.5, apilado) cerrado automáticamente por GitHub al borrarse su base; rama rebasada
  sobre `qa` (build, vet, lint, tests, `sqlc diff` ✅) y reemplazado por PR #5 → `qa`:
  gitleaks ✅, squash merge `67f3739`.
- PRs #1 (PostGIS 17→18) y #2 (actions/checkout 4→7) de Dependabot cerrados: apuntaban a
  `main` y #1 es un salto mayor de PostgreSQL. `.github/dependabot.yml` ajustado
  (`target-branch: qa`, ignorar mayores de postgis) en `feature/dependabot-qa`.
- Protección de ramas: ninguna (decisión del usuario).
- Nota: `main` local tiene 2 commits de docs no publicados, ya incluidos en `qa` vía #3; se
  alineará con `origin/main` en el primer PR `qa` → `main`.
## 2026-11-19 · Fase 1 · T1.6 — River

- **Estado:** ✅ Implementada y probada en `feature/t1.6-river` (desde `qa`). **Sin commit**:
  pendiente de autorización.
- **Archivos:** `internal/platform/jobs/` (clientes insert/worker, colas default/notifications/
  maintenance, `PingArgs`/`PingWorker`, pruebas de integración), `db/migrations/00002_river.sql`
  (generado con River CLI v0.51.2, sin la versión 1), `cmd/worker/main.go`,
  `internal/platform/config` (`WORKER_SHUTDOWN_TIMEOUT`), `internal/platform/pgtest` (reintentos),
  `sqlc.yaml` (`omit_unused_structs`), compose (servicio `worker`), `.env.example`.
- **Dependencias nuevas:** river v0.51.2 + riverpgxv5.
- **Decisiones:** esquema River como migración Goose (un solo sistema de migraciones); worker
  no arranca con migraciones pendientes; soft stop con timeout y luego cancelación; la API aún
  no usa el cliente de inserción (llegará con los módulos que encolan).
- **Pruebas:** ver `test-results.md`.
- **Pendiente de autorización:** commit/push/PR de T1.6; merge del PR #6 (Dependabot).
- **Próximos pasos:** T1.7 (OpenAPI 3.1).

## 2026-11-19 · Integración de T1.6 y PR #6

- PR #6 (Dependabot → `qa`) squash merge `1eaf3af`.
- T1.6: commit, rebase sobre `qa` (conflictos solo de docs, conservadas ambas partes), PR #7,
  gitleaks ✅, squash merge `e90d6a3`. El clasificador de permisos bloqueó `--delete-branch`;
  la rama `feature/t1.6-river` quedó en local y remoto (borrado pendiente de confirmación).

## 2026-11-19 · Fase 1 · T1.7 — Contrato OpenAPI 3.1

- **Estado:** ✅ Implementada y probada en `feature/t1.7-openapi` (desde `qa`). **Sin commit**:
  pendiente de autorización.
- **Archivos:** `api/openapi.yaml`, `api/spec.go` (embed), `api/spec_test.go`,
  `internal/app/contract_test.go`, `Taskfile.yml` (`openapi:lint`; `test:integration` con
  `DOCKER_HOST` explícito), `go.mod`/`go.sum` (libopenapi, libopenapi-validator).
- **Decisiones:** validación del spec y del contrato en Go (sin imagen Docker extra); 404/405
  como respuestas genéricas `Problem` no listadas por operación; esquemas de seguridad
  declarados (bearer opaco, cookie `__Host-session`) para aplicarse desde la Fase 2.
- **Pruebas:** ver `test-results.md`.
- **Próximos pasos:** T1.8 (GitHub Actions CI).

## 2026-11-19 · Integración de T1.7 y limpieza de ramas

- PR #8 (T1.7) → `qa`: gitleaks ✅, squash merge `768a8d1`.
- Ramas fusionadas borradas (local y remoto) tras verificar que su contenido coincide con el
  squash en `qa`: `feature/t1.6-river`, `feature/t1.7-openapi`; local `feature/dependabot-qa`.
- El usuario pidió no mostrar diálogos de confirmación para acciones ya autorizadas en el chat.
  Claude no puede ampliar sus propios permisos (bloqueado); el usuario debe cambiar la regla
  `ask` → `allow` con `/permissions`.

## 2026-11-19 · Fase 1 · T1.8 — GitHub Actions

- **Estado:** 🟡 Implementada y validada localmente en `feature/t1.8-ci`; PR abierto para
  ejecutar el pipeline real (criterio de aceptación). Merge pendiente de autorización.
- **Archivos:** `.github/workflows/ci.yml`, `.github/workflows/security.yml` (reemplaza
  `secret-scan.yml`), `Taskfile.yml` y `.pre-commit-config.yaml` (gitleaks v8.30.1, Trivy 0.76.2),
  `docs/architecture/devsecops.md` §5.
- **Decisiones:** acciones fijadas por SHA; escáneres como imágenes por digest; versión de
  setup-go con ≥ 2 semanas; `-race` también en integración; security semanal.
