# Resultados de pruebas

Solo resultados **reales** de comandos ejecutados. Formato: fecha · fase/tarea · comando ·
resultado · observaciones.

---

## 2026-11-18 · Fase 0 · T0.2

| Comando | Resultado | Observaciones |
|---|---|---|
| `docker run … gitleaks:v8.24.0 dir /repo` | ⚠️ **No ejecutado** | Docker Desktop no estaba en ejecución (`dockerDesktopLinuxEngine` no encontrado). Repetir con Docker activo o al primer push (workflow `security`). |
| `grep -rniE "(password\|secret\|token)\s*[:=]\s*['\"][^'\"]{8,}"` | ✅ Sin coincidencias | Revisión manual sustituta, no equivale a gitleaks. |

No existen pruebas de código: la Fase 0 no contiene código funcional.

## 2026-11-18 · Fase 0 · T0.4 — Verificación de toolchain

| Comando | Resultado |
|---|---|
| `go version` | ✅ go1.27.4 windows/amd64 |
| `govulncheck -version` | ✅ v1.10.0 |
| `goose --version` | ✅ v3.30.1 |
| `sqlc version` | ✅ v1.31.1 |
| `golangci-lint version` | ✅ 2.15.0 |
| `task --version` | ✅ 3.54.0 |
| gitleaks | ⚠️ Sigue sin ejecutarse (Docker Desktop apagado) |

## 2026-11-18 · Fase 1 · T1.1

| Comando | Resultado | Observaciones |
|---|---|---|
| `golangci-lint config verify` | ✅ exit 0 | |
| `task build` | ✅ | `bin/api.exe`, `bin/worker.exe` con versión inyectada (`999de2e-dirty`) |
| `go vet ./...` | ✅ | |
| `golangci-lint run` | ✅ 0 issues | |
| `go test -count=1 ./...` | ✅ ok `internal/platform/buildinfo` (2 pruebas) | `cmd/*` sin pruebas (solo arranque) |
| `govulncheck ./...` | ✅ No vulnerabilities found | |
| `bin/api.exe` | ✅ exit 0 | Log JSON con versión, commit y Go |
| depguard con import `net/http` en `internal/modules/probe/domain` | ✅ Detectado (`domain-purity`) | Archivo temporal eliminado |
| `go test -race` | ⚠️ No ejecutado | Sin gcc/cgo en Windows; se cubrirá en CI (T1.8) |

## 2026-11-18 · Fase 1 · Cambio de ruta del módulo + escaneo de secretos

| Comando | Resultado | Observaciones |
|---|---|---|
| `task check` (módulo `github.com/lhbotero1985/booking-platform-backend`) | ✅ 0 issues, pruebas ok, sin vulnerabilidades | |
| `gitleaks git` (historial, 4 commits) | ✅ no leaks found | Docker Desktop activo; resuelve el pendiente de T0.2 |
| `gitleaks dir` (directorio de trabajo) | ✅ no leaks found | |

## 2026-11-18 · Fase 1 · T1.2 — Plataforma HTTP base

| Comando | Resultado | Observaciones |
|---|---|---|
| `golangci-lint run` | ✅ 0 issues | 1ª ejecución: 7 hallazgos (noctx, errorlint) y luego 3 (bodyclose) en pruebas; corregidos |
| `go test -count=1 -cover ./...` | ✅ todos ok | Cobertura: app 100 %, buildinfo 100 %, config 98.4 %, httpx 92.9 %, logging 90.9 % |
| `go test` (1ª ejecución) | ❌ → ✅ | `IsSensitiveKey("X-Api-Key")` no se redactaba; se añadió `api-key` |
| `govulncheck ./...` | ✅ No vulnerabilities found | 1er intento falló por DNS (`lookup vuln.go.dev`); reintento ok |
| `APP_ENV=staging bin/api.exe` | ✅ exit 1 con error claro | Falla rápido con config inválida |
| Smoke: `GET /healthz` | ✅ 200 `application/json` `{"status":"ok",...}` | Binario real en 127.0.0.1:18080 |
| Smoke: `GET /nope` | ✅ 404 `application/problem+json` | Access log JSON con request_id, sin query string |
| `go test -race` | ⚠️ No ejecutado | Sin cgo en Windows; CI (T1.8) |
| Apagado ordenado (SIGTERM) | ⚠️ No verificado en Windows | Se verificará en contenedor Linux (T1.3) |

## 2026-11-19 · Fase 1 · T1.3 — Docker y Docker Compose (DEV)

| Comando | Resultado | Observaciones |
|---|---|---|
| `task up` (1er intento) | ❌ | DNS: `lookup gcr.io: no such host`; imágenes base descargadas con reintentos |
| `task up` (2º intento) | ❌ | Puerto 5432 ocupado por PostgreSQL nativo (pid 8052); DEV movido a 55432 |
| `task up` (3er intento) | ✅ exit 0 | postgres `healthy`, api `Started` |
| `GET http://127.0.0.1:8080/healthz` | ✅ 200 `{"status":"ok",...}` | Desde el host contra el contenedor |
| `docker inspect` api | ✅ `User=nonroot`, `ReadonlyRootfs=true`, `CapDrop=[ALL]`, `no-new-privileges` | |
| Tamaño de imagen | ✅ 12.4 MB | distroless/static-debian12 |
| PostGIS | ✅ PostgreSQL 17.5, PostGIS 3.5.2 | `create extension postgis` ok |
| `docker compose stop api` (SIGTERM) | ✅ "api shutting down" → "api stopped cleanly", ExitCode 0 | Verifica apagado ordenado en Linux |
| `trivy config /deploy` (HIGH,CRITICAL) | ✅ exit 0, sin hallazgos | Trivy 0.58.1 |
| `trivy image booking-platform-backend:dev` | ✅ Total: 0 (HIGH: 0, CRITICAL: 0) | 1er intento falló al bajar la DB (IPv6 inalcanzable a mirror.gcr.io); se usa `ghcr.io/aquasecurity/trivy-db` |
| `task check` | ✅ 0 issues, pruebas ok, sin vulnerabilidades | |
| `gitleaks dir` | ✅ no leaks found | Incluye `.env` local (ignorado por Git) |

## 2026-11-19 · Fase 1 · T1.4 — Base de datos, migraciones y readiness

| Comando | Resultado | Observaciones |
|---|---|---|
| `golangci-lint run` (incluye build tag `integration`) | ✅ 0 issues | 1ª ejecución: 3 × gosec G101 por URLs sintéticas en tests → exclusión acotada a `_test.go` |
| `go test -count=1 ./...` (unitarias) | ✅ todos ok | Nuevos: config de BD (incl. no filtrar credenciales), mapeo de errores PG, readyz (ok / 503 sin internals / timeout 2 s) |
| `govulncheck ./...` | ✅ No vulnerabilities found | 1er intento falló por DNS |
| `go test -tags=integration ./...` (1ª) | ❌ | `TestUpDownUp`: `cannot drop extension postgis because other objects depend on it` — la imagen postgis preinstala postgis_topology/tiger. Corregido: el Down no elimina postgis |
| `go test -tags=integration ./...` (2ª) | ✅ 7 paquetes ok | PostgreSQL/PostGIS real vía testcontainers |
| ↳ migrate `TestUpDownUp` | ✅ | pending → up → extensiones presentes → down total → pending → up |
| ↳ db `WithTx` commit / rollback por error / rollback por pánico | ✅ | |
| ↳ db exclusion constraint (btree_gist + tstzrange) | ✅ | Solape mismo profesional → 23P01; rangos contiguos y otro profesional → ok (mecanismo ADR-0004) |
| ↳ db unique con citext | ✅ | `Ana@Example.com` vs `ana@example.COM` → 23505 |
| ↳ db `statement_timeout` y `application_name` | ✅ | 15s / db-test |
| ↳ app `TestReadyzAgainstRealDatabase` | ✅ | 503 con migraciones pendientes → 200 tras `Up` → 503 con BD detenida |
| `docker build` | ✅ al 4º intento | DNS de Docker (`auth.docker.io`, `registry-1.docker.io`) |
| E2E compose: `GET /readyz` antes de migrar | ✅ 503 | log: `check=migrations error="migrate: pending migrations"` |
| E2E: `api migrate status` / `api migrate up` | ✅ | 00001 pending → applied |
| E2E: `GET /readyz` después de migrar | ✅ 200 `{"status":"ready"}` | |
| `APP_ENV=qa api migrate down` | ✅ rechazado | `migrate down is not allowed in qa`, exit 1 |
| Contraseña de BD en logs de la API | ✅ no aparece | |

## 2026-11-19 · Fase 1 · T1.5 — SQLC

| Comando | Resultado | Observaciones |
|---|---|---|
| `sqlc generate` (1ª) | ❌ | `field x-go-defaults not found`: sqlc no admite anclas YAML con claves propias → overrides globales |
| `sqlc generate` (2ª) | ✅ | Paquete `internal/platform/db/platformq` (sqlc v1.31.1, pgx/v5) |
| `task sqlc:check` (`sqlc vet` + `sqlc diff`) | ✅ exit 0 | |
| `sqlc diff` con consulta modificada a propósito | ✅ exit 1 | Detecta código generado desactualizado; consulta restaurada (sin diff) |
| `task check` | ✅ 0 issues, pruebas ok, sin vulnerabilidades | El código generado queda excluido del lint (cabecera "Code generated") |
| `go test -tags=integration ./...` | ✅ 7 paquetes ok | Nuevo `TestSQLCGeneratedQuery`: la consulta generada lista postgis, btree_gist y citext en PG real |
