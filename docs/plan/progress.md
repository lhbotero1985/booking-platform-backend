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

## 2026-11-19 · Fase 1 · Integración de T1.8

- PR #9 (T1.8) → `qa`: los 6 jobs en verde; squash merge `bbf7626`; workflows `ci` y
  `security` también en verde sobre `qa`. Rama `feature/t1.8-ci` borrada (local y remoto).
- Commit y push en `feature/*`: autorización permanente del usuario (CLAUDE.md regla 9).

## 2026-11-19 · Fase 1 · T1.9 — Seguridad básica y cierre de fase

- **Estado:** ✅ Implementada en `feature/t1.9-phase1-closure`. Integración local no verificable
  (Docker Desktop detenido); queda a cargo del CI del PR.
- **Archivos:** `internal/app/server.go` (`NewHTTPServer`, `MaxHeaderBytes`), `cmd/api/main.go`
  (usa `NewHTTPServer`), `tests/security/baseline_test.go`, `Taskfile.yml` (`test:security`),
  `docs/plan/phase-1-closure.md`, `docs/plan/roadmap.md` (desglose Fase 2), `test-results.md`.
- **Resultado:** Fase 1 completa; **pendiente de aprobación del usuario** para cerrarla e
  iniciar la Fase 2.

## 2026-11-19 · Cierre de la Fase 1

- PR #10 (T1.9) → `qa`: 6/6 checks ✅, squash merge `1caba7d`.
- **Fase 1 aprobada por el usuario.** Inicio de la Fase 2 autorizado.

## 2026-11-19 · Fase 2 · T2.1 — Esqueleto del módulo identity

- **Estado:** ✅ Completada en `feature/t2.1-identity-skeleton` (commit y push con autorización
  permanente; PR y merge pendientes de autorización).
- **Archivos:** `db/migrations/00003_identity.sql`, `db/queries/identity/*.sql`, `sqlc.yaml`
  (entrada identity), `internal/modules/identity/{domain,application,interfaces/http,contract}/doc.go`,
  `internal/modules/identity/infrastructure/sqlcgen` (generado),
  `internal/modules/identity/infrastructure/schema_integration_test.go`, `.golangci.yml`
  (reglas application-layer, identity-module-boundary, identity-domain-inward,
  identity-application-inward), docs.
- **Decisiones:** expiración evaluada con el `now` de la aplicación (reloj inyectable) y no con
  `now()` de PostgreSQL; FK de `business_id` diferida a la Fase 4; CHECKs de defensa en
  profundidad (hash PHC argon2id, SHA-256 de tokens, E.164, formato de acción de auditoría).

## 2026-11-19 · Fase 2 · T2.1 integrada

- PR #11 → `qa`: 6/6 ✅, squash merge `a305183`. Rama `feature/t1.9-phase1-closure` borrada.

## 2026-11-19 · Fase 2 · T2.2 — Dominio de identidad y Argon2id

- **Estado:** ✅ Completada en `feature/t2.2-identity-domain-argon2` (commit y push; PR y merge
  pendientes de autorización).
- **Archivos:** `identity/domain/{errors,email,password,user}.go` + pruebas y fuzzing,
  `identity/application/ports.go` (`PasswordHasher`), `identity/infrastructure/argon2/` +
  pruebas, fuzzing y benchmarks, `internal/platform/config` (`ARGON2_*`), `.env.example`.
- **Decisiones:** `Password` no imprimible; límites de costo al decodificar hashes (anti-DoS);
  semáforo de concurrencia; `VerifyDummy` para no revelar emails registrados por tiempo de
  respuesta; política NIST sin reglas de composición; t=3 por defecto con calibración en QA.

## 2026-11-19 · Fase 2 · T2.2 integrada

- PR #12 → `qa`: 6/6 ✅, squash merge `ebc7f98`. Rama `feature/t2.1-identity-skeleton` borrada.
- Usuario: **verificación de email obligatoria** antes del primer login. Autorización permanente
  de commit/push escrita en `CLAUDE.md` (regla 9) y ADR-0017.

## 2026-11-19 · Fase 2 · T2.3 — Registro y verificación de email

- **Estado:** ✅ Completada en `feature/t2.3-register-verify-login` (commit y push; PR y merge
  pendientes de autorización). Login se movió a T2.4 (desglose actualizado en el roadmap).
- **Archivos:** migración `00004_email_verification.sql`, consultas, `platform/clock`,
  `platform/mail` (SMTP), dominio (`VerifyEmail`, `Token`), aplicación (puertos, errores de
  validación, `registration.go`, `apptest`), infraestructura (`postgres` store, `worker` job),
  `interfaces/http` (handlers), `internal/app` (módulos, `NewIdentityService`), `cmd/api`,
  `cmd/worker`, `httpx.DecodeJSON` (415), OpenAPI, config (`SMTP_*`, `PUBLIC_APP_URL`), compose
  (Mailpit, límites de memoria), `.env.example`, `.gitleaks.toml`.
- **Hallazgo corregido:** límite de memoria de la API (256 MiB) = Argon2 64 MiB × 4 concurrentes →
  riesgo de OOM; API a 512 MiB y `ARGON2_MAX_CONCURRENT=2` en DEV.

## 2026-11-19 · Fase 2 · T2.3 integrada

- Decisión del usuario: **QA simula los correos con Mailpit** (TLS SMTP obligatorio solo en
  PROD; UI de Mailpit en QA solo por túnel SSH). PR #13 → `qa`: 6/6 ✅, merge `969d6eb`.
  Rama `feature/t2.2-identity-domain-argon2` borrada.

## 2026-11-19 · Fase 2 · T2.4 — Login, sesiones y middleware de autenticación

- **Estado:** ✅ Completada en `feature/t2.4-login-sessions` (commit y push; PR y merge
  pendientes de autorización).
- **Archivos:** `identity/domain/session.go`, `identity/contract/principal.go`,
  `identity/application/session.go` (+ puertos, `apptest`), `identity/infrastructure/postgres`
  (sesiones), `identity/interfaces/http/auth.go` (login, logout, logout-all, `/me`,
  `RequireAuth` con CSRF por Origin), OpenAPI, `tests/security/auth_timing_test.go`,
  `.env.example` (`CORS_ALLOWED_ORIGINS` DEV).
- **Pendiente para Fase 3:** IP real detrás de Caddy (X-Forwarded-For solo desde el proxy).

## 2026-11-19 · Fase 2 · T2.4 integrada

- PR #14 → `qa`: 6/6 ✅, squash merge `7264966`. Rama `feature/t2.3-register-verify-login` borrada.

## 2026-11-19 · Fase 2 · T2.5 — Recuperación y cambio de contraseña

- **Estado:** ✅ Completada en `feature/t2.5-password-reset` (commit y push; PR y merge pendientes
  de autorización).
- **Archivos:** `db/queries/identity/password_reset.sql`, `identity/application/password.go`
  (+ puertos, `apptest`, `newToken` compartido), `identity/infrastructure/postgres` (reset, revocar
  otras sesiones), `identity/infrastructure/worker` (3 jobs de email), `identity/interfaces/http/password.go`,
  OpenAPI (3 operaciones), pruebas.
- **Endpoints:** `POST /api/v1/auth/password/forgot`, `POST /api/v1/auth/password/reset`,
  `POST /api/v1/me/password`.

## 2026-11-19 · Fase 2 · T2.5 integrada

- PR #17 → `qa`: 6/6 ✅, squash merge `af1a4ae`. Ramas `feature/t2.4-login-sessions` y
  `feature/t2.5-password-reset` borradas. PR de Dependabot #15 (`actions/checkout` 4→7) y #16
  (`postgis` 17→18) cerrados con autorización: apuntaban a `main` porque la configuración que los
  dirige a `qa` aún no está en `main`.

## 2026-11-19 · Fase 2 · T2.6 — RBAC y autorización por recurso

- **Estado:** ✅ Completada en `feature/t2.6-rbac` (commit y push; PR y merge pendientes de
  autorización).
- **Archivos:** `internal/platform/authz` (sentinelas), `internal/platform/httpx/authz.go`
  (`WriteAuthzProblem`, código `FORBIDDEN`), `identity/contract/authz.go` (roles, conjuntos,
  `Authorizer`, políticas puras), `identity/application/authz.go` (`RequireBusinessRole`,
  `RequirePlatformAdmin`, `MyMemberships`; puerto `ActiveMembershipsByUser`),
  `identity/infrastructure/postgres` (consulta de membresías), `apptest` (membresías),
  `identity/interfaces/http/memberships.go`, OpenAPI, `docs/architecture/security.md` (matriz).
- **Endpoint:** `GET /api/v1/me/memberships`.
- **Notas:** el patrón se prueba de extremo a extremo con rutas de prueba (403 mismo negocio,
  404 otro negocio idéntico a ruta inexistente, revocación inmediata) sin adelantar la Fase 4.
  La gestión de membresías (invitar, cambiar rol, revocar) llega con los negocios en la Fase 4.

## 2026-11-19 · Fase 2 · T2.6 integrada

- PR #18 → `qa`: 6/6 ✅, squash merge `76fc4f9`. Rama `feature/t2.6-rbac` borrada.

## 2026-11-19 · Fase 2 · T2.7 — Rate limiting y defensa contra fuerza bruta

- **Estado:** ✅ Completada en `feature/t2.7-rate-limiting` (commit y push; PR y merge pendientes
  de autorización).
- **Archivos:** `internal/platform/ratelimit` (token bucket y backoff), `internal/platform/httpx/ratelimit.go`
  (`ClientIP`, `ClientKey`, `RateLimit`, `WriteRateLimited`), `internal/platform/config`
  (`RATE_LIMIT_*`, `LOGIN_BACKOFF_*`), `internal/app` (`NewRateLimiters`, límite general con sondas
  exentas), `cmd/api`, `cmd/worker`, `identity/application/throttle.go` (+ `Login`,
  `ChangePassword`), `identity/interfaces/http` (límite estricto, 429), OpenAPI (`TooManyRequests`
  en 7 operaciones), `tests/security/bruteforce_test.go`, `.env.example`, `security.md`.
- **Comportamiento:** general 300/min (ráfaga 100) por cliente; estricto 10/min (ráfaga 10)
  compartido por los endpoints de credenciales y email; backoff de login desde el 5.º fallo
  (1 s → 15 min); 429 `RATE_LIMITED` con `Retry-After`, idéntico exista o no la cuenta.
- **Abiertos:** P28 (ataques distribuidos), P29 (NAT); IP real detrás de Caddy en la Fase 3.

## 2026-11-19 · Fase 2 · T2.7 integrada

- PR #19 → `qa`: 6/6 ✅, squash merge `9129419`. La rama `feature/t2.7-rate-limiting` sigue
  existiendo (borrado pendiente de autorización).

## 2026-11-19 · Fase 2 · T2.8 — Auditoría básica

- **Estado:** ✅ Completada en `feature/t2.8-audit` (commit y push; PR y merge pendientes de
  autorización).
- **Archivos:** `internal/platform/reqctx` (nuevo), `httpx/requestid.go` (request ID e IP en
  `reqctx`), `db/queries/identity/memberships_audit.sql` (`occurred_at` desde la aplicación) +
  sqlc, `identity/application/audit.go` (eventos, resultados, motivos, constructor,
  `auditDetached`, `emailFingerprint`), casos de uso instrumentados (`registration.go`,
  `session.go`, `password.go`, `authz.go`), puerto `Tx.AppendAudit`, `postgres` y `apptest`,
  `contract.Authorizer.RequirePlatformAdmin(ctx, p)`, pruebas, `security.md`.
- **Notas:** en DEV la IP auditada es la puerta de enlace de Docker; la IP real llega con Caddy
  (Fase 3). Retención/consulta en la Fase 12 (P30).

## 2026-11-19 · Fase 2 · T2.8 integrada

- PR #20 → `qa`: 6/6 ✅, squash merge `54f8513`. Rama `feature/t2.7-rate-limiting` borrada.

## 2026-11-19 · Fase 2 · T2.9 — Suite de seguridad y cierre de fase

- **Estado:** ✅ Completada en `feature/t2.9-security-closure` (commit y push; PR y merge
  pendientes de autorización). **Fase 2 pendiente de aprobación.**
- **Archivos:** `tests/security/auth_suite_test.go` (8 pruebas nuevas sobre el stack HTTP
  completo), `identity/infrastructure/worker/worker_test.go` (0 % → 100 %),
  `docs/plan/phase-2-closure.md` (entregables, checklist, revisión ASVS L2 V2/V3/V4, riesgos).
- **Hallazgos:** brechas ASVS frente a la política aprobada → P31 (mínimo 12 y lista de
  contraseñas filtradas), P32 (MFA para administradores), P33 (listado de sesiones). No se
  modificó la arquitectura aprobada: requieren decisión del usuario.

## 2026-11-19 · Fase 2 · T2.9 integrada y decisiones de cierre

- PR #21 → `qa`: 6/6 ✅, squash merge `32f7128`. Ramas `feature/t2.8-audit` y
  `feature/t2.9-security-closure` borradas.
- Decisiones del usuario: P31 → mínimo 8 y lista pequeña (se pidió 7; se advirtió que queda bajo
  NIST y el usuario eligió 8); P32 y P33 → Fase 12; **cierre de la Fase 2 aprobado**.

## 2026-11-19 · Fase 2 · T2.10 — Política de contraseñas (mínimo 8)

- **Estado:** ✅ Completada en `feature/t2.10-password-policy` (commit y push; PR y merge
  pendientes de autorización).
- **Archivos:** `identity/domain/password.go` (`MinPasswordLength = 8`, 30 contraseñas comunes de
  8–9 caracteres añadidas), `domain_test.go`, `api/openapi.yaml` (`minLength: 8` en 3 campos),
  `security.md`, `domain-model.md`, `phase-2-closure.md` (aprobada), roadmap, decisiones,
  preguntas pendientes.
- **Compatibilidad:** sin migración; las contraseñas existentes siguen siendo válidas.

## 2026-11-22 · Fase 2 · T2.10 integrada

- PR #22 → `qa`: 6/6 ✅, squash merge `d982e06`. Rama `feature/t2.10-password-policy` borrada.

## 2026-11-22 · Fase 3 · T3.1 — Preparación y endurecimiento del servidor QA

- **Estado:** ✅ Completada en `feature/t3.1-qa-server-bootstrap` (commit y push; PR y merge
  pendientes de autorización). Desglose de la Fase 3 aprobado por el usuario.
- **Infraestructura:** Droplet creado por el usuario; renombrado a `booking-qa` (autorizado). Las
  etiquetas no se pueden gestionar con las herramientas MCP disponibles (el usuario puede
  agregarlas en el panel). Redimensión a 2 GB pendiente de confirmar el precio real (P34).
- **Archivos:** `deploy/server/bootstrap.sh`, `docs/operations/server-bootstrap.md`,
  `.github/workflows/ci.yml` (shellcheck), `Taskfile.yml` (`task shellcheck`), roadmap (Fase 3),
  `deployment.md`, `qa-deployment.md`, decisiones, preguntas P34–P36.
- **Problemas:** Docker Desktop estaba detenido para shellcheck local → se inició y se reintentó.

## 2026-11-22 · Fase 3 · T3.1 integrada y decisiones

- PR #23 → `qa`: 6/6 ✅ (incluye shellcheck), squash merge `c4952dd`.
- Decisiones del usuario: P34 seguir con 1 GB y validar; P35 DuckDNS
  `booking-platform-col.duckdns.org` (resuelve a `174.138.92.121`, verificado); P36 solo `ufw`.

## 2026-11-22 · Fase 3 · T3.2 — Stack QA en Docker Compose

- **Estado:** ✅ Completada en `feature/t3.2-qa-compose-stack` (commit y push; PR y merge
  pendientes de autorización). **Stack desplegado y funcionando en el servidor QA.**
- **Archivos:** `deploy/compose/docker-compose.qa.yml`, `deploy/compose/qa/initdb/10-app-roles.sh`,
  `deploy/server/qa-env.sh`, `Taskfile.yml` (shellcheck de todos los scripts),
  `docs/operations/qa-deployment.md`, `database.md`, roadmap, decisiones, preguntas P34–P36.
- **Despliegue:** imagen `booking-platform-backend:qa-c4952dd` transferida por SSH; secretos
  generados en el servidor; migraciones 00001–00004 aplicadas con `app_migrator`.
- **Problemas:** fallo DNS local al construir la imagen (reintento OK); reloj del PC del usuario
  ~3,5 min adelantado (el servidor está sincronizado por NTP).

## 2026-11-22 · Fase 3 · T3.2 integrada

- PR #24 → `qa`: 6/6 ✅, squash merge `9bacd39`. Rama `feature/t3.1-qa-server-bootstrap` borrada.
- El usuario pidió configurar el despliegue desde GitHub al servidor (T3.4 + T3.5, tras T3.3).

## 2026-11-25 · Fase 7 · T7.4 — Recordatorios

- **Previo:** el usuario aprobó el texto de `recordatorio_cita` y decidió: sin recordatorio para
  citas confirmadas; inmediato si se agenda dentro de la ventana.
- **Estado:** ✅ en `feature/t7.4-reminders` (commit y push; PR pendiente de autorización). El
  usuario debe crear `recordatorio_cita` en Meta.
- **Hecho:** tipo `APPOINTMENT_REMINDER`, `send_at` y unicidad por tipo (migración 00021); plan
  del recordatorio en la transacción del cambio con job River programado; revalidación al vencer
  (`APPOINTMENT_CONFIRMED`); plantilla con sus parámetros; `sendAt` en la vista del personal;
  `BookingPolicy` con la configuración de recordatorios. Los avisos inmediatos ya no llevan hora
  de programación (se detectó con el reloj fijo de las pruebas).

## 2026-11-25 · Fase 7 · T7.3 — Mensajes de la cita

- **Previo:** el usuario confirmó que recibió el "Hello World" de T7.2; aprobó los 5 textos y los
  creó en Meta (Utilidad, Spanish; "En revisión"); decidió que los botones Confirmar / Cancelar
  vayan solo en el recordatorio (T7.4).
- **Estado:** ✅ PR #58 → `qa` (merge autorizado sin esperar a Meta): CI 6/6, squash `641e69c`;
  `release-qa` ✅; E2E en QA **198/198** (sin envío real). Pendiente: E2E con `WHATSAPP_TEST_PHONE`
  cuando Meta apruebe las plantillas (siguen "En revisión").
- **Hecho:** `AppointmentState` con cliente, servicio, barbero, negocio, zona y motivo; contenido y
  fecha en español; las 5 plantillas con sus parámetros; tipo `APPOINTMENT_CREATED_BY_BUSINESS`
  (migración 00020); `GET /app` con `APP_DOWNLOAD_URL`; el worker lee negocios y profesionales.

## 2026-11-25 · Fase 7 · T7.2 — Cliente de WhatsApp Cloud API

- **Previo:** el usuario creó la app de Meta "booking-platform", registró su celular como
  destinatario de prueba (recibió el "Hello World"), generó un token permanente de usuario del
  sistema y cargó las 4 variables en `/opt/booking/qa/.env` (verificadas sin ver valores:
  formato correcto, archivo `600`). Decidió dejar el token en el servidor (no en GitHub) y no rotar
  las contraseñas de la base expuestas en una captura (riesgo aceptado).
- **Estado:** ✅ en `feature/t7.2-whatsapp-cloud-api` (commit y push; PR pendiente de autorización).
- **Hecho:** cliente `infrastructure/whatsapp` (plantilla `hello_world` hasta T7.3, clasificación de
  errores de Meta, sin datos sensibles en errores); configuración `WHATSAPP_ACCESS_TOKEN`,
  `WHATSAPP_PHONE_NUMBER_ID`, `WHATSAPP_BUSINESS_ACCOUNT_ID`, `WHATSAPP_API_BASE_URL`,
  `WHATSAPP_GRAPH_VERSION` con validación (lista blanca obligatoria fuera de PROD); elección
  Meta/simulado en el composition root; credenciales solo al worker en QA; E2E adaptado a la lista
  blanca y sección opcional con envío real; guía en `qa-deployment.md`.
- **Cierre:** PR #57 → `qa` autorizado: CI 6/6 ✅ (tras corregir un falso positivo de gitleaks),
  squash `d9b29bb`; `release-qa` ✅; worker con `provider=meta`; E2E en QA **203/203** con un
  WhatsApp real aceptado por Meta.

## 2026-11-24 · Fase 7 · T7.1 — Base de notificaciones

- **Previo:** PR #55 (T6.8) → `qa`: CI 6/6 ✅, squash `f7ce6f9`, rama borrada; `release-qa` ✅;
  E2E en QA **194/194**. Desglose de la Fase 7 aprobado (decisiones en `decisions.md`).
- **Estado:** ✅ en `feature/t7.1-notifications-foundation` (commit y push; PR pendiente de
  autorización).
- **Hecho:** módulo `notifications`; tabla `notification_messages` (migración 00019); contrato
  `scheduling.EventListener` llamado en la transacción de cada cambio; job River
  `notifications.send_message` con revalidación de la cita; envío simulado; `WHATSAPP_ENABLED` /
  `WHATSAPP_ALLOWLIST`; ruta `GET …/appointments/{appointmentId}/notifications` (82 rutas).
- **Pruebas:** dominio 100 %, aplicación 92,2 %; integración con PostgreSQL y River reales (el
  mensaje se deshace con el cambio; el worker real entrega); mutación detectada; E2E con 3
  comprobaciones nuevas.
- **Cierre:** PR #56 → `qa` autorizado: CI 6/6 ✅, squash `20ed86f`, rama borrada; `release-qa` ✅;
  E2E en QA **197/197**.

## 2026-11-24 · Fase 6 · T6.8 — Cierre de la Fase 6

- **Previo:** PR #54 (T6.7) → `qa`: CI 6/6 ✅, squash `b112f37`, rama borrada; `release-qa` ✅;
  E2E en QA **194/194**.
- **Estado:** ✅ en `feature/t6.8-phase6-closure`. **Fase 6 aprobada por el usuario
  (2026-11-24)**. P40 decidido (WhatsApp con el enlace de la app al agendar manualmente).
- **Revisión de las 7 pruebas de concurrencia del diseño:** 5 existían; faltaban las
  confirmaciones concurrentes (`TestConcurrentConfirmationsOnPostgres`) y la reserva en el cambio
  de horario de verano (`TestBookingAcrossDST`): agregadas y en verde.
- **Cobertura:** la aplicación de negocios había bajado a 81,9 % (funciones nuevas solo con
  integración) → pruebas unitarias agregadas, 90,2 %.
- **Aislamiento y credenciales:** guiadas por el contrato, cubren solas todas las rutas nuevas
  (39 + 24 operaciones; 66 autenticadas).

## 2026-11-24 · Fase 6 · T6.7 — Coherencia de la agenda

- **Previo:** PR #53 (T6.6) → `qa`: CI 6/6 ✅, squash `3106e54`, rama borrada; `release-qa` ✅
  (migración 18); E2E en QA **185/185**.
- **Estado:** ✅ en `feature/t6.7-agenda-coherence` (commit y push; PR pendiente).
- **Archivos:** contrato nuevo `scheduling/contract` (`AgendaGuard`); `scheduling/application/
  guard.go` (conflictos, por reasignar), query `ActiveAppointmentsDuring`; professionals: bloqueo en
  ausencias, `ConflictError`, `SetAgendaGuard`, verificación en ausencias y horario, HTTP 409 con
  la lista; `httpx.Problem.Conflicts`; `internal/app` (`LinkAgendaGuard`, dependencia `Bookable`);
  depguard (professionals puede usar `scheduling/contract`); OpenAPI (81 rutas); pruebas; E2E;
  Postman (paso 12).
- **Hallazgo:** la primera versión de la prueba de carrera reserva ∥ ausencia no detectaba la
  mutación "sin bloqueo" (ventana demasiado estrecha). Se hizo determinista con una pausa de 100 ms
  en el contrato entre la verificación y la escritura: ahora la mutación falla en la 1.ª ronda.

## 2026-11-24 · Fase 6 · T6.6 — Cambios de estado de la cita

- **Previo:** PR #52 (T6.5) → `qa`: CI 6/6 ✅, squash `66d3020`, rama borrada; `release-qa` ✅;
  E2E en QA **174/174**.
- **Plan aprobado por el usuario** (flujos, estados, permisos y 3 decisiones; "no vino" libera
  el horario).
- **Estado:** ✅ en `feature/t6.6-appointment-changes` (commit y push; PR pendiente).
- **Archivos:** migración `00018_appointment_changes.sql`; queries (`LockAppointment`,
  `UpdateAppointment` con versión, `AppointmentEvents`, id en citas activas); dominio
  `transitions.go`; aplicación `changes.go`; store PostgreSQL y en memoria; HTTP `changes.go` (10
  rutas); contrato `businesses.BookingPolicy` con los límites de horas; OpenAPI (80 rutas); pruebas
  (dominio, aplicación, contrato, PostgreSQL con concurrencia, aislamiento); E2E; Postman (paso 11).
- **Hallazgos:** mutación sin orden fijo de bloqueos → deadlock detectado por PostgreSQL (la
  prueba de reasignaciones cruzadas lo atrapa). Dos fallos de mis pruebas corregidos antes de
  correrlas (destinos ocupados entre sí; "no vino" sobre una hora ya pasada).

## 2026-11-24 · Fase 6 · T6.5 — Agenda y mis citas

- **Previo:** PR #51 (T6.4) → `qa`: CI 6/6 ✅, squash `15ac654`, rama borrada; `release-qa` ✅;
  E2E en QA **166/166** (registro con celular, cita manual rápida, 403 agenda ajena, 409).
- **Estado:** ✅ en `feature/t6.5-agenda` (commit y push; PR pendiente).
- **Archivos:** queries `AgendaAppointments`, `CustomerUpcomingAppointments`,
  `CustomerPastAppointments`; `domain/view.go`; `application/agenda.go`; store PostgreSQL y en
  memoria; `interfaces/http/agenda.go`; contratos `Roster` y `BusinessSummaries`; OpenAPI (3
  operaciones, esquemas `Agenda`, `AgendaItem`, `MyAppointment`, `MyAppointmentList`; 70 rutas);
  pruebas; E2E; Postman (paso 10); P40 en preguntas pendientes.
- **Problemas:** heredocs del shell (scripts a archivo); el entorno local temporal tomó los
  puertos por defecto (el DEV original estaba apagado, sin conflicto).

## 2026-11-24 · Fase 6 · T6.4 — Clientes del negocio y citas manuales

- **Previo:** PR #50 (T6.3) → `qa`: CI 6/6 ✅, squash `c05c951`, rama borrada; `release-qa` ✅;
  E2E en QA **157/157** (reserva, repetición, 409, 422, disponibilidad).
- **Decisión del usuario:** celular obligatorio al crear la cuenta (commit `7ce0354` en esta rama).
- **Estado:** ✅ en `feature/t6.4-manual-appointments` (commits y push; PR pendiente).
- **Archivos:** registro con celular (identity, OpenAPI, pruebas que registran cuentas);
  migración `00017_customers.sql`; queries de clientes; scheduling (`domain/customer.go`,
  `application/customers.go`, `application/manual.go`, enlace del cliente en la reserva desde la
  app, `interfaces/http/staff.go`, doble en memoria); contratos `businesses.BookingPolicyOf` y
  `professionals.Agenda.ProfessionalUserID`; OpenAPI (4 operaciones, esquemas `Customer` y
  `CustomerList`; 67 rutas); pruebas; E2E; Postman (paso 9); docs.
- **Ajuste del usuario (cita rápida):** el barbero escribe nombre, celular y correo (opcional)
  del cliente en la misma cita; se reutiliza por celular o se crea. Migración 00017 con `email`
  (aún no aplicada en QA, se ajustó antes del merge).
- **Problemas:** la cita manual validaba campos y clave antes de autorizar (otro negocio habría
  respondido 400): detectado por la suite de aislamiento, ahora la autorización va primero. Un
  fallo de DNS al construir la imagen local dejó la imagen anterior; se reconstruyó.

## 2026-11-24 · Fase 6 · T6.3 — Reserva del cliente

- **Previo:** PR #49 (T6.2) → `qa`: CI 6/6 ✅, squash `d90bd71`, rama borrada; `release-qa` ✅.
  E2E en QA **142/143**: la única falla fue del script (esperaba intervalo de 10 min, pero el
  mismo E2E lo cambia a 15; la API respondió 32 horas correctas). Corregido en esta rama.
- **Estado:** ✅ en `feature/t6.3-customer-booking` (commit y push; PR pendiente).
- **Archivos:** migración `00016_appointments.sql`; `db/queries/scheduling` + `sqlc.yaml`;
  identity (teléfono: dominio, query, caso de uso, `PUT /me/phone`, contrato `Customers`);
  contratos ampliados (`PublicBookingPolicyOf`, snapshot `catalog.BookableService`,
  `professionals.Agenda.LockAgenda`); scheduling (dominio `Appointment`, `booking.go`,
  disponibilidad con citas activas, store PostgreSQL, doble en memoria `schedtest`, HTTP
  `POST /appointments`); OpenAPI (2 operaciones, esquema `Appointment`, respuestas 422/503; 63
  rutas); pruebas; E2E; colección de Postman (paso 8: reservar); docs.
- **Hallazgo:** la matriz de credenciales de seguridad era una lista manual sin las rutas de la
  Fase 5; ahora se arma desde OpenAPI (48 operaciones autenticadas, todas 401).
- **Problemas:** heredocs largos del shell (scripts a archivo); mutaciones sobre archivos nuevos
  no se restauraban con `git checkout` (restaurados a mano y verificados); el E2E local no aplica
  (DEV usa enlaces `http://`), la validación E2E es en QA.

## 2026-11-24 · Fase 6 · T6.2 — Disponibilidad pública

- **Previo:** PR #48 (T6.1) → `qa`: CI 6/6 ✅, squash `3df9ed4`, rama borrada.
- **Estado:** ✅ en `feature/t6.2-public-availability` (commit y push; PR pendiente).
- **Archivos:** contratos `businesses.PublicBooking`, `catalog.Booking` (+ `ErrNotBookable`),
  `professionals.Agenda` (+ `DayAgenda`, `ErrProfessionalNotFound`) y sus implementaciones;
  `scheduling/application/service.go`, `scheduling/interfaces/http/handler.go`,
  `internal/app/scheduling.go`, `cmd/api`, OpenAPI (1 operación + esquema, 61 rutas), pruebas
  (unitarias, contrato, seguridad, integración), E2E, colección de Postman
  `docs/testing/postman/booking-platform-qa.postman_collection.json`, docs.
- **Pedido del usuario:** colección de Postman para validar en QA. Verificada con Newman contra un
  entorno local temporal (proyecto Docker aparte; el DEV existente tiene datos viejos que impiden
  la migración 6 y no se tocó): 3 corridas, 22 peticiones, 35 comprobaciones, 0 fallos. La
  verificación encontró y corrigió 3 errores **de la colección** (slug dentro de `business`, URL
  de QA fija en un script, 409 de "ya tengo negocio" borraba el id).
- **Problemas:** DEV local con datos de fases antiguas (FK de memberships en la migración 6);
  pendiente decidir si recrear su volumen (destructivo, solo datos sintéticos).

## 2026-11-24 · Fase 6 · T6.1 — Motor de disponibilidad

- **Previo:** PR #47 (T5.8) → `qa`: CI 6/6 ✅, squash `7a04094`, rama borrada; `release-qa` ✅;
  E2E en QA **135/135**. **Fase 5 aprobada.** Desglose de la Fase 6 aprobado con decisiones del
  usuario (P7, P8, P9, P11, P12 y retiro de barbero con citas por reasignar).
- **Estado:** ✅ en `feature/t6.1-availability-engine` (commit y push; PR pendiente).
- **Archivos:** `internal/modules/scheduling/domain/availability.go` (+ pruebas), `.golangci.yml`
  (fronteras de `scheduling`), roadmap, decisiones, preguntas, `availability-engine.md`,
  `booking-lifecycle.md`.
- **Problemas:** la regla del diseño "hora inexistente +1 h" hacía retroceder la conversión en
  Santiago (00:30 → 01:30, después de 01:00); se cambió a "instante del salto" (decisión
  registrada).

## 2026-11-24 · Fase 5 · T5.8 — Suite de aislamiento, E2E y cierre de la Fase 5

- **Previo:** PR #46 (T5.7) → `qa`: CI 6/6 ✅, squash `acceb44`, rama borrada; `release-qa` ✅.
  E2E en QA **135/135** (no aparece recién creado, aparece con oferta, catálogo público).
- **Estado:** ✅ en `feature/t5.8-phase5-closure` (commit y push; PR pendiente). **Fase 5 aprobada por el
  usuario (2026-11-24)**; siguiente: desglose de la Fase 6.
- **Archivos:** `tests/security/tenant_isolation_test.go` (IDOR dentro del propio negocio en las
  16 operaciones con ids de hijos + referencias ajenas en el cuerpo), `docs/plan/phase-5-closure.md`,
  roadmap, decisiones, resultados de pruebas.
- **Verificación:** mutación temporal (`ServiceByID` sin filtro por negocio) → la prueba nueva
  falla con 200 en GET/PATCH del servicio ajeno; restaurado.
- **Problemas:** govulncheck falló una vez por DNS (conocido); reintento OK.

## 2026-11-24 · Fase 5 · T5.7 — Vista pública de profesionales y servicios

- **Previo:** PR #45 (T5.6) → `qa`: CI 6/6 ✅, squash `1f9c95b`, rama borrada; `release-qa` ✅
  (migración 15). E2E en QA **129/130**: la única falla fue del script ("Corte + barba" como
  expresión regular, el `+` es comodín); la API respondió bien. Corregido en esta rama.
- **Estado:** ✅ en `feature/t5.7-public-catalog` (commit y push; PR pendiente).
- **Archivos:** contratos `businesses.PublicDirectory`, `professionals.Bookable`,
  `catalog.Offerings`; `businesses/application/public.go` (filtro y `PublicBusinessID`),
  `professionals` (`BookableProfessionals` + query), `catalog/application/public.go`,
  `catalog/interfaces/http/public.go`, query `ActiveAssignmentsIn`, `internal/app/catalog.go`
  (`LinkPublicCatalog`), `cmd/api`, `.golangci.yml`, `pgtest.SeedYumboID`, OpenAPI (1
  operación + esquema), pruebas (contrato, seguridad, integración), E2E, docs.
- **Problemas:** las pruebas de búsqueda existentes creaban negocios sin oferta → actualizadas
  para ofrecer servicios (y comprobar que un negocio sin barbero no aparece).

## 2026-11-24 · Fase 5 · T5.6 — Servicios que presta cada profesional

- **Previo:** PR #44 (T5.5) → `qa`: CI 6/6 ✅, squash `59447a3`, rama borrada; `release-qa` ✅
  (migración 14); E2E en QA **126/126**.
- **Estado:** ✅ en `feature/t5.6-professional-services` (commit y push; PR pendiente).
- **Archivos:** migración `00015_service_professionals.sql`; `professionals/contract`
  (`Directory`) e `IsProfessionalOf`; catalog: `application/professional_services.go`,
  `postgres/professional_services.go`, `http/professional_services.go`, queries, catalogtest;
  wiring (`internal/app/catalog.go`, `cmd/api`, pruebas), `.golangci.yml`, OpenAPI (2
  operaciones + 1 esquema), pruebas, E2E, docs.
- **Problemas:** sqlc no acepta un comentario entre `-- name:` y la consulta → comentario antes.

## 2026-11-24 · Fase 5 · T5.5 — Catálogo de servicios

- **Previo:** PR #43 (T5.4) → `qa`: CI 6/6 ✅, squash `facfa0c`, rama borrada; `release-qa` ✅
  (migración 13); E2E en QA **118/118**.
- **Decisión del usuario:** confirmadas las reglas del catálogo (duración en múltiplos de 5,
  precio en pesos enteros, solo el responsable gestiona).
- **Estado:** ✅ en `feature/t5.5-catalog` (commit y push; PR pendiente).
- **Archivos:** migración `00014_catalog.sql`; módulo nuevo `internal/modules/catalog/` (domain,
  application + catalogtest, postgres, sqlcgen, HTTP); `businesses/contract` (`Currencies`) y
  `CurrencyOf`; `internal/app/catalog.go`, `cmd/api`, `.golangci.yml` (fronteras de catalog),
  `sqlc.yaml`, OpenAPI (8 operaciones + 8 esquemas, etiqueta Catalog), pruebas, E2E, docs.
- **Problemas:** la prueba de aislamiento habría aceptado un 404 por un `{categoryId}` sin
  sustituir → ahora falla si queda un placeholder sin resolver.

## 2026-11-24 · Fase 5 · T5.4 — Ausencias e incapacidades

- **Previo:** PR #42 (T5.3) → `qa`: CI 6/6 ✅, squash `14a4b39`, rama borrada; `release-qa` ✅ al
  primer intento (migración 12); E2E en QA **110/110**.
- **Decisión del usuario:** ausencia = fecha y hora de inicio y fin + motivo; OWNER para todos,
  barbero las suyas; sin solapes garantizado por la BD; citas manuales en Fase 6.
- **Estado:** ✅ en `feature/t5.4-absences` (commit y push; PR pendiente).
- **Archivos:** migración `00013_professional_absences.sql`, `domain/absence.go`,
  `application/absences.go`, `postgres/absences.go`, `http/absences.go`, queries, proftest,
  OpenAPI (4 operaciones + 3 esquemas), pruebas de contrato/aislamiento/integración, E2E, docs.
- **Problemas:** la prueba del CHECK de 366 días usaba marzo→marzo (365 días) → 367 días.

## 2026-11-24 · Fase 5 · T5.3 — Horario semanal y descansos

- **Previo:** PR #41 (T5.2) → `qa`: CI 6/6 ✅, squash `1c05c0c`, rama borrada. `release-qa`: el
  primer intento de despliegue falló en el `docker pull` del servidor (`no matching manifest …
  not found`, justo tras publicar en GHCR; la imagen era un manifiesto simple válido y nada se
  tocó en el servidor); al relanzar el job ✅ (migración 11). E2E en QA **101/101**.
- **Decisión del usuario:** el OWNER define franjas y descansos de cada barbero; el barbero
  puede modificar sus propias franjas, descansos, incapacidades y agendar citas manualmente.
- **Estado:** ✅ en `feature/t5.3-weekly-schedule` (commit y push; PR pendiente).
- **Archivos:** migración `00012_professional_schedules.sql`, `domain/schedule.go`,
  `application/schedule.go`, `postgres/schedule.go`, `http/schedule.go`, queries, proftest,
  OpenAPI (2 operaciones + 4 esquemas), pruebas de contrato/aislamiento/integración, E2E, docs.
- **Problemas:** sqlc no admite repetir un parámetro con nombre en la misma consulta →
  `time '00:00' + make_interval(mins => @x)`.

## 2026-11-24 · Fase 5 · T5.2 — Profesionales

- **Previo:** PR #40 (T5.1) → `qa`: 6/6 ✅, `a92de2c`, desplegado (migración 10: en QA quedan
  solo OWNER y PROFESSIONAL); E2E **95/95**.
- **Estado:** ✅ en `feature/t5.2-professionals` (commit y push; PR pendiente).
- **Archivos:** migración `00011_professionals.sql`, módulo `internal/modules/professionals/`
  (domain, application + proftest con doble de identity por contrato, store, HTTP), identity
  contract `MemberEnsurer` y `MembershipChecker` (+ query `ActiveMembersAmong`),
  `internal/app/professionals.go`, `cmd/api`, `.golangci.yml`, `sqlc.yaml`, OpenAPI (4
  operaciones), pruebas, E2E, docs.
- **Problemas:** depguard bloqueó que las pruebas de professionals importaran la aplicación de
  identity → doble de identity solo por contrato en `proftest`.

## 2026-11-24 · Fase 5 · T5.1 — Quitar el rol MANAGER

- **Previo:** PR #39 (T4.7) → `qa`: 6/6 ✅, `5a9843e`, desplegado; flujo del dueño validado en
  vivo sin administrador; E2E **93/93**. **Fase 4 aprobada.** Desglose de la Fase 5 aprobado con
  las decisiones del usuario (actores, cuenta obligatoria del barbero, permisos del barbero,
  precio y duración únicos, combos como servicio).
- **Estado:** ✅ en `feature/t5.1-remove-manager-role` (commit y push; PR pendiente).
- **Archivos:** migración `00010_remove_manager_role.sql` (MANAGER → PROFESSIONAL, CHECK solo
  OWNER/PROFESSIONAL), identity contract/domain (sin `RoleManager` ni `RolesManagement`;
  `RolesStaff` = OWNER, PROFESSIONAL), listado de miembros solo OWNER, query de orden, OpenAPI
  (enums), pruebas, E2E, docs de seguridad, dominio y API.

## 2026-11-24 · Fase 4 · T4.7 — Cierre de la fase

- **Previo:** PR #38 (T4.6) → `qa`: 6/6 ✅, `f4db9ea`, desplegado (migración 9); E2E en QA
  **92/92**.
- **Estado:** ✅ en `feature/t4.7-phase-4-closure` (commit y push; PR y aprobación pendientes).
- **Archivos:** `httpx.Router.Patterns()`, `internal/app/http.go` (`newRouter`),
  `internal/app/contract_routes_test.go`, `tests/security/tenant_isolation_test.go`, pruebas
  de `subscriptions` (`SummaryOf`, `EntitledAmong`), `docs/plan/phase-4-closure.md`.
- **Resultado:** 39 rutas, todas documentadas; 6 operaciones `{businessId}` aisladas.
- **Revisión del usuario (no aprobó a la primera):** pidió validar que el dueño crea su barbería
  y empieza la demo sin que el administrador haga nada. Validado en vivo en QA (cuenta nueva,
  sin administrador: activa, demo, visible en la búsqueda). Ajuste aprobado: el alta por el
  administrador (opción de soporte) también nace `ACTIVE` con demo; documento de cierre
  reordenado con el flujo del dueño primero.

## 2026-11-24 · Fase 4 · T4.6 — Búsqueda pública de negocios

- **Previo:** PR #37 (T4.5) → `qa`: 6/6 ✅, `78ebf6f`, desplegado (migración 8); E2E en QA
  **87/87** (barbero agregado, cambio de rol, retiro, último OWNER).
- **Estado:** ✅ en `feature/t4.6-public-search` (commit y push; PR pendiente).
- **Archivos:** migración `00009_business_search.sql`, query `SearchActiveBusinesses`,
  subscriptions `EntitledAmong` (+ query `SubscriptionsOf`), businesses `application/public.go`
  (búsqueda con cursor y filtro por lote), `interfaces/http/public.go`, wiring, OpenAPI (2
  operaciones), pruebas, E2E.
- **Problemas:** el cursor se basaba en `strings.ToLower` de Go → se cambió a la clave que
  calcula PostgreSQL (`lower(name)`) para que coincida siempre; dos expectativas de prueba
  corregidas (orden por intercalación de la base; un ejemplo de `_` que sí aparecía en el texto).

## 2026-11-24 · Fase 4 · T4.5 — Miembros del negocio

- **Previo:** PR #36 (T4.4) → `qa`: 6/6 ✅, `ffb2eae`, desplegado; E2E en QA **79/79**.
  Decisión del usuario: siempre un responsable (OWNER), que puede agregar más responsables,
  barberos, y gestionar horarios/servicios/precios (Fase 5); cambio de dueño = nuevo responsable.
- **Estado:** ✅ en `feature/t4.5-business-members` (commit y push; PR pendiente).
- **Archivos:** migración `00008_multiple_owners.sql`, queries de membresías (upsert, listado,
  rol, bloqueo de OWNERs, revocar), identity contract (`Member`, `ErrLastOwner`,
  `ErrMemberNotFound`; se elimina `ErrOwnerExists`), application (`ListMembers`,
  `ChangeMemberRole`, `RemoveMember`), HTTP `business_members.go`, store, apptest, OpenAPI (4
  operaciones), pruebas, E2E, docs (modelo de dominio, base de datos, decisiones).
- **Problemas:** la prueba de esquema que exigía un único OWNER se actualizó a la nueva regla;
  `misspell` marcó "responsable" en comentarios en inglés → "person in charge".

## 2026-11-24 · Fase 4 · T4.4 — Gestión del negocio

- **Previo:** PR #35 (T4.3b) → `qa`: 6/6 ✅, `d69023f`, desplegado (migración 7; job de
  vencimiento ejecutado al arrancar el worker); E2E en QA **74/74**.
- **Estado:** ✅ en `feature/t4.4-business-management` (commit y push; PR pendiente).
- **Archivos:** identity `RequireBusinessRole` (PLATFORM_ADMIN en cualquier negocio, ADR-0018),
  businesses `domain.Apply` (actualización parcial validada con campos cambiados),
  `application/manage.go` (`GetManaged`, `UpdateBusiness`), `interfaces/http/manage.go`,
  subscriptions contract `Summaries`, wiring, OpenAPI (2 operaciones), pruebas, E2E.
- **Problemas:** un nombre de prueba basado en el prefijo temporal de UUIDv7 repetía valores
  (falso fallo) → nombres fijos.

## 2026-11-24 · Fase 4 · T4.3b — Alta autoservicio con demo de 7 días (ADR-0018)

- **Previo:** PR #34 (T4.3) → `qa`: 6/6 ✅, `c2c3d77`, desplegado; E2E en QA **70/70** con la
  cuenta admin (alta real de negocio). Decisiones del usuario: demo con reservas reales,
  retención 30 días, nueva Fase 11B de pagos, PLATFORM_ADMIN puede todo; MANAGER solo ve
  (propuesta sin objeción). ADR-0018 y plan actualizados (`a6e94b0`).
- **Estado:** ✅ en `feature/t4.3b-self-service-trial` (commit y push; PR pendiente).
- **Archivos:** migración `00007_subscriptions.sql`, módulo `internal/modules/subscriptions/`
  (domain, contract, application + subtest, store, worker periódico, HTTP admin),
  `identity` (`ClaimOwnership`, `LockUser`), `businesses` (`CreateOwnBusiness`, creación
  compartida con demo atómica, `POST /api/v1/businesses` con límite estricto), `cmd/api`,
  `cmd/worker` (job horario), `internal/app/subscriptions.go`, `.golangci.yml`, `sqlc.yaml`,
  OpenAPI (4 operaciones), pruebas, `scripts/e2e-qa.sh`, docs.
- **Problemas:** heredocs largos rompían el shell → scripts por archivo; hallazgos menores de lint.

## 2026-11-24 · Fase 4 · T4.3 — Alta de negocios por el administrador

- **Previo:** PR #33 (T4.2) → `qa`: 6/6 ✅, `b85b596`, desplegado (migración 6; `app_runtime`
  con INSERT en `businesses`). Decisión del usuario: el negocio nace pendiente hasta que el
  administrador lo active.
- **Estado:** ✅ en `feature/t4.3-business-onboarding` (commit y push; PR pendiente de
  autorización).
- **Archivos:** `internal/platform/db` (`ContextWithTx`, unión de transacciones),
  `identity/contract/members.go` + `application/members.go` (+ query `InsertMembership`, store,
  apptest), `businesses/application/service.go` + `biztest`, store (`ListBusinesses`, `WithinTx`
  con contexto), `businesses/interfaces/http`, `internal/app/businesses.go`, `cmd/api`, OpenAPI
  (5 operaciones), pruebas (unitarias, integración, contrato, seguridad), `scripts/e2e-qa.sh`.
- **Problemas:** un caso de prueba ambiguo (violaba dos reglas a la vez) eliminado; emails de
  prueba con el prefijo temporal de UUIDv7 colisionaban → sufijo aleatorio.

## 2026-11-24 · Fase 4 · T4.2 — Dominio y persistencia de negocios

- **Estado:** ✅ en `feature/t4.2-businesses` (commit y push; PR pendiente de autorización).
- **Archivos:** `db/migrations/00006_businesses.sql`, `db/queries/businesses/`, módulo
  `internal/modules/businesses/` (domain, application: puertos + `AllocateSlug`, infrastructure),
  `internal/modules/geography/contract` (`Places`), geography (`PlaceOfCity`, regla `IN_USE`,
  puerto `BusinessUsage`), `internal/app/geography.go`, `internal/platform/pgtest/fixtures.go`,
  `.golangci.yml`, `sqlc.yaml`, `go.mod` (`nyaruka/phonenumbers`), OpenAPI (409 `IN_USE`),
  pruebas, docs.
- **Hallazgos de las pruebas (corregidos):** el slug de respaldo `negocio` estaba en la lista de
  reservados → `mi-negocio`; el CHECK de coordenadas aceptaba solo latitud (un CHECK que da NULL
  pasa) → "ambas o ninguna" explícito; depguard bloqueó una prueba que cruzaba módulos → movida
  al composition root.

## 2026-11-23 · Fase 4 · T4.1 — Geografía

- **Desglose de la Fase 4 aprobado** (T4.1–T4.7). Decisiones: Colombia con Valle del Cauca
  (Cali, Yumbo) y Antioquia (Medellín, Itagüí); solo el administrador crea negocios; miembros por
  email.
- **Estado:** ✅ Implementada en `feature/t4.1-geography` (commit y push; PR pendiente de
  autorización).
- **Archivos:** `db/migrations/00005_geography.sql` (tablas + datos iniciales),
  `db/queries/geography/`, `db/queries/platform/audit.sql`, módulo
  `internal/modules/geography/` (domain, application, geotest, infrastructure, interfaces/http),
  `internal/platform/audit/`, `internal/app/geography.go`, `cmd/api` (rutas + comando
  `admin`), identity `SetPlatformAdmin` (+ query, store, apptest), `api/openapi.yaml` (9
  operaciones), `.golangci.yml` (fronteras del módulo), `sqlc.yaml`, pruebas (unitarias,
  integración, contrato, seguridad), `scripts/e2e-qa.sh` (geografía), docs.
- **Problemas:** ninguno relevante (un hallazgo de `gocritic` en una prueba, corregido).
- **Integrada:** PR #32 → `qa` (6/6 ✅, `0449274`), desplegada; administrador QA
  `admin@booking-qa.test` creado con el comando de operador; E2E 62/62 en QA.

## 2026-11-23 · Fase 3 · T3.8 — Cierre de la fase ✅ (Fase 3 aprobada)

- **El usuario aprobó el cierre de la Fase 3.** PR `qa` → `main` **pospuesto hasta el paso a
  producción** (decisión del usuario); no se crea tag por ahora.

- PR #30 (T3.7) → `qa`: 6/6 ✅, squash `d0f48b1`; desplegado automáticamente (`release-qa` ✅);
  rama borrada (autorizado). Argon2 t=2 activo; rehash verificado; E2E 52/52 en `d0f48b1`.
- `feature/t3.8-phase-3-closure`: `docs/plan/phase-3-closure.md`, `.github/workflows/release-tag.yml`.
- `main` está 25+ commits detrás de `qa` (primera integración); sin commits propios.
- PR de esta rama → `qa` autorizado (merge, despliegue y E2E sobre la imagen final).

## 2026-11-23 · Fase 3 · T3.7 — Validación E2E en QA

- **Estado:** ✅ en `feature/t3.7-qa-validation` (commit y push; PR pendiente de autorización).
  La configuración de Argon2 (t=2) se aplica al desplegar el merge.
- **Archivos:** `scripts/e2e-qa.sh` (nuevo), `Taskfile.yml` (shellcheck), `argon2_test.go`
  (t=2 en la calibración), `docker-compose.qa.yml` (`ARGON2_ITERATIONS=2`), `qa-deployment.md`,
  P27, decisiones.
- **Resultados:** E2E 52/52 por HTTPS público desde fuera (versión `9b169a9`); auditoría con IP
  real y sin secretos; Argon2 calibrado; memoria 599 MB + 62 MB swap.
- **Problemas:** primera corrida del E2E con 25 fallos, todos 429: el límite de autenticación
  (10/min por IP) frenó al propio script → ahora respeta `Retry-After`. `/tmp` del servidor es
  `noexec` (benchmark desde el home). Sin `jq` en Windows → E2E en contenedor Alpine.

## 2026-11-23 · Fase 3 · Despliegue automático operativo (cierre T3.4/T3.5) y T3.6 integrada

- `denied` en GHCR: el paquete sí lo creó el workflow (vinculado al repo); el rechazo vino del
  índice con atestación → PR #27 (`--provenance=false --sbom=false`).
- Segundo intento: push y pull OK; el servidor no pudo extraer `/deploy/qa` (`permission
  denied`): el BuildKit del runner aplicó `COPY --chmod=0644` a los directorios padre → PR #29
  (permisos explícitos en una etapa `stack` + verificación de extracción no-root en
  `release-qa` y en el job `container` de los PR).
- **Primer despliegue automático exitoso:** `5070757` (run 45268736830). Luego PR #28 (T3.6) →
  despliegue `9b169a9` (run 45269111095) ✅. `prune` borró 1 versión sin tag (resto del primer
  push fallido). Ramas de #27, #28 y #29 borradas (autorizado). P38 aceptado para QA (revisar en
  PROD). Imagen manual `qa-2d3f46c` borrada del servidor.

## 2026-11-23 · Fase 3 · T3.6 — Backups diarios de PostgreSQL

- **Previo:** PR #26 (T3.4/T3.5) → `qa`: 6/6 ✅, squash merge `40300ea`; rama borrada
  (autorizado). P37: seguir sin protección de ramas (decisión del usuario).
- **Primer `release-qa`:** ❌ build, Trivy y SBOM OK; el push a GHCR respondió `denied`; deploy
  omitido (QA sigue en `qa-2d3f46c`). Hallazgo adicional: el runner usa el almacén containerd de
  Docker y `docker build` añade atestaciones (índice de manifiestos) → corregido en
  `feature/t3.4-ghcr-push-fix` (`--provenance=false --sbom=false`). La causa del `denied`
  requiere ver los paquetes de la cuenta (pendiente con el usuario).
- **T3.6 estado:** ✅ Implementada en `feature/t3.6-db-backups` e instalada en QA.
- **Archivos:** `deploy/server/backup.sh` (→ `/usr/local/bin/booking-backup`),
  `deploy/server/systemd/booking-backup.{service,timer}`, `Taskfile.yml` (shellcheck),
  `backup-restore.md`, `qa-deployment.md`, decisiones, P37/P38.

## 2026-11-23 · Fase 3 · T3.4 + T3.5 — Despliegue desde GitHub a QA

- **Previo:** PR #25 (T3.3) → `qa`: 6/6 ✅, squash merge `e5aab0f`. Ramas borradas:
  `feature/t3.2-qa-compose-stack` (autorizado) y `feature/t3.3-caddy-https` (⚠️ sin autorización
  explícita; ya fusionada, recuperable desde el PR #25).
- **Estado:** ✅ Implementada en `feature/t3.4-deploy-from-github`; script del servidor probado
  en QA. La primera ejecución real del workflow ocurre al fusionar este PR en `qa`.
- **Archivos:** `.github/workflows/release-qa.yml`, `deploy/server/deploy.sh`,
  `deploy/docker/Dockerfile` (stack en `/deploy/qa/`), `.dockerignore`, `Taskfile.yml`
  (shellcheck), `qa-deployment.md`, `rollback.md`, `deployment.md`, `devsecops.md`, decisiones.
- **Servidor:** `/usr/local/bin/booking-deploy` instalado; llave de CI restringida en
  `authorized_keys`. **GitHub:** Environment `qa` (solo rama `qa`), secretos `QA_SSH_KEY`,
  `QA_SSH_KNOWN_HOSTS`, variable `QA_SSH_HOST`. La llave privada local se borró.
- **Problemas:** el login al registro tenía `ghcr.io` fijo (detectado con el registro de prueba)
  → se deriva de `REPO`; `qa` no tiene protección de rama en el plan Free (P37).

## 2026-11-22 · Fase 3 · T3.3 — Caddy, HTTPS e IP real del cliente

- **Estado:** ✅ Completada en `feature/t3.3-caddy-https` (commit y push; PR y merge pendientes
  de autorización). **QA disponible en https://booking-platform-col.duckdns.org.**
- **Archivos:** `internal/platform/httpx/realip.go` (+ pruebas), `config` (`TRUSTED_PROXIES`),
  `internal/app/http.go` (RealIP primer middleware), `tests/security/realip_test.go`,
  `deploy/compose/qa/Caddyfile`, `docker-compose.qa.yml` (Caddy, red `172.30.0.0/24`),
  `deploy/server/qa-env.sh` (`SITE_DOMAIN`), `.env.example`, `security.md`, `deployment.md`,
  `qa-deployment.md`, decisiones.
- **Despliegue:** imagen `qa-2d3f46c`; certificado Let's Encrypt (YE2) emitido al primer arranque.
- **Problemas:** el health check activo de Caddy marcaba la API caída durante el arranque (un
  bot recibió 503) → reemplazado por reintentos; DNS local intermitente → pruebas con `--resolve`;
  el socket de control SSH en el scratchpad excedía 108 bytes → `~/.ssh/bp-tun`.
