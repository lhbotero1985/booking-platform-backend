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

## 2026-11-19 · Fase 1 · T1.6 — River (jobs en segundo plano)

| Comando | Resultado | Observaciones |
|---|---|---|
| `sqlc diff` (1ª) | ❌ → ✅ | Detectó modelos River nuevos en `platformq` al entrar las tablas al esquema; solución `omit_unused_structs: true` |
| `golangci-lint run` (1ª) | ❌ → ✅ 0 issues | misspell: "cancelled" → "canceled" en comentarios/logs |
| `task check` | ✅ 0 issues, pruebas ok, sin vulnerabilidades | |
| `go test -tags=integration` jobs+migrate | ⚠️ intermitente | `failed to create Docker provider` (testcontainers + Docker Desktop en Windows con paquetes concurrentes). Mitigado con reintentos en `pgtest.Start` |
| `go test -tags=integration ./...` × 4 corridas | ✅ 4/4, 8 paquetes ok | Tras los reintentos |
| ↳ migrate `TestUpDownUp` con `00002_river.sql` | ✅ | Up/Down/Up de todo el esquema River |
| ↳ jobs `TestInsertTxCommittedJobIsProcessed` | ✅ | Job insertado con `InsertTx` + commit se procesa |
| ↳ jobs `TestInsertTxRolledBackJobIsNeverProcessed` | ✅ | Rollback → 0 filas en `river_job`, nunca procesado (outbox, ADR-0008) |
| ↳ jobs `TestUniqueByArgsSkipsDuplicates` | ✅ | 2º insert `UniqueSkippedAsDuplicate`, 1 fila |
| ↳ jobs `TestPingWorkerCompletes` | ✅ | Registro de workers de plataforma, cola `maintenance` |
| ↳ Stop graceful del cliente en cada prueba | ✅ | `client.Stop` sin error |
| `docker build` | ✅ 2º intento | DNS |
| E2E: worker con migración 00002 pendiente | ✅ | `pending migrations: run api migrate up…`, reinicios del contenedor |
| E2E: `api migrate up` | ✅ | `00002_river.sql` aplicada |
| E2E: worker tras migrar | ✅ `worker started`; `/readyz` 200 | |
| E2E: job `platform.ping` insertado por SQL | ✅ `completed`, attempt 1 | Log `ping job processed` |
| E2E: SIGTERM al worker | ✅ `worker shutting down` → `worker stopped cleanly`, exit 0 | |
| Contraseña de BD en logs (api + worker) | ✅ no aparece | |
| gitleaks | ✅ no leaks found | |

## 2026-11-19 · Fase 1 · T1.7 — Contrato OpenAPI 3.1

| Comando | Resultado | Observaciones |
|---|---|---|
| `TestSpecIsValidOpenAPI31` | ✅ | Spec 3.1.0 válido contra el esquema OpenAPI (libopenapi v0.43.1 / validator v0.16.3) |
| `TestSpecConventions` | ✅ | operationId único, tags, errores 4xx/5xx con `application/problem+json` |
| `TestContractOperations` (1ª) | ❌ → ✅ | `POST /healthz 405` no se puede declarar en la operación GET → 404/405 pasan a ser respuestas genéricas documentadas en `info.description` |
| `TestContractOperations` | ✅ | `GET /healthz` 200, `GET /readyz` 200 y 503 conformes (status, headers, content-type, body) |
| `TestContractGenericErrorsAreProblems` | ✅ | 404 y 405 (POST /healthz, DELETE /readyz) son `Problem` válidos con `application/problem+json` |
| `TestContractDetectsViolations` | ✅ | Verificación negativa: body inválido en /healthz y Problem inválido son rechazados |
| `task openapi:lint` | ✅ exit 0 | |
| `golangci-lint run` | ✅ 0 issues | |
| `govulncheck ./...` | ✅ No vulnerabilities found | 1er intento falló por DNS |
| `go test -tags=integration ./...` (sin DOCKER_HOST) | ⚠️ 1 fallo en 4 corridas | `failed to create Docker provider` (detección automática de testcontainers en Windows bajo concurrencia) |
| `task test:integration` (DOCKER_HOST explícito del contexto Docker) × 6 | ✅ 6/6, 9 paquetes ok | Causa raíz corregida |
| gitleaks | ✅ no leaks found | |

## 2026-11-19 · Fase 1 · T1.8 — GitHub Actions (validación local previa)

| Comando | Resultado | Observaciones |
|---|---|---|
| `actionlint` 1.7.7 (incluye shellcheck) sobre `.github/workflows/*` | ✅ exit 0 | |
| gitleaks v8.30.1 (digest fijado) sobre historial completo | ✅ no leaks found | Misma imagen que CI |
| Trivy 0.76.2 `config /deploy` | ✅ 0 misconfigurations | Misma imagen que CI |
| Trivy 0.76.2 `image` (build actual) | ✅ debian 0, `api` 0, `worker` 0 | HIGH/CRITICAL con fix |

## 2026-11-19 · Fase 1 · T1.8 — GitHub Actions (ejecución real)

PR #9 (`feature/t1.8-ci` → `qa`) y push a `qa` tras el merge (`bbf7626`).

| Workflow / job | PR #9 | `qa` (push) |
|---|---|---|
| ci / build + unit tests (race) | ✅ 3m31s | ✅ |
| ci / integration tests (PostgreSQL) | ✅ 3m03s | ✅ |
| ci / lint (golangci-lint, sqlc) | ✅ 3m52s | ✅ |
| security / secrets (gitleaks, historial completo) | ✅ | ✅ |
| security / dependencies (govulncheck) | ✅ | ✅ |
| security / container (trivy config + image) | ✅ | ✅ |

- Primera ejecución real de `-race` (unitarias e integración): sin carreras detectadas.
- Cobertura unitaria en CI: total 52.2 % (app 100 %, buildinfo 100 %, config 98.7 %, httpx 92.9 %,
  logging 90.9 %, db 38.3 % — el resto de db lo cubre integración).

## 2026-11-19 · Fase 1 · T1.9 — Seguridad básica y cierre

| Comando | Resultado | Observaciones |
|---|---|---|
| `task test:security` (1ª) | ❌ → ✅ | `TestOversizedHeadersRejected`: 64 KiB + 1 KiB devolvió 200 — net/http admite MaxHeaderBytes + 4096 de holgura; la prueba usa ahora +8 KiB y documenta el límite efectivo (~68 KiB) |
| `task test:security` | ✅ 7 pruebas | Cabeceras en 200/404/405/TRACE/413/503; sin fugas; CORS solo lista blanca exacta; body chunked; inyección vía X-Request-ID; 431; slowloris cortado en ~1 s |
| `golangci-lint run` (1ª) | ❌ → ✅ 0 issues | noctx: `net.Listen`/`net.Dial` → `ListenConfig`/`DialContext` |
| `task check` (unitarias + lint + build + vet) | ✅ | |
| `govulncheck ./...` | ✅ No vulnerabilities found | 1er intento falló por DNS |
| `task test:integration` | ⚠️ No verificable localmente | Docker Desktop detenido (`open //./pipe/dockerDesktopLinuxEngine`); se valida en el CI del PR |

## 2026-11-19 · Fase 1 · T1.9 — validación final

| Comando | Resultado |
|---|---|
| `task test:integration` (Docker Desktop reactivado) | ✅ 10 paquetes ok |
| gitleaks v8.30.1 (historial) | ✅ no leaks found |
| PR #10 — ci (test/race, integración, lint) + security (gitleaks, govulncheck, trivy) | ✅ 6/6; squash merge `1caba7d` |

## 2026-11-19 · Fase 2 · T2.1 — Esqueleto del módulo identity

| Comando | Resultado | Observaciones |
|---|---|---|
| `sqlc generate` / `sqlc diff` | ✅ | Paquete `identity/infrastructure/sqlcgen` (users, sessions, memberships, audit) |
| `go vet -tags=integration ./...` / `golangci-lint run` | ✅ 0 issues | |
| depguard con 4 violaciones intencionales | ✅ 4/4 detectadas | domain→net/http (`domain-purity`), domain→application (`identity-domain-inward`), application→pgx (`application-layer`), identity→otro módulo (`identity-module-boundary`); import válido interfaces→application no marcado. Archivos temporales eliminados |
| Integración identity (PostgreSQL real) | ✅ 7 pruebas | email citext único; CHECKs (status, E.164, hash PHC argon2id, nombre); token SHA-256 (32 bytes); sesión activa respeta expiración por inactividad y revocación; `TouchSession` no supera la expiración absoluta; cascada usuario→sesiones; un OWNER activo por negocio; membresía única; rol inválido; formato de acción de auditoría |
| `TestUpDownUp` con 00003 | ✅ | |
| `task test:integration` | ✅ 11 paquetes ok | |
| `govulncheck ./...` | ✅ No vulnerabilities found | 1er intento falló por DNS |

## 2026-11-19 · Fase 2 · T2.1 — CI

| Comando | Resultado |
|---|---|
| PR #11 — ci + security | ✅ 6/6; squash merge `a305183` |

## 2026-11-19 · Fase 2 · T2.2 — Dominio de identidad y Argon2id

| Comando | Resultado | Observaciones |
|---|---|---|
| `go test -cover ./internal/modules/identity/...` | ✅ domain 100 %, argon2 95.8 % | Meta del dominio ≥ 90 % cumplida |
| Pruebas del dominio | ✅ | Email (normalización, display names, dominios inválidos, longitudes), política de contraseña (runas, blocklist sin mayúsculas, carácter repetido), `Password` nunca se imprime (fmt `%v %+v %#v %s %q`, struct anidado, slog JSON, json.Marshal), invariantes de `User`, ciclo suspender/reactivar, estados y roles |
| Pruebas de Argon2id | ✅ | Formato PHC, sales únicas, verificación correcta/incorrecta, `needsRehash` al subir parámetros, 16 hashes malformados rechazados **sin ejecutar el KDF** (< 50 ms, incl. "memory bomb" de 4 GiB), parámetros fuera de rango, contraseña vacía, semáforo de concurrencia (máx. 2 observado), cancelación por contexto, `VerifyDummy` con tiempo comparable a `Verify` (ratio 0.5–2) |
| `go test -fuzz FuzzParseEmail -fuzztime 30s` | ✅ PASS | Emails aceptados siempre normalizados, acotados y estables al re-parsear |
| `go test -fuzz FuzzDecode -fuzztime 30s` | ✅ PASS | Ningún hash aceptado fuera de los límites de costo |
| Benchmark Argon2id, 64 MiB, p=2 (equipo de desarrollo, 16 CPU) | t=3: 71 ms · t=4: 88 ms · t=5: 108 ms · t=6: 129 ms | 67 MB/op. Default t=3; calibración final en el Droplet QA (Fase 3) vía `ARGON2_ITERATIONS` |
| `golangci-lint run` | ✅ 0 issues | 1ª: revive (comentarios en accesores), gosec G115 en prueba → corregidos |
| `govulncheck ./...` | ✅ No vulnerabilities found | 1er intento falló por DNS |
