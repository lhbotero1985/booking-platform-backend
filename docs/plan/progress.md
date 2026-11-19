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
