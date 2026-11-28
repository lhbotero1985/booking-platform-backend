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

## 2026-11-19 · Fase 2 · T2.2 — CI

| Comando | Resultado |
|---|---|
| PR #12 — ci + security | ✅ 6/6; squash merge `ebc7f98` |

## 2026-11-19 · Fase 2 · T2.3 — Registro y verificación de email

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias dominio | ✅ 100 % | `VerifyEmail` (idempotente, rechaza suspendidos), `Token` (forma, hash ≠ raw, prefijo de tipo, nunca impreso) |
| Unitarias aplicación (10 casos) | ✅ 76.4 % | Pendiente + job (sin envío inline); validación por campo; **sin enumeración** (mismo resultado, hash en ambos caminos, reenvío solo si pendiente); flujo completo; un solo uso; rotación invalida el token anterior; expiración 24 h; basura rechazada; suspendido no verifica; fallo SMTP reintentable; rollback |
| Handlers HTTP | ✅ 88.6 % | 202 idéntico para email nuevo/existente; 400 con `errors[]`; 415 para formularios; campo `isPlatformAdmin` rechazado (mass assignment); 500 sin internals; contraseña y token ausentes de logs |
| `httpx.DecodeJSON` exige `application/json` | ✅ | 6 Content-Type no JSON → 415 |
| Mailer SMTP | ✅ | Mensaje RFC 5322 UTF-8 QP; inyección de cabeceras (CRLF en asunto) rechazada |
| Config SMTP / PUBLIC_APP_URL | ✅ 98.9 % | QA/PROD exigen TLS y https; usuario sin contraseña rechazado |
| Contrato OpenAPI (peticiones y respuestas) | ✅ | register 202/400/415, resend 202, verify 200/400 |
| Integración PostgreSQL (`postgres` store) | ✅ 3 pruebas | Job encolado en la misma transacción; reenvío coalescido (1 job); args del job sin token; BD guarda solo el hash; verificación activa la cuenta; reuso y expiración rechazados; registro duplicado silencioso |
| E2E Docker Compose + Mailpit | ✅ | register 202 ×2 (idénticos) → 1 email en Mailpit (asunto en español, `bp_ev_`, 49 chars) → `PENDING_VERIFICATION` → verify 200 → `ACTIVE` + `email_verified_at`; reuso 400 `INVALID_TOKEN`; job `completed` en 1 intento; token y contraseña **0** veces en logs de api y worker |
| gitleaks: regla `bp_(s\|ev\|pr)_` | ✅ | Detecta un token falso de prueba (archivo temporal); repo sin fugas |
| `task test:integration` | ✅ 19 paquetes | |
| `golangci-lint` / govulncheck / `sqlc diff` | ✅ | govulncheck 1er intento falló por DNS |

## 2026-11-19 · Fase 2 · T2.3 — CI y ajuste QA

| Comando | Resultado |
|---|---|
| Config: `SMTP_TLS=none` aceptado en QA (Mailpit), rechazado en PROD | ✅ |
| PR #13 — ci + security | ✅ 6/6; squash merge `969d6eb` |

## 2026-11-19 · Fase 2 · T2.4 — Login, sesiones y autenticación

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias dominio (`Session`, políticas) | ✅ 100 % | Móvil 30 d/90 d, web 12 h/7 d, admin 1 h/12 h; touch con throttling de 5 min; idle nunca supera el absoluto |
| Unitarias aplicación (10 casos de sesión) | ✅ 81.6 % | Login móvil/web/admin; email inexistente, contraseña incorrecta y cuenta suspendida → mismo error (+`VerifyDummy`); no verificada → `EMAIL_NOT_VERIFIED` solo con contraseña correcta; validación; rechazo de tokens vacíos/malformados/de otro tipo/desconocidos/expirados; sliding hasta el absoluto (1ª ejecución: error del propio test, 15×11 h < 7 días → corregido a 20); suspensión bloquea sesiones; logout solo la actual; logout-all todas |
| Handlers HTTP (8 pruebas) | ✅ 86.5 % | Token móvil en body; web solo en cookie `__Host-session` (HttpOnly, Secure, SameSite=Lax, Path=/, sin Domain); `/me` sin hash; 401 idénticos (salvo requestId); matriz 401 (6 casos, sin fallback a cookie con esquema no Bearer); CSRF (sin Origin, ajeno, look-alike → 403; permitido → 204 y cookie borrada); logout-all `{"revoked":2}`; tokens fuera de logs |
| Contrato OpenAPI | ✅ | login 200 (MOBILE/WEB)/401/403, `/me` 200/401, logout-all 200, logout 401 tras revocar |
| `TestLoginTimingDoesNotRevealAccounts` (Argon2id real) | ✅ ratio 0.99–1.02 aislado | 1ª corrida con la suite en paralelo falló por medir series secuenciales bajo carga → muestras intercaladas (n=15, medianas); 4/4 corridas de la suite completa en verde |
| Integración PostgreSQL (sesiones) | ✅ 5 pruebas | IP/UA guardados, token en claro no; throttling de `last_seen`; suspensión inmediata; logout-all; expiración por inactividad |
| E2E Docker Compose (17 pasos) | ✅ | 202 → 403 `EMAIL_NOT_VERIFIED` → verify 200 → 401 `INVALID_CREDENTIALS` → login MOBILE 200 (48 chars) → `/me` 200 → sin token 401 → login WEB (cookie endurecida, sin token en body) → `/me` cookie 200 → CSRF 403 ×2 → logout 204 → 401 → logout-all `{"revoked":1}` → 401; secretos en logs: 0; BD: 0 activas / 2 |
| `golangci-lint` | ✅ 0 issues | G101 falso positivo en `CodeInvalidCredentials` anotado |
| govulncheck / `sqlc diff` / gitleaks | ✅ | |

## 2026-11-19 · Fase 2 · T2.4 — CI

| Comando | Resultado |
|---|---|
| PR #14 — ci + security | ✅ 6/6; squash merge `7264966` |

## 2026-11-19 · Fase 2 · T2.5 — Recuperación y cambio de contraseña

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias aplicación (6 casos nuevos) | ✅ 81.4 % | Forgot silencioso (solo cuentas activas reciben email); reset revoca todas las sesiones, invalida la clave vieja, un solo uso, aviso programado; política verificada **antes** de consumir el token; rotación, expiración 30 min, token de otro tipo rechazado; cuenta suspendida no restablece ni recibe email; cambio autenticado (clave actual incorrecta/vacía, débil, sin cambio) conserva la sesión actual y revoca las demás |
| Handlers HTTP | ✅ 85.5 % | forgot idéntico existente/inexistente; reset 400 débil → 200 + cookie borrada → sesión previa 401 → reuso `INVALID_TOKEN`; `/me/password` 401 sin auth, 400 `INVALID_CURRENT_PASSWORD`, 204; contraseñas y token fuera de logs |
| Contrato OpenAPI | ✅ | forgot 202/400, reset 400/200, `/me/password` 400/204 (`format: email` es anotación en JSON Schema 2020-12: el validador no la exige; el backend sí → 400) |
| Integración PostgreSQL | ✅ 7 pruebas | forgot coalescido (1 job), hash del token con expiración ≤ 30 min, hash de contraseña cambiado, sesión revocada, aviso encolado, reuso rechazado; cambio autenticado conserva la sesión actual |
| Hallazgo propio corregido | ✅ | Una directiva `//nolint` (falso positivo G101) quedó dentro del literal del cuerpo del email → movida fuera; nueva prueba que falla si una directiva aparece en el email |
| E2E Docker Compose + Mailpit (12 pasos) | ✅ | forgot 202 ×2 idénticos → 1 email "Restablece tu contraseña" (`bp_pr_`) → reset débil 400 → reset 200 → sesión previa 401 → clave vieja 401 / nueva 200 → reuso 400 → aviso "Tu contraseña fue cambiada" → cambio autenticado 204 y sesión actual 200; secretos en logs 0; 3 jobs `completed` |
| `task test:integration` | ✅ 18 paquetes | |
| `golangci-lint` / govulncheck / `sqlc diff` / gitleaks | ✅ | 4 falsos positivos G101 en plantillas de email anotados |

## 2026-11-19 · Fase 2 · T2.5 — CI

| Comando | Resultado |
|---|---|
| PR #17 — ci + security | ✅ 6/6 (unit+race 4m36s, integración 3m35s, lint 3m6s, Trivy, govulncheck, gitleaks); squash merge `af1a4ae` |

## 2026-11-19 · Fase 2 · T2.6 — RBAC y autorización por recurso

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias `contract` (políticas puras) | ✅ 4 pruebas | Matriz 3 conjuntos × 3 roles × mismo/otro negocio + cliente sin membresía; falla cerrado (lista vacía, negocio nulo, rol desconocido); negocios independientes; admin sin rol implícito |
| Unitarias aplicación | ✅ 4 pruebas | Revocación pierde acceso en la siguiente llamada; anónimo → `ErrUnauthenticated`; fallo de BD se propaga (no se convierte en decisión de política); listado ordenado y acotado al usuario |
| `httpx.WriteAuthzProblem` | ✅ | 404/403 problem+json, errores envueltos, no mapea otros errores |
| Handlers HTTP | ✅ 2 pruebas | `/me/memberships` vacío/con rol/401; patrón E2E con rutas de prueba: 200 permitido, 403 `FORBIDDEN`, 404 otro negocio y UUID inválido **idénticos** al 404 de ruta inexistente, 404 tras revocar |
| Contrato OpenAPI | ✅ | `/me/memberships` 200 (vacío y con membresía) y 401 |
| Integración PostgreSQL | ✅ `TestAuthorizationOnPostgres` | 2 usuarios, 3 membresías; 200/403/404; `status='REVOKED'` excluida de inmediato; flag de admin leído de la fila del usuario al autenticar |
| `task test:integration` | ✅ | Todos los paquetes ok |
| `task check` | ✅ | lint 0 issues (2 comentarios revive corregidos); govulncheck: 1.er intento falló por DNS (`vuln.go.dev`), reintento sin vulnerabilidades |
| `task sqlc:check` / `openapi:lint` | ✅ | |
| E2E Docker Compose + Mailpit (6 pasos) | ✅ | 401 sin auth → verify 200 → `{"items":[]}` → membresía MANAGER insertada → listada → revocada → `{"items":[]}`; token en logs: 0 |

## 2026-11-19 · Fase 2 · T2.6 — CI

| Comando | Resultado |
|---|---|
| PR #18 — ci + security | ✅ 6/6 (unit+race 4m46s, integración 2m37s, lint 2m30s, Trivy 1m0s, govulncheck, gitleaks); squash merge `76fc4f9` |

## 2026-11-19 · Fase 2 · T2.7 — Rate limiting y fuerza bruta

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias `ratelimit` | ✅ 6 pruebas | Ráfaga y recarga exactas con reloj fijo; tabla acotada (1000 claves → ≤ 100) y limpieza de inactivas; 200 goroutines → exactamente la ráfaga (50); backoff 1-2-4-8-8 s, expira, éxito reinicia, olvido tras `ForgetAfter`, valores por defecto. 1.ª corrida falló por error de coma flotante en la espera (999.999999 ms) → redondeo al ms hacia arriba y épsilon |
| `httpx` | ✅ 3 pruebas | `ClientKey` (IPv4, IPv4 mapeada, IPv6 /64, inválida); middleware 429 + `Retry-After` + problem+json; rotar direcciones dentro de un /64 comparte bucket; `Retry-After` mínimo 1 |
| Aplicación | ✅ 4 pruebas | Backoff idéntico para cuenta existente e inexistente; bloqueado **no** llama al hasher; otra IP no afectada; variantes de mayúsculas comparten clave; crece y expira; acierto reinicia; errores de validación no cuentan; cambio de contraseña con backoff por usuario |
| Handlers HTTP | ✅ 2 pruebas | Límite estricto compartido por 6 endpoints públicos + `/me/password` (`Retry-After: 15`); `/me` no afectado; recarga; 429 de backoff idéntico (cuerpo y `Retry-After`) para cuenta existente e inexistente |
| `internal/app` | ✅ | Límite general con `/healthz` y `/readyz` exentos; 429 conserva cabeceras de seguridad y `X-Request-ID` |
| Configuración | ✅ | Valores por defecto, sobrescrituras y 6 valores fuera de rango rechazados |
| Contrato OpenAPI | ✅ | 429 de backoff (cuenta inexistente) y de límite estricto (forgot, register) conformes |
| `tests/security` fuerza bruta (config por defecto) | ✅ | 200 intentos desde una IP en un minuto → 195 con 429 y solo **5** verificaciones de contraseña; otra IP no afectada; tras 10 min de pausa, ≤ 1 verificación |
| `task check` | ✅ | lint 0 issues, govulncheck sin vulnerabilidades |
| `task test:integration` | ✅ | Todos los paquetes ok |
| E2E Docker Compose | ✅ | 12 logins desde una IP → 10×401 + 2×429 (`Retry-After`, `RATE_LIMITED`); 25 `/healthz` → 200; tras recarga, misma cuenta 7 intentos → 5×401 + 2×429 (backoff); contraseñas en logs: 0 |

## 2026-11-19 · Fase 2 · T2.7 — CI

| Comando | Resultado |
|---|---|
| PR #19 — ci + security | ✅ 6/6 (unit+race 3m59s, integración 3m41s, lint 3m24s, Trivy 51s, govulncheck, gitleaks); squash merge `9129419` |

## 2026-11-19 · Fase 2 · T2.8 — Auditoría básica

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias aplicación (`audit_test.go`) | ✅ 7 pruebas | Registro (éxito / `email_taken`), verificación (token inválido / éxito); login: éxito con sesión, `wrong_password`, `unknown_account` (sin actor, `email_fp` de 16 hex), `email_not_verified` (DENIED), `account_suspended`, `throttled`; cambio, olvido, restablecimiento, logout y logout-all con contadores; `authz.denied` (`insufficient_role`, `no_membership`, PLATFORM_ADMIN) y acceso permitido sin fila; **atomicidad** (fallo de auditoría → login falla y no se crea sesión; login fallido igual responde 401); **sin secretos** (contraseñas, tokens, emails, nombre, user agent) en todo un flujo |
| `reqctx` / HTTP | ✅ | Ida y vuelta del contexto; el `X-Request-ID` del cliente y la IP llegan a la fila de auditoría |
| Pruebas existentes | ✅ | Primera corrida: pánico en `newAuthzService` (sin reloj ni logger) → `NewService` con valores por defecto |
| Integración PostgreSQL | ✅ 2 pruebas | 4 filas reales (login FAILURE/SUCCESS, `authz.denied`, logout) con IP IPv6 `inet`, `occurred_at` = reloj de la aplicación, recurso `session`, JSON de metadatos; restricción CHECK temporal (`NOT VALID`) que rechaza la fila → el login hace rollback y no deja sesión |
| `task check` / `sqlc:check` / `test:integration` | ✅ | lint 0 issues, sin vulnerabilidades |
| E2E Docker Compose + Mailpit | ✅ | 1.er `task up` falló de forma transitoria (exit 17) y el reintento levantó todo. 6 filas: registered, email_verified, login FAILURE (`wrong_password`, `request_id` del cliente), login SUCCESS, password_changed, logout; filas con email, contraseña o token: 0 |

## 2026-11-19 · Fase 2 · T2.8 — CI

| Comando | Resultado |
|---|---|
| PR #20 — ci + security | ✅ 6/6 (unit+race 3m49s, integración 3m32s, lint 2m42s, Trivy 49s, govulncheck, gitleaks); squash merge `54f8513` |

## 2026-11-19 · Fase 2 · T2.9 — Suite de seguridad y cierre

| Comando | Resultado | Observaciones |
|---|---|---|
| `go test ./tests/security/` | ✅ 17 pruebas | 8 nuevas: matriz 5 rutas × 12 credenciales inválidas (401 + `WWW-Authenticate` + `no-store`), tokens nuevos y sin fijación, cookie endurecida, CSRF (4 Origin inválidos), asignación masiva (4 campos), anti-enumeración en 4 endpoints × 3 estados de cuenta, tokens ligados a su propósito, secretos fuera de logs y auditoría. 1.ª corrida: el caso "cookie inválida" en POST recibió 403 `CSRF_REJECTED` (correcto: CSRF se evalúa antes de autenticar) → el caso envía un Origin permitido |
| `go test -count=3 ./tests/security/` | ✅ | Sin inestabilidad (incluye la prueba de tiempos) |
| `worker_test.go` | ✅ 100 % | Registro único de workers, args solo con `user_id`, opciones (coalescido vs. no), delegación, timeout acotado, errores propagados para reintento |
| Cobertura (`go test -cover`) | ✅ | domain 100 %, worker 100 %, reqctx 100 %, ratelimit 98.8 %, argon2 95.8 %, httpx 93.7 %, http 85.2 %, application 84.6 %, contract 80 % |
| `task check` | ✅ | 1.ª corrida: gosec G124 en una cookie de petición simulada → atributos agregados; luego 0 issues, sin vulnerabilidades |
| `task test:integration` / `sqlc:check` | ✅ | |
| `task secrets` (gitleaks) | ✅ | 25 commits, sin fugas |

## 2026-11-19 · Fase 2 · T2.9 — CI

| Comando | Resultado |
|---|---|
| PR #21 — ci + security | ✅ 6/6 (unit+race 4m25s, integración 3m35s, lint 3m21s, Trivy 48s, govulncheck, gitleaks); squash merge `32f7128` |

## 2026-11-19 · Fase 2 · T2.10 — Política de contraseñas (mínimo 8)

| Comando | Resultado | Observaciones |
|---|---|---|
| Unitarias dominio | ✅ 100 % | 7 → corta, 7 runas (9 bytes) → corta, `12345678` / `Password1` / `aaaaaaaa` → comunes, `tijera78` y `ñandú123` (8 runas) → válidas |
| `go test ./...` (incl. contrato OpenAPI y `tests/security`) | ✅ | Spec con `minLength: 8` validada |
| `task check` | ✅ | lint 0 issues; govulncheck falló por DNS (`vuln.go.dev`) y el reintento: sin vulnerabilidades |
| `task test:integration` | ✅ | |

## 2026-11-22 · Fase 2 · T2.10 — CI

| Comando | Resultado |
|---|---|
| PR #22 — ci + security | ✅ 6/6 (unit+race 3m59s, integración 2m35s, lint 3m2s, Trivy 55s, govulncheck, gitleaks); squash merge `d982e06` |

## 2026-11-22 · Fase 3 · T3.1 — Servidor QA (`174.138.92.121`)

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `bash -n` + shellcheck v0.10.0 (`-S style`) | ✅ | 1 aviso informativo (SC1091 `/etc/os-release`) → directiva `source=/dev/null` |
| `ssh root@… 'bash -s' < bootstrap.sh` (paso 1) | ✅ exit 0 | Docker 29.10.0, Compose 5.8.0, swap 2 GB, ufw activo, fail2ban sin baneos |
| Segunda ejecución del paso 1 | ✅ exit 0 | Idempotente |
| `ssh deploy@…`: `id`, `sudo -n true`, `docker run hello-world` | ✅ | Grupo `docker`, sudo sin contraseña |
| Paso 2 `--lock-root` | ✅ | `sshd -T`: `permitrootlogin no`, `passwordauthentication no`, `kbdinteractiveauthentication no`, `allowusers deploy`, `maxauthtries 3` |
| `ssh root@…` tras el bloqueo | ✅ rechazado | `Permission denied (publickey)` |
| `ssh deploy@…` solo con contraseña | ✅ rechazado | `Permission denied (publickey)` |
| Puertos desde fuera (22, 80, 443, 5432, 8025) | ✅ | Solo 22 abierto; 80/443 permitidos en ufw pero sin servicio aún |
| Servicios escuchando (`ss -tln`) | ✅ | Solo `sshd` y el resolvedor local (127.0.0.53/54) |
| `unattended-upgrades`, swappiness, Docker, zona horaria | ✅ | Habilitado con reinicio 04:30; 10; `local` + `live-restore` + `no-new-privileges`; UTC |

## 2026-11-22 · Fase 3 · T3.1 — CI

| Comando | Resultado |
|---|---|
| PR #23 — ci + security | ✅ 6/6 (lint con shellcheck 3m13s); squash merge `c4952dd` |

## 2026-11-22 · Fase 3 · T3.2 — Stack QA

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| shellcheck (3 scripts) / `docker compose config --quiet` | ✅ | |
| `docker build` imagen `qa-c4952dd` | ✅ | 1.er intento falló por DNS (`production.cloudfront.docker.com`); reintento OK, 28.5 MB |
| Stack QA **en local** (Docker Desktop) | ✅ | Roles creados, 4 migraciones con `app_migrator`, `/readyz` 200, E2E 5 pasos verde |
| Stack QA **en el servidor** | ✅ | `app roles created`; migraciones al día; `/readyz` `{"status":"ready"}`; `/healthz` versión `qa-c4952dd` |
| E2E en el servidor (túnel SSH a 8080/8025) | ✅ | register 202 → email en Mailpit con enlace `https://booking-platform-col.duckdns.org/verify-email` → verify 200 → login MOBILE (token 48 caracteres) → `/me` 200 |
| Privilegios `app_runtime` (local y servidor) | ✅ | SELECT OK; CREATE TABLE → `permission denied for schema public`; DROP/ALTER → `must be owner`; TRUNCATE → `permission denied`; CREATE ROLE denegado; sin acceso a la base `postgres` |
| Privilegios `app_migrator` | ✅ | No superusuario, no crea roles; dueño de las tablas |
| Memoria en el servidor (1 GB) | ✅ | api 135 MiB, worker 68 MiB, postgres 89 MiB, mailpit 10 MiB; host 650 MB usados, swap 28 MB |
| Puertos desde fuera (22, 80, 443, 5432, 8080, 8025) | ✅ | Solo 22 abierto; 8080/8025 escuchan solo en `127.0.0.1` |

## 2026-11-22 · Fase 3 · T3.2 — CI

| Comando | Resultado |
|---|---|
| PR #24 — ci + security | ✅ 6/6; squash merge `9bacd39` |

## 2026-11-22 · Fase 3 · T3.3 — Caddy, HTTPS e IP real

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestRealIP` (12 casos) + `TestRealIPDisabledWithoutTrustedProxies` | ✅ | Cliente directo con cabecera falsa → IP TCP; proxy de confianza → IP del cliente (cadena, varias líneas, IPv6, IPv4 mapeada); entrada malformada o solo saltos de confianza → IP del proxy; misma subred pero otra IP → no confiable; la petición original no se muta |
| `TestTrustedProxiesConfig` | ✅ | Vacío por defecto; IPs → /32 y /128; `0.0.0.0/0`, `::/0`, basura y `/33` rechazados |
| `tests/security` `TestClientCannotSpoofItsIP` | ✅ | Cliente directo rotando `X-Forwarded-For` sigue en un bucket (429); detrás del proxy, buckets por cliente real; prefijos falsos en la cadena no evitan el límite |
| `caddy validate` / `caddy fmt` | ✅ | |
| Certificado | ✅ | `CN=booking-platform-col.duckdns.org`, emisor Let's Encrypt YE2, vence 2027-02-20; TLS 1.3 |
| HTTP → HTTPS | ✅ | 308 a `https://…` |
| Cabeceras | ✅ | HSTS `max-age=31536000`, `nosniff`, `no-referrer`, `no-store`, `X-Request-ID`, `alt-svc h3`; sin `Server` |
| TLS ≤ 1.1 / cuerpo 3 MB | ✅ | Conexión rechazada / 413 |
| IP real (login fallido con `X-Forwarded-For: 6.6.6.6`) | ✅ | Log de la API `remote_addr 181.54.0.95:0` y `audit_log.ip 181.54.0.95` = IP pública del cliente (la misma vista por SSH) |
| E2E por HTTPS público (Mailpit por túnel SSH) | ✅ | register 202 → email → verify 200 → login → `/me` 200 |
| Puertos desde fuera | ✅ | 22, 80, 443 abiertos; 5432, 8080, 8025 cerrados |
| Memoria con Caddy | ✅ | caddy 12 MiB; host 660 MB usados, swap 37 MB |
| `task check` / `task shellcheck` | ✅ | 0 issues, sin vulnerabilidades |
| Incidencias | ⚠️ resuelto | Health check activo → 503 en el arranque (cambiado a reintentos); DNS local intermitente (`--resolve`) |

## 2026-11-23 · Fase 3 · T3.3 — CI

| Comando | Resultado |
|---|---|
| PR #25 — ci + security | ✅ 6/6; squash merge `e5aab0f` |

## 2026-11-23 · Fase 3 · T3.4 + T3.5 — Despliegue desde GitHub

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `task check` / `task shellcheck` (incluye `deploy.sh`) / `task secrets` | ✅ | 0 issues, sin vulnerabilidades (govulncheck reintentado por DNS local), sin fugas |
| `actionlint 1.7.7` | ✅ `release-qa.yml` | Aviso previo SC2046 en `ci.yml` (división de palabras intencional en la lista de scripts) |
| `trivy config` `deploy/` | ✅ | 0 misconfiguraciones |
| `docker build` + extracción de `/deploy/qa/` | ✅ | Imagen 28,5 MB; compose, Caddyfile e initdb (0755) extraídos |
| Despliegue en QA con registro local temporal (`127.0.0.1:5000`, copia del script con `REPO` de prueba) | ✅ | pull → backup 37 KB → migraciones (0 pendientes) → recrea api/worker → smoke → `ok` en `deployments.log`, `API_IMAGE` actualizado; 1.ª ejecución falló por `ghcr.io` fijo en el login (corregido) |
| Rollback (versión esperada incorrecta) | ✅ | Smoke falla a los 60 s → restaura stack e imagen `qa-2d3f46c` → smoke OK → `rolled-back`, exit 1; `/healthz` = `qa-2d3f46c` |
| Validación de entrada | ✅ | Rechazados (exit 1): otro registro, tag sin digest, `;id`, `&& id`, versión `../etc`, `bash -i`, sin token |
| Llave de CI restringida | ✅ | Comando arbitrario → ejecuta solo `booking-deploy` (usage, exit 1); reenvío de puertos → `administratively prohibited`; token falso → `registry login failed` |
| Limpieza del servidor | ✅ | Registro de prueba, imágenes de prueba y `qa-c4952dd` borrados; script de prueba eliminado |
| Workflow `release-qa` en GitHub | ⏳ | Se ejecuta al fusionar en `qa` |

## 2026-11-23 · Fase 3 · T3.4/T3.5 — CI y primer release

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #26 — ci + security | ✅ 6/6 | Squash merge `40300ea` |
| `release-qa` run 45261417122 | ❌ | build ✅, Trivy ✅, SBOM ✅; push GHCR `denied`; deploy y prune omitidos; QA intacto (`qa-2d3f46c`). El build generó índice + atestación (almacén containerd) → fix `--provenance=false --sbom=false` |

## 2026-11-23 · Fase 3 · T3.6 — Backups

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `task shellcheck` (incluye `backup.sh`) | ✅ | Sin hallazgos |
| `systemd-analyze verify` service + timer | ✅ | Sin avisos |
| `systemctl start booking-backup.service` | ✅ | `Result=success`; `daily-20261123T203457Z.dump` (37 KB) + `.sha256`; `last-backup` ok; timer próximo 2026-11-24 03:16 UTC |
| `booking-backup verify` | ✅ | Restaurado en `booking_verify`: 12 tablas, `goose_version 4`, mismos conteos; base temporal eliminada |
| Archivo truncado (20 KB, sin checksum) | ✅ rechazado | `pg_restore: could not read from input file` → exit 1 |
| Archivo alterado (1 byte) | ✅ rechazado | `checksum mismatch` → exit 1 |
| **Restauración en sitio** (`restore … --yes`) | ✅ | Usuarios 2 → registro marcador (3) → restore → 2 y marcador ausente; dump de seguridad `prerestore-*`; api/worker detenidos ≈5 s; `/readyz` OK por Caddy |
| Permisos tras restaurar | ✅ | Base y tablas de `app_migrator`; `app_runtime` INSERT ✅, CREATE en `public` ❌ (correcto), CONNECT ✅; ACL de la base sin PUBLIC; registro vía API 202 (escritura con `app_runtime`) |
| Copia semanal + verify del domingo | ⏳ | Misma ruta de código que `verify` (probado); primera ejecución automática el domingo 2026-11-29 |

## 2026-11-23 · Fase 3 · Despliegue automático a QA (correcciones y verificación)

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| PR #27 / #29 / #28 — ci + security | ✅ 6/6 cada uno | Squash merges `ab5b22a`, `5070757`, `9b169a9` |
| `release-qa` 45266135708 (`ab5b22a`) | ❌ | Push y pull OK; extracción de `/deploy/qa` → `permission denied` (directorios 0644); servidor sin cambios |
| `docker export` de la imagen corregida | ✅ | Directorios 0755, archivos 0644, `10-app-roles.sh` 0755 |
| `release-qa` 45268736830 (`5070757`) | ✅ | image ✅ (extracción no-root ✅), deploy ✅ (pull, backup, migraciones, smoke, HTTPS público), prune ✅ (1 versión sin tag borrada) |
| `release-qa` 45269111095 (`9b169a9`) | ✅ | Segundo despliegue; `deployments.log` `ok` × 2; `predeploy-*` por despliegue; servidor conserva solo imagen actual y anterior |
| `/healthz` público | ✅ | `{"status":"ok","version":"9b169a9"}` |

## 2026-11-23 · Fase 3 · T3.7 — Validación E2E en QA

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `scripts/e2e-qa.sh` (1.ª corrida) | ❌ 25 fallos | Todos 429 del límite de autenticación (10/min por IP) contra el propio script; el API se comportó correctamente → el script ahora respeta `Retry-After` |
| `scripts/e2e-qa.sh` contra https://booking-platform-col.duckdns.org (contenedor Alpine, IP pública del cliente, Mailpit por túnel SSH) | ✅ **52/52** | 1 min 51 s; versión `9b169a9`. Plataforma, registro + verificación, anti-enumeración (registro y forgot 202, login 401 igual), login antes de verificar 403, tokens de un solo uso, `/me` sin hash, token falso 401, cambio de contraseña (sesión actual sí, otras revocadas), reset (todas revocadas), logout/logout-all, 429 con `Retry-After` y `RATE_LIMITED` |
| `audit_log` de la corrida | ✅ | 15 combinaciones acción/resultado (SUCCESS/FAILURE/DENIED), todas con IP `181.54.0.95` = IP pública del cliente (ipify); 0 filas con contraseñas o tokens en `metadata` |
| `BenchmarkHashCalibration` en el Droplet (1 vCPU DO-Premium-Intel) | ✅ | 64 MiB p=2: t=2 222–262 ms, t=3 330–347 ms, t=4 437 ms, t=5 541 ms, t=6 735 ms; 64 MiB por hash |
| Recursos tras el E2E | ✅ | 599 MB usados, swap 62 MB; api 132/256 MiB; disco 18 % |
| `task check` / `task shellcheck` | ✅ | Ver commit de T3.7 |

## 2026-11-23 · Fase 3 · T3.7 — Tras el despliegue

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| PR #30 — ci + security | ✅ 6/6 | Squash `d0f48b1` |
| `release-qa` (`d0f48b1`) | ✅ | image, deploy, prune; contenedor api con `ARGON2_ITERATIONS=2` |
| Rehash automático | ✅ | Usuario E2E: `m=65536,t=3,p=2` → `t=2` tras el primer login; los otros 4 usuarios siguen en t=3 hasta su próximo login |
| Latencia de login (localhost del servidor, 6 muestras) | ✅ | 1.º 737 ms (verifica t=3 + rehash t=2), luego 206–405 ms |
| `scripts/e2e-qa.sh` en `d0f48b1` | ✅ 52/52 | 1 min 27 s (antes 1 min 51 s) |

## 2026-11-23 · Fase 3 · T3.8 — Cierre

| Comando | Resultado | Observaciones |
|---|---|---|
| `actionlint` `release-tag.yml` | ✅ | |
| `release-tag.yml` en GitHub | ⏳ | Se ejecuta al crear el tag `v0.3.0` |

## 2026-11-23 · Fase 3 · T3.8 — Despliegue final

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #31 — ci + security | ✅ 6/6 | Squash `f861fb1`; `release-qa` ✅ |
| `scripts/e2e-qa.sh` en `f861fb1` | ✅ 52/52 | Imagen final de la Fase 3 |

## 2026-11-23 · Fase 4 · T4.1 — Geografía

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/geography/...` | ✅ | domain **100 %**, application **91 %** (lecturas solo activas y ocultamiento en cascada, 6 escrituras × no admin → 403 sin cambios, creación/edición con auditoría y campos cambiados, validación por campo, duplicados → conflicto, PATCH sin cambios = no-op) |
| `TestSetPlatformAdmin` / `platform/audit` | ✅ | Cuenta no verificada rechazada, idempotente, auditoría `source=cli` sin actor; constructor de auditoría con IP y request ID |
| `TestContractGeographyEndpoints` | ✅ | 9 operaciones nuevas validadas contra OpenAPI: 200/201/400/401/403/404/409 |
| Integración PostgreSQL (`-tags=integration`) | ✅ | Datos iniciales (CO, CO-VAC/CO-ANT, 4 ciudades en America/Bogota); nombres únicos sin distinguir mayúsculas y códigos ISO → conflicto; auditoría con campos cambiados; **rollback si la auditoría falla**; CHECK del esquema rechazan 6 casos inválidos; `SetPlatformAdmin` en BD |
| `tests/security` | ✅ | 6 rutas admin en la matriz de 12 credenciales inválidas (401); no admin → 403 + `authz.denied` × 6; revocación de `PLATFORM_ADMIN` inmediata con la misma sesión; asignación masiva (id, createdAt, code, countryCode, divisionId) → 400; rutas malformadas/inyección → 404 genérico; país público sin campos internos |
| `task check` / `task sqlc:check` / `task shellcheck` | ✅ | 0 issues, sin vulnerabilidades |
| Suite completa `go test -tags=integration ./...` | ✅ | Todos los paquetes |
| PR #32 — ci + security | ✅ 6/6 | Squash `0449274`; `release-qa` ✅ (migración 5 aplicada en QA, 4 ciudades) |
| `api admin grant-platform-admin admin@booking-qa.test` en QA | ✅ | Cuenta sintética (Mailpit) verificada; `is_platform_admin=t`; auditoría `admin.platform_admin_granted` `source=cli` |
| Escrituras admin en QA | ✅ | Ciudad duplicada → 409; zona inválida → 400; desactivar Yumbo → desaparece de la lista pública; reactivar → vuelve; 2 entradas `geography.city_updated` con IP real |
| `scripts/e2e-qa.sh` en `0449274` | ✅ **62/62** | 1.ª corrida 61/62: la comparación por regex del script no escapaba `+57` (error del script, no del API) → corregido |

## 2026-11-24 · Fase 4 · T4.2 — Dominio y persistencia de negocios

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/businesses/...` | ✅ | domain **93,9 %** (nombres, descripción, dirección, slug y derivación con tildes/ñ/reservados/longitud, teléfonos CO móvil y fijo y E.164 internacional, coordenadas, zona horaria, moneda, rangos de política, recordatorios, invariantes del agregado, suspender/activar idempotente); application **100 %** (`AllocateSlug`: libre, `-3` tras colisiones, agotado, error del store) |
| Geografía: `TestDeactivationBlockedByActiveBusinesses`, `TestPlaceOfCity` | ✅ | País/división/ciudad con negocios activos → `ErrInUse`; renombrar sí; `PlaceOfCity` con padres inactivos → `Active=false` |
| Integración businesses (PostgreSQL) | ✅ | Ida y vuelta completa (coordenadas, `location` PostGIS derivada `POINT(-76.532 3.4516)`), actualización/traslado de ciudad; slug duplicado → `ErrSlugTaken`; slug en mayúsculas rechazado; `AllocateSlug` → `-2`; ciudad de otra división → FK; CHECK de teléfono, intervalo, coordenadas a medias, estado y `--`; membresía de negocio inexistente → FK; borrar negocio con miembros → RESTRICT; auditoría de negocio inexistente aceptada |
| `TestGeographyCannotDeactivateAreasInUse` (composition root, PostgreSQL) | ✅ | Ciudad/división/país con negocio activo → `IN_USE`; tras suspender el negocio se desactiva la ciudad |
| Contrato | ✅ | `PATCH` ciudad y país en uso → 409 `IN_USE` conforme a OpenAPI |
| Hallazgos corregidos | ⚠️→✅ | Slug de respaldo reservado; CHECK de coordenadas con NULL; prueba que cruzaba módulos (depguard) |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / integración completa (`-p 2`) | ✅ | govulncheck reintentado por DNS local; imagen 36,6 MB (antes 28,5 MB: metadatos de teléfonos) |

## 2026-11-24 · Fase 4 · T4.2 — CI y despliegue

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #33 — ci + security | ✅ 6/6 | Squash `b85b596`; `release-qa` ✅; migración 6 en QA; `app_runtime` con INSERT en `businesses` |

## 2026-11-24 · Fase 4 · T4.3 — Alta de negocios

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestWithTxJoinsTheContextTransaction` (PostgreSQL) | ✅ | La transacción anidada con el contexto se une a la externa: error interno → ambas filas revertidas; éxito → ambas confirmadas |
| `TestAddMember` (identity) | ✅ | Admin agrega OWNER (auditoría `membership.granted`), OWNER agrega MANAGER; ya miembro, no verificado, inexistente, email malformado → errores del contract; MANAGER → 403; ajeno → 404; segundo OWNER → `ErrOwnerExists`; rol desconocido rechazado |
| `go test ./internal/modules/businesses/...` | ✅ | application **87,5 %**: alta (`PENDING_REVIEW`, slug, E.164, zona/moneda de la ciudad, valores por defecto, OWNER, auditoría), segundo con `-2`, **atomicidad** (fallo del OWNER → ni negocio ni auditoría), validación por campo, ciudad desconocida/inactiva, solo admin en 5 casos, activar/suspender idempotentes, ubicación inactiva → `ErrLocationInactive`, paginación y filtros |
| `TestBusinessOnboardingOnPostgres` (composition root) | ✅ | Negocio + membresía OWNER + 2 entradas de auditoría en BD; OWNER no verificado → **ninguna fila** del negocio ni de su auditoría; el autorizador de identity ve el rol; activar/suspender; ciudad inactiva → `LOCATION_INACTIVE`; listado filtrado |
| `TestContractBusinessEndpoints` | ✅ | 5 operaciones contra OpenAPI: 201/200/400/401/403/404/409, coordenadas `null`, `nextCursor` |
| `tests/security` | ✅ | 5 rutas en la matriz de credenciales inválidas; no admin → 403 + `authz.denied` ×5 sin cambios; 9 campos controlados por el servidor (status, slug, id, timeZone, currency, countryCode, divisionId, bookingPolicy, createdAt) → 400; OWNER no verificado → 400 `ACCOUNT_NOT_ELIGIBLE` sin crear negocio |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / unitarias + integración completas | ✅ | govulncheck reintentado por DNS local |
| Despliegue y E2E en QA | ⏳ | Tras el merge |

## 2026-11-24 · Fase 4 · T4.3 — CI, despliegue y E2E

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #34 — ci + security | ✅ 6/6 | Squash `c2c3d77`; `release-qa` ✅ |
| `scripts/e2e-qa.sh` con `ADMIN_EMAIL`/`ADMIN_PASSWORD` en QA | ✅ **70/70** | Alta real por el admin: `PENDING_REVIEW`, COP, `America/Bogota`, `+573001234567`; el dueño ve su membresía OWNER; activar → `ACTIVE`; suspender → `SUSPENDED`; dueño no verificado → 400 |

## 2026-11-24 · Fase 4 · T4.3b — Alta autoservicio con demo de 7 días

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/subscriptions/...` | ✅ | domain **100 %** (demo de 7 días, `Entitled` al segundo exacto del fin, extender/reabrir, límites 1–90 días, cortesía idempotente, no extensible si ACTIVE); application **88,6 %** (inicio, vencimiento solo de las vencidas, extensión, auditoría con y sin actor, solo admin); worker **90,9 %** |
| `go test ./internal/modules/businesses/...` | ✅ | Autoservicio `ACTIVE` con demo y OWNER = quien llama (auditoría `via=self_service`); segunda → `ErrAlreadyOwner` sin persistir; ciudad inactiva; anónimo; **fallo de la demo → ni negocio ni auditoría** (autoservicio y admin); el alta admin también inicia demo |
| `TestSelfServiceOnboardingOnPostgres` | ✅ | **Dos solicitudes simultáneas del mismo usuario → exactamente 1 negocio** (la otra `ALREADY_OWNER`, sin filas huérfanas); suscripción `TRIALING` a 7 días; 3 entradas de auditoría; SQL real del job vence la demo (`EXPIRED`, auditoría `source=job` sin actor); negocio con OWNER no se puede borrar |
| `TestContractSelfServiceAndSubscriptions` | ✅ | 4 operaciones nuevas contra OpenAPI: 201/400/401/403/404/409 (`ALREADY_OWNER`, `NOT_EXTENDABLE`) |
| `tests/security` | ✅ | Autoservicio y 3 rutas admin de suscripción en la matriz de credenciales inválidas; no admin → 403; 6 campos controlados por el servidor (`ownerEmail`, status, slug, subscription, trialEndsAt, currency) → 400; quien llama queda OWNER de ese negocio; segunda → 409 |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / unitarias + integración completas | ✅ | govulncheck reintentado por DNS local |
| Despliegue y E2E en QA | ⏳ | Tras el merge |

## 2026-11-24 · Fase 4 · T4.3b — CI, despliegue y E2E

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #35 — ci + security | ✅ 6/6 | Squash `d69023f`; `release-qa` ✅; migración 7 (1 suscripción `TRIALING` creada para el negocio existente) |
| Job `subscriptions.expire_trials` en QA | ✅ | `completed` al arrancar el worker (`RunOnStart`) |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **74/74** | Autoservicio 201 `ACTIVE`; segunda 409 `ALREADY_OWNER` |

## 2026-11-24 · Fase 4 · T4.4 — Gestión del negocio

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPlatformAdminActsOnAnyBusiness` (identity) | ✅ | Admin pasa los 3 conjuntos de roles en cualquier negocio sin auditoría de denegación; sin la marca → 404 |
| `TestApplyUpdate` (domain) | ✅ | Cambios parciales con lista de campos; mismo valor = sin cambio ni nueva fecha; inválido no modifica; cambio de país rechazado; coordenadas iguales con otro puntero = sin cambio. Domain **95,1 %** |
| `manage_test.go` (application) | ✅ | Ver: OWNER/MANAGER/PROFESSIONAL/admin sí; otro negocio y sin membresía → 404. Editar: MANAGER/PROFESSIONAL → 403, ajenos → 404, OWNER y admin sí (auditoría con el admin como actor); teléfono, descripción, traslado Cali→Medellín (división y zona), quitar coordenadas, política y recordatorios parciales; no-op sin auditoría; 7 errores de campo a la vez; ciudad de otro país/inactiva/desconocida. Application **89 %** |
| `TestContractBusinessManagement` | ✅ | GET/PATCH contra OpenAPI: 200/400/401/403/404, `coordinates: null` |
| `tests/security` `TestBusinessManagementIsTenantScoped` | ✅ | Negocio ajeno (GET y PATCH) → 404 **idéntico** al de un negocio inexistente y sin cambios; MANAGER lee pero no edita; slug/status/currency/timeZone/countryCode/id → 400; admin edita cualquier negocio; 2 rutas nuevas en la matriz de credenciales inválidas |
| `TestSelfServiceOnboardingOnPostgres` (ampliada) | ✅ | Dueño traslada a Medellín → `division_id` de Antioquia en BD; vista con demo; extraño → 404; auditoría `business.updated` con `fields=location` |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / unitarias + integración completas | ✅ | govulncheck reintentado por DNS local |
| Despliegue y E2E en QA | ⏳ | Tras el merge |

## 2026-11-24 · Fase 4 · T4.4 — CI, despliegue y E2E

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #36 — ci + security | ✅ 6/6 | Squash `ffb2eae`; `release-qa` ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **79/79** | Dueño ve su negocio con demo `TRIALING`, lo edita (descripción, intervalo 15), negocio ajeno → 404 |

## 2026-11-24 · Fase 4 · T4.5 — Miembros del negocio

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestManageMembers` / `TestAddMember` (identity) | ✅ | Listar: OWNER y MANAGER sí, PROFESSIONAL 403, ajeno 404; solo OWNER agrega/cambia; último OWNER no se degrada ni retira; **cambio de dueño** (nuevo OWNER → el anterior baja y sale); retirado pierde acceso; re-agregar reactiva; varios OWNER permitidos; mismo rol = sin auditoría; auditoría de cambios y revocaciones |
| `TestBusinessAlwaysKeepsAnOwner` (PostgreSQL) | ✅ | **Dos OWNER se retiran mutuamente a la vez → exactamente uno lo logra y queda 1 OWNER**; re-agregar reactiva la misma fila; miembro activo → `ErrAlreadyMember` |
| `TestMembershipConstraints` (esquema) | ✅ | Actualizada: segundo OWNER aceptado; duplicado usuario-negocio y rol desconocido rechazados |
| `TestContractBusinessMembers` | ✅ | 4 operaciones contra OpenAPI: 201/204/400/403/404/409 (`ALREADY_MEMBER`, `LAST_OWNER`) |
| `tests/security` `TestMembersAreTenantScoped` | ✅ | 4 operaciones sobre miembros de otro negocio → 404 idéntico a inexistente; barbero no se autopromueve (403); retiro inmediato (misma sesión → 404); último OWNER no sale; 4 rutas en la matriz de credenciales inválidas |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / unitarias + integración completas | ✅ | govulncheck reintentado por DNS local |
| Despliegue y E2E en QA | ⏳ | Tras el merge |

## 2026-11-24 · Fase 4 · T4.5 — CI, despliegue y E2E

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #37 — ci + security | ✅ 6/6 | Squash `78ebf6f`; `release-qa` ✅ (migración 8) |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **87/87** | Barbero registrado y verificado; agregado como PROFESSIONAL, cambio a MANAGER, retirado; último OWNER no sale (409 `LAST_OWNER`) |

## 2026-11-24 · Fase 4 · T4.6 — Búsqueda pública

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `public_test.go` (application) | ✅ | Solo activos con demo válida, orden sin distinguir mayúsculas, filtro de texto, otra ciudad vacía, comodines literales; **paginación con un tercio de demos vencidas** (todas las visibles exactamente una vez, páginas ≤ límite); 7 validaciones (ciudad, texto, cursor inválido o falsificado, límites); detalle por slug: vencido, suspendido, inexistente, `../etc` → 404. Application **90,2 %** |
| `TestPublicSearchOnPostgres` | ✅ | SQL real + filtro de subscriptions: vencido y suspendido ocultos, paginado de a 2, orden por intercalación (ñ con n), `50%` literal, `_` no es comodín, búsqueda sin distinguir mayúsculas con tildes (`ÑANDÚ`), detalle por slug |
| `TestContractPublicBusinesses` | ✅ | 2 operaciones públicas contra OpenAPI: 200 (cursor), 400, 404 |
| `tests/security` | ✅ | Sin campos internos en la vista pública; suspendido → 404 idéntico a inexistente; `%%`, `_%`, `' OR 1=1--` no coinciden |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / unitarias + integración completas | ✅ | |
| Despliegue y E2E en QA | ⏳ | Tras el merge |

## 2026-11-24 · Fase 4 · T4.6 — CI, despliegue y E2E

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #38 — ci + security | ✅ 6/6 | Squash `f4db9ea`; `release-qa` ✅ (migración 9) |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **92/92** | Búsqueda pública encuentra el negocio autoservicio, sin campos internos; detalle por slug; slug inexistente 404; sin ciudad 400 |

## 2026-11-24 · Fase 4 · T4.7 — Cierre

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestEveryRouteIsDocumented` | ✅ | 39 rutas registradas = 39 operaciones del contrato (ninguna sin documentar, ninguna documentada sin ruta) |
| `TestEveryBusinessRouteIsTenantIsolated` | ✅ | Las 6 operaciones `{businessId}` del contrato: usuario de otro negocio → 404 idéntico a negocio inexistente; datos y miembros del otro negocio sin cambios; ≥ 6 `authz.denied` auditados |
| `TestSummaryAndEntitledAmong` (subscriptions) | ✅ | Lote: solo la demo extendida habilitada; lote vacío; resumen con y sin suscripción. Application **89,2 %** |
| `task check` / `sqlc:check` / `secrets` / integración completa | ✅ | govulncheck reintentado por DNS local |

## 2026-11-24 · Fase 4 · T4.7 — Validación del flujo del dueño y ajuste

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| Flujo del dueño en vivo en QA (`f4db9ea`), **sin la cuenta administradora** | ✅ | Registro 202 → verificación 200 → login → crea barbería: `ACTIVE`, demo `TRIALING` hasta 2026-12-01, `entitled=true`, es OWNER → la búsqueda pública la encuentra sin login → detalle por slug 200 |
| Alta por el administrador nace `ACTIVE` (unitarias, contrato, seguridad, integración) | ✅ | Suspender → reactivar → idempotencias sin auditoría; reactivación con ubicación inactiva → `LOCATION_INACTIVE`; filtros por estado |
| `task check` / `shellcheck` / unitarias + integración completas | ✅ | |

## 2026-11-24 · Fase 4 · T4.7 — Despliegue final

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #39 — ci + security | ✅ 6/6 | Squash `5a9843e`; `release-qa` ✅ |
| Flujo del dueño sin administrador en QA | ✅ | Registro → verificación → barbería `ACTIVE` → demo `TRIALING` → OWNER → visible en la búsqueda pública → detalle por slug |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **93/93** | Alta del administrador nace `ACTIVE`; suspender, reactivar, suspender |

## 2026-11-24 · Fase 5 · T5.1 — Quitar el rol MANAGER

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| Unitarias (identity, businesses, contrato, seguridad) | ✅ | Matriz de roles con OWNER y PROFESSIONAL; listado de miembros solo OWNER (PROFESSIONAL 403); revocación inmediata con `RolesStaff`; auditoría con `required_roles=OWNER` |
| Integración (PostgreSQL) | ✅ | Migración 00010 aplicada; `MANAGER` y roles desconocidos rechazados por el CHECK; autorización con profesional |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| Despliegue y E2E en QA | ⏳ | Tras el merge (incluye "MANAGER ya no existe" → 400) |

## 2026-11-24 · Fase 5 · T5.1 — CI, despliegue y E2E

| Comando | Resultado | Observaciones |
|---|---|---|
| PR #40 — ci + security | ✅ 6/6 | Squash `a92de2c`; `release-qa` ✅ (migración 10) |
| Membresías en QA tras la migración | ✅ | OWNER ACTIVE 13, PROFESSIONAL REVOKED 3 (los MANAGER de prueba convertidos) |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **95/95** | Barbero → OWNER → PROFESSIONAL; `MANAGER` → 400 |

## 2026-11-24 · Fase 5 · T5.2 — Profesionales

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/professionals/...` | ✅ | domain **100 %**; application **91,5 %**: barbero nuevo por email → miembro PROFESSIONAL + perfil; el OWNER también es profesional y conserva su rol; un perfil por cuenta; cuentas no verificadas/inexistentes/email inválido → `ACCOUNT_NOT_ELIGIBLE`; 4 errores de campo; barbero lee pero no crea ni edita; ajeno 404; admin edita; cambios auditados con campos; no-op sin auditoría; retirado del negocio → `bookable=false` |
| `TestEnsureMemberAndActiveMembersAmong` (identity) | ✅ | Nuevo miembro PROFESSIONAL, miembro existente conserva rol, idempotente, no elegibles, profesional no puede, lote de miembros activos y tras retiro |
| `TestProfessionalsOnPostgres` | ✅ | Membresía + perfil en BD; OWNER como profesional sin perder OWNER; duplicado → `ErrAlreadyExists`; el mismo barbero en un segundo negocio; retiro → no reservable; auditoría; sin lecturas cruzadas |
| `TestEveryRouteIsDocumented` / `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **43 rutas documentadas**; **10 operaciones `{businessId}`** aisladas (incluye las 4 nuevas, sin escribir otra prueba) |
| `TestContractProfessionals` | ✅ | 4 operaciones contra OpenAPI: 200/201/400/401/403/404/409 |
| `task check` / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #41 — ci + security | ✅ 6/6 | Squash `1c05c0c` |
| `release-qa` (1.er intento) | ❌ | `docker pull` en el servidor: `no matching manifest for linux/amd64/v3 … not found` justo tras el push; imagen verificada como manifiesto simple; nada cambió en el servidor |
| `release-qa` (job relanzado) | ✅ | Migración 11 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **101/101** | Barbero y responsable como profesionales; 409 duplicado |

## 2026-11-24 · Fase 5 · T5.3 — Horario semanal y descansos

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/professionals/...` | ✅ | domain **99,1 %**: `HH:MM` (rechaza `9:30`, `24:00`, `12:60`), orden, rangos que se tocan permitidos, 11 violaciones con ruta (día inválido/duplicado, hora, rango invertido o vacío, >6 rangos, solapes de franjas y de descansos, descanso fuera de franja); application **92,4 %**: OWNER define, barbero edita el suyo y lee el de otros, 403 al editar el de un compañero, admin limpia la semana, no-op sin auditoría, auditoría con días y `self`, extraño/otro negocio 404, barbero retirado 404 |
| `TestProfessionalScheduleOnPostgres` | ✅ | Ida y vuelta con 00:00 y 23:59; filas en BD; barbero propio vs. ajeno; auditoría; FK compuesta rechaza fila con otro negocio (2 tablas); CHECK `start < end` (2 tablas); **8 reemplazos concurrentes → el resultado es exactamente una de las semanas (2 filas)** |
| Prueba de mutación del bloqueo | ✅ detecta | Sin `FOR UPDATE` la prueba falla 3/3 (14 filas mezcladas en vez de 2); restaurado |
| `TestEveryRouteIsDocumented` / `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **45 rutas documentadas**; **12 operaciones `{businessId}`** aisladas (incluye las 2 nuevas); el horario de Bob no cambia |
| `TestContractProfessionals` | ✅ | `GET/PUT …/schedule` contra OpenAPI: 200/400/403/404 |
| `task check` (build, vet, lint 0, test, vuln) / `sqlc:check` / `shellcheck` / `secrets` | ✅ | govulncheck tras reintento por DNS local |
| Integración completa (`-tags integration -p 2 ./...`) | ✅ | |
| PR #42 — ci + security | ✅ 6/6 | Squash `14a4b39` |
| `release-qa` | ✅ | Migración 12 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **110/110** | Horario: OWNER define, 400 descanso fuera de franja, barbero edita el suyo, 403 el del responsable, lectura permitida |

## 2026-11-24 · Fase 5 · T5.4 — Ausencias e incapacidades

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/professionals/...` | ✅ | domain **99,2 %** (periodo: igual/invertido/sub-segundo/cero/>366 días; motivo ≤ 200 de una línea); application **94,0 %**: OWNER registra para el barbero, barbero las suyas con zona horaria → UTC, solape 409, contiguas permitidas, otro profesional mismo periodo OK, 403 sobre las del compañero, lectura del equipo, ventana `from/to` e invertida 400, edición con auditoría de campos y `self`, no-op sin auditoría, borrar dos veces 404, admin, extraños/otro negocio 404, barbero retirado 404 |
| `TestProfessionalAbsencesOnPostgres` | ✅ | UTC; solape rechazado por la **restricción de exclusión** (alta y edición); FK compuesta, `starts_at < ends_at`, máx. 366 días; auditoría; **8 altas simultáneas que se solapan → exactamente 1 guardada y 7 `ABSENCE_OVERLAP`** |
| Prueba de mutación de la exclusión | ✅ detecta | Sin `professional_absences_no_overlap` la prueba falla (solapes aceptados); restaurada |
| `TestEveryRouteIsDocumented` / `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **49 rutas documentadas**; **16 operaciones `{businessId}`** aisladas; las ausencias de Bob no cambian |
| `TestContractProfessionals` | ✅ | Ausencias contra OpenAPI: 201/200/204/400/403/404/409 |
| `task check` (lint 0) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | govulncheck tras reintento por DNS local |
| PR #43 — ci + security | ✅ 6/6 | Squash `facfa0c` |
| `release-qa` | ✅ | Migración 13 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **118/118** | Ausencia del barbero guardada en UTC, solape 409 `ABSENCE_OVERLAP`, 403 sobre las del responsable, edición y borrado |

## 2026-11-24 · Fase 5 · T5.5 — Catálogo de servicios

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/catalog/...` | ✅ | domain **100 %** (nombres, descripción, duración 5–480 ×5, precio sin centavos, estado, orden); application **96,0 %**: categorías (duplicado sin distinguir mayúsculas, edición con campos auditados, no-op, borrar deja servicios sin categoría), servicios ("Corte + barba" 45 min 35.000 COP, moneda del negocio, campos requeridos, categoría ajena → `CATEGORY_NOT_FOUND`, quitar categoría, no-op), barbero lee pero 403 al gestionar, extraño 404, admin, negocio inexistente |
| `TestCurrencyOf` (businesses) | ✅ | COP; negocio inexistente → `ErrBusinessNotFound` |
| `TestCatalogOnPostgres` | ✅ | Moneda COP desde el negocio; nombres únicos por negocio sin distinguir mayúsculas y reutilizables en otro negocio; categoría ajena rechazada (aplicación y **FK compuesta**); CHECK de duración y precio; borrar categoría → servicio sin categoría; **6 altas simultáneas del mismo nombre (mayúsculas mezcladas) → 1 servicio**; 5 entradas de auditoría |
| `TestEveryRouteIsDocumented` / `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **57 rutas documentadas** (antes de documentar, la prueba detectó las 8 rutas nuevas); **24 operaciones `{businessId}`** aisladas con IDs reales de categoría y servicio; el catálogo de Bob no cambia; falla si queda un placeholder sin resolver |
| `TestContractCatalog` | ✅ | 8 operaciones contra OpenAPI: 200/201/204/400/401/403/404/409 |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #44 — ci + security | ✅ 6/6 | Squash `59447a3` |
| `release-qa` | ✅ | Migración 14 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **126/126** | Categoría y duplicado 409; "Corte + barba" 45 min 35.000 COP; centavos 400; barbero 403 al crear y lista; cambio de precio |

## 2026-11-24 · Fase 5 · T5.6 — Servicios que presta cada profesional

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/catalog/...` | ✅ | application **94,6 %**: asignar "Corte" y "Corte + barba" (duplicados ignorados, orden del catálogo), no-op sin auditoría, barbero lee y 403 al asignar, servicio ajeno/inexistente → `SERVICE_NOT_FOUND` por posición, >200 → `TOO_MANY_SERVICES`, profesional inexistente o de otro negocio 404, extraño 404, lista vacía |
| `TestIsProfessionalOf` (professionals) | ✅ | Propio sí; otro negocio no |
| `TestProfessionalServicesOnPostgres` | ✅ | Asignación y lectura del barbero; servicio ajeno rechazado (aplicación y **FK compuestas** de servicio y profesional); **8 reemplazos concurrentes → el resultado es exactamente una de las listas** |
| Prueba de mutación del bloqueo | ✅ detecta | Sin `pg_advisory_xact_lock` la prueba falla 3/3 (`service_professionals_pkey` duplicada al mezclarse); restaurado |
| `TestEveryRouteIsDocumented` / `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **59 rutas documentadas**; **26 operaciones `{businessId}`** aisladas; las asignaciones de Bob no cambian |
| `TestContractCatalog` | ✅ | `GET/PUT …/professionals/{id}/services`: 200/400/403/404 |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | govulncheck tras reintento por DNS local |
| PR #45 — ci + security | ✅ 6/6 | Squash `1f9c95b` |
| `release-qa` | ✅ | Migración 15 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ⚠️ **129/130** | Falla del script, no de la API: `check` compara con expresión regular y el `+` de "Corte + barba" es comodín (got "Corte + barba", want "Corte + barba"). Corregido en T5.7 (`Corte \+ barba`) |

## 2026-11-24 · Fase 5 · T5.7 — Vista pública de profesionales y servicios

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPublicCatalog` (catalog) | ✅ | Solo servicios activos prestados por profesionales reservables (excluye el que solo presta un no reservable, el que nadie presta y el inactivo); orden de profesionales; categoría inactiva → sin categoría; categorías en uso; slug desconocido 404; `BookableAmong` (con oferta / prestado solo por no reservable / vacío); falla explícita sin dependencias públicas |
| `TestBookableProfessionals` (professionals) | ✅ | Excluye perfil INACTIVE y barbero retirado; solo datos públicos |
| `TestSearchPublicRequiresSomethingToBook` / `TestPublicBusinessID` (businesses) | ✅ | Demo vigente **y** oferta; error del catálogo no se oculta; slug público / vencido / inexistente / inválido |
| `TestPublicCatalogOnPostgres` | ✅ | Módulos reales: sin oferta no aparece; servicio sin barbero no aparece; tras asignar aparece; catálogo público con precio y barbero; al retirarse el barbero desaparece de la búsqueda y del catálogo |
| `TestPublicSearchExposesOnlyBookableBusinesses` (seguridad) | ✅ | Negocio sin barberos oculto; catálogo público sin campos internos; suspendido → 404 idéntico a inexistente |
| `TestContractPublicBusinesses` / `TestEveryRouteIsDocumented` | ✅ | **60 rutas documentadas**; catálogo público contra OpenAPI 200/404 |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #46 — ci + security | ✅ 6/6 | Squash `acceb44` |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **135/135** | Negocio nuevo no aparece en la búsqueda (0); tras asignar "Corte + barba" al barbero aparece (1), sin campos internos; catálogo público 200 con "Corte + barba 4000000 Barbero E2E" y sin campos internos; slug inexistente 404 |

## 2026-11-24 · Fase 5 · T5.8 — Suite de aislamiento y cierre de la Fase 5

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **26** operaciones `{businessId}` contra otro negocio + **16** operaciones con ids de hijos de otro negocio usados en el propio (404 idéntico a inexistente); asignar un servicio ajeno → 400 `SERVICE_NOT_FOUND`; servicio bajo categoría ajena rechazado; nada cambió en el negocio ajeno |
| Mutación: `ServiceByID` sin filtro por negocio | ✅ detectada | La prueba falla con 200 en GET y PATCH del servicio ajeno; código restaurado |
| `task check` (build, vet, lint 0, test, vuln) | ✅ | govulncheck: un fallo DNS, reintento sin vulnerabilidades |
| `task test:integration` | ✅ | 36 paquetes, 0 fallos |
| Cobertura | ✅ | professionals domain 99,2 % / application 94,1 %; catalog domain 100 % / application 93,9 %; businesses domain 95,1 % / application 90,4 %; identity application 83,6 % |
| PR #47 — ci + security | ✅ 6/6 | Squash `7a04094` |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **135/135** | Flujo completo del responsable |

## 2026-11-24 · Fase 6 · T6.1 — Motor de disponibilidad

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestAvailable` (11 casos) | ✅ | Ejemplo del diseño; preparación nueva y existente; la preparación no necesita caber antes del cierre; descanso con rejilla anclada; ausencia parcial y total; antelación mínima; dos franjas sin cruzar el hueco; servicio más largo que el día; día sin horario |
| `TestAvailableBookingWindow` | ✅ | Ventana en el calendario local (23:30 en Bogotá ya es mañana en UTC); hoy y hoy+60 válidos; ayer y hoy+61 rechazados; entradas inválidas |
| `TestAvailableAcrossDST` | ✅ | Nueva York (marzo: 01:00–04:00 = 2 h reales; franja que empieza en el hueco arranca a las 03:00; noviembre: 01:00 ofrecida dos veces como instantes distintos) y Madrid (marzo y octubre) |
| `TestResolveLocal` | ✅ | Cada media hora de 2026 en Bogotá, Ciudad de México, Santiago, Nueva York y Madrid: nunca retrocede; hora ambigua → primera ocurrencia; hueco → salto (≤ 1 h); huecos solo en zonas con DST |
| `TestAvailableMatchesNaive` | ✅ | 3000 días aleatorios (semilla fija) idénticos a una referencia minuto a minuto |
| Mutaciones (ignorar la preparación de la cita existente; rejilla anclada al segmento) | ✅ detectadas | Ambas hacen fallar `TestAvailable` y `TestAvailableMatchesNaive`; código restaurado |
| Cobertura `scheduling/domain` | ✅ 100 % | |
| `task check` (lint 0, vuln) | ✅ | |

## 2026-11-24 · Fase 6 · T6.2 — Disponibilidad pública

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPublicAvailability` (scheduling) | ✅ | Contratos llamados con los ids correctos; ausencias leídas del día local completo en un día de DST (Nueva York, 23 h); horas con ausencia descontada; 404 único para negocio, servicio y barbero; fuera de ventana; errores inesperados propagados; zona horaria inválida no se disfraza de 404. Cobertura 96,8 % |
| `TestBookableService` (catalog) | ✅ | Duración del servicio; 404 si no lo presta, inactivo, barbero no reservable, servicio inexistente u otro negocio; falla explícita sin dependencias |
| `TestDayAgenda` (professionals) | ✅ | Franjas y descansos del día en minutos; ausencias del periodo (excluye la de otro día); día libre vacío; otro negocio → `ErrProfessionalNotFound` |
| `TestPublicBookingPolicy` (businesses) | ✅ | Política y zona del negocio público; vencido / inexistente / inválido → `ErrBusinessNotFound` |
| `TestContractPublicAvailability` | ✅ | Contra OpenAPI: 09:00–09:30, 10:30, 11:30 (descanso y ausencia); día sin horario `[]`; ayer y hoy+61 → 400; 404 negocio / servicio no asignado / inexistente / barbero INACTIVE |
| `TestEveryRouteIsDocumented` | ✅ | **61 rutas documentadas** |
| `TestPublicSearchExposesOnlyBookableBusinesses` (seguridad) | ✅ | Disponibilidad solo con campos públicos; suspendido, y servicio o barbero de otro negocio bajo un slug visible → 404 idéntico a inexistente |
| `TestPublicAvailabilityOnPostgres` | ✅ | Módulos y SQL reales: no asignado → 404; tras asignar 08:00–08:30 y 09:20–09:30 (descanso; huecos cortos por la ausencia descartados); ausencia del día siguiente no cuenta; barbero retirado → 404 |
| Postman + Newman (entorno local temporal) | ✅ 3 corridas | 22 peticiones, 35 comprobaciones, 0 fallos (cuenta nueva y cuenta existente); horas 09:00–12:30, 14:00–14:30, 16:00–17:30 |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #49 — ci + security | ✅ 6/6 | Squash `d90bd71` |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ⚠️ **142/143** | Falla del script, no de la API: esperaba 46 horas cada 10 min, pero el mismo E2E cambia el intervalo a 15; la API respondió 32 (10:00–12:15 y 14:00–19:15), correcto. Corregido en T6.3 |

## 2026-11-24 · Fase 6 · T6.3 — Reserva del cliente

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestBook` / `TestBookRejections` (scheduling, en memoria) | ✅ | Snapshot, evento CREATED, bloqueo tomado; misma clave → misma cita; clave con otro cuerpo → 422; la clave es por cuenta; preparación de la cita existente respetada; fuera de rejilla / horario / ventana; sin teléfono; clave inválida; 404 negocio / servicio / barbero; un intento fallido no consume la clave |
| `TestNewAppointment` (dominio) | ✅ | Fin y ocupación con preparación; validaciones; estados que ocupan agenda. Dominio 100 % |
| `TestPhoneAndCustomerProfile` (identity) | ✅ | Normalización (+57 para móvil colombiano, E.164 extranjero), inválidos, misma cifra sin nueva auditoría, perfil del cliente, suspendido |
| `TestContractBooking` (contra OpenAPI) | ✅ | 422 sin teléfono; teléfono 400/200; `/me.phone`; sin clave 400; 201; repetición; 422 clave reutilizada; 409 ocupado; 400 pasado; 404; 401; la disponibilidad ya no ofrece lo reservado. **63 rutas documentadas** |
| `TestBookingOnPostgres` | ✅ | Módulos y SQL reales: snapshot sobrevive a un cambio de precio; evento; repetición; disponibilidad sin el tramo reservado; 409 solapada |
| `TestNoDoubleBookingUnderConcurrency` | ✅ | **50 clientes, mismo horario → 1 éxito, 49 × 409**; 20 solapadas (10:00/10:20) → 1; misma clave ×10 en paralelo → 1 cita, todas las respuestas con su id |
| `TestExclusionConstraintIsTheSafetyNet` | ✅ | Insertando sin bloqueo ni motor: dentro de la preparación → rechazo; justo después → OK; una cancelada no ocupa |
| Mutación: sin bloqueo (solo constraint) | ✅ pasa | La restricción sola impide la doble reserva |
| Mutación: sin constraint (solo bloqueo) | ✅ concurrencia pasa | El bloqueo solo impide la doble reserva; la prueba directa de la restricción falla, como debe |
| Mutación: sin bloqueo ni constraint | ✅ detectada | 2 y 16 reservas dobles: la prueba falla |
| `TestAuthenticatedRoutesRejectInvalidCredentials` | ✅ | Ahora desde OpenAPI: **48 operaciones autenticadas** × 11 credenciales inválidas → 401 (antes la lista manual omitía la Fase 5) |
| `TestPublicSearchExposesOnlyBookableBusinesses` | ✅ | Reservar en negocio suspendido, o con servicio/barbero de otro negocio → 404 idéntico a inexistente; una reserva válida → 201 |
| Postman + Newman (entorno local temporal) | ✅ 2 corridas | 28 peticiones, 49 comprobaciones, 0 fallos (cuenta nueva y repetida; la 2.ª reservó 09:30 porque 09:00 estaba tomada) |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #50 — ci + security | ✅ 6/6 | Squash `c05c951` |
| `release-qa` | ✅ | Migración 16 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **157/157** | Teléfono, reserva 201, repetición con la misma clave, 422 clave reutilizada, 409 hora tomada, 401 sin sesión, disponibilidad sin el tramo reservado |

## 2026-11-24 · Fase 6 · T6.4 — Clientes del negocio y citas manuales (y celular en el registro)

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestRegistrationRequiresPhone` (identity) | ✅ | Sin celular / vacío → `REQUIRED`; inválido → `INVALID_PHONE`; ninguna cuenta creada; guardado en E.164 |
| `TestPhoneAndCustomerProfile` (identity) | ✅ | El celular viene del registro; cuenta antigua sin celular → `ErrPhoneRequired` |
| `TestCustomers` (scheduling) | ✅ | Validaciones; el barbero crea; celular duplicado 409; ajeno → 404; búsqueda por nombre, apellido con tilde, celular con espacios o +57; `%` y `_` literales; edición; celular tomado; inexistente |
| `TestBookManual` (scheduling) | ✅ | OWNER confirmada sin antelación mínima; repetición; barbero en su agenda (pendiente); barbero en agenda ajena → 403; solapada / pasada → 409; cliente inexistente y campos faltantes → 400; clave inválida; ajeno → 404 |
| `TestAppBookingJoinsTheCustomerList` | ✅ | Reserva desde la app crea el cliente o enlaza el que tenía ese celular; sin duplicados |
| `TestParsePhoneAndName` (dominio) | ✅ | Dominio 100 % |
| `TestContractStaffBooking` / `TestContractIdentityEndpoints` | ✅ | Clientes y citas manuales contra OpenAPI (201/400/403/404/409); registro sin celular → 400. **67 rutas documentadas** |
| `TestManualBookingOnPostgres` | ✅ | SQL real: búsqueda con `%` literal, celular único; cita manual con su cliente; la reserva desde la app aparece en la lista; **cita manual + 10 reservas desde la app por la misma hora → 1** |
| `TestEveryBusinessRouteIsTenantIsolated` | ✅ | **30 operaciones** con otro negocio y **17** con ids de hijos ajenos (incluye `{customerId}`) → 404 idéntico; la lista de clientes de Bob no cambia |
| `TestAuthenticatedRoutesRejectInvalidCredentials` | ✅ | Desde OpenAPI (52 operaciones autenticadas) |
| `TestBookManualQuickWay` (scheduling) | ✅ | Cliente escrito en la cita: nombre normalizado, celular E.164, correo en minúsculas; celular existente reutilizado (correo completado solo si faltaba, nunca sobrescrito); sin duplicados; `customer` + `customerId` / sin cliente / sin celular / correo inválido / sin nombre → 400 con el campo `customer.*`; una cita fallida no deja cliente |
| `TestParsePhoneAndName` (dominio, con correo) | ✅ | Correo opcional, minúsculas, formas inválidas. Dominio 100 % |
| `TestContractStaffBooking` (forma rápida) / `TestManualBookingOnPostgres` (forma rápida) | ✅ | 201 con cliente escrito y búsqueda con su correo; 400 `customer.phone`; en PostgreSQL el cliente con correo se guarda y una cita solapada no deja cliente |
| Postman + Newman (entorno local temporal) | ✅ 2 corridas | 33 peticiones, 60 comprobaciones, 0 fallos (paso 9: cita manual rápida; la 2.ª corrida reutiliza el cliente); registro sin celular → 400 |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #51 — ci + security | ✅ 6/6 | Squash `15ac654` |
| `release-qa` | ✅ | Migración 17 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **166/166** | Registro sin celular 400; cita manual rápida (nombre, celular, correo); cliente creado con correo; cliente de la app en la lista; 403 agenda ajena; 409; confirmada por defecto |

## 2026-11-24 · Fase 6 · T6.5 — Agenda y mis citas

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestViewRange` (dominio) | ✅ | Día, semana (lunes a domingo, cruzando año), mes, año; lunes y domingo. Dominio 100 % |
| `TestAgenda` (scheduling) | ✅ | Responsable ve todos / filtra uno; barbero ve la suya (o la pide); otra → 403; miembro sin perfil → vacía; ajeno → 404; semana / mes / año / otro día; barbero desconocido, cursor inválido, límite > 500 → 400; paginación de a 1 sin repetir; sin fecha → hoy del negocio |
| `TestMyAppointments` (scheduling) | ✅ | Próximas en orden, pasadas, con negocio y barbero; paginación; detalle propio; otro cliente / inexistente → no encontrado |
| `TestAgendaOnPostgres` | ✅ | SQL real: día / filtro / semana / mes / año (con una cita pasada) / día siguiente; paginación por cursor con nombres; "mis citas" próximas y pasadas con el nombre real del negocio; cita de otro cliente → no encontrada |
| `TestContractAgenda` (contra OpenAPI) | ✅ | Semana del responsable (2 barberos, `from`/`to`, hora local); su propia agenda; mes del barbero; 403 agenda ajena; cliente → 404; vista inválida 400; `nextCursor`; mis citas con negocio y barbero; `scope` inválido 400; cita de otro cliente → 404 idéntico a inexistente. **70 rutas documentadas** |
| `TestEveryBusinessRouteIsTenantIsolated` / credenciales | ✅ | La agenda entra sola a la suite de aislamiento; las rutas `/me/appointments` a la matriz de credenciales |
| Postman + Newman (entorno local temporal) | ✅ 2 corridas | 40 peticiones, 72 comprobaciones, 0 fallos; la consola muestra la agenda del día (hora, barbero, servicio, cliente, celular, estado) |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #52 — ci + security | ✅ 6/6 | Squash `66d3020` |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **174/174** | Agenda del responsable y del barbero, 403 agenda ajena, paginación, mis citas con el negocio, detalle, 404 cita ajena |

## 2026-11-24 · Fase 6 · T6.6 — Cambios de estado de la cita

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestTransitions` (dominio) | ✅ | Cada acción desde cada uno de los 6 estados (permitidas y 409); 15 min para iniciar; completar y "no vino" desde la hora; cancelar en la hora → empezada; límite del cliente exacto; mover: cliente sin cambio de estado, negocio → pendiente con cuenta / confirmada sin cuenta; motivo normalizado e inválido. Dominio 99,4 % |
| `TestCustomerChanges` | ✅ | Confirmar (versión 2) y repetir sin cambio; cita ajena → no encontrada; el cliente no puede iniciar; mover sobre su propio horario (09:00 → 09:10); sobre otra cita → 409; límite de 2 h para cancelar y mover; cancelar con motivo y repetir |
| `TestStaffChanges` | ✅ | Cancelar sin motivo → 400; agenda ajena → 403; otro negocio → 404; mover cliente sin cuenta (confirmada) y con cuenta (pendiente); solo el responsable reasigna; historial CREATED → CONFIRMED → RESCHEDULED → REASSIGNED con hora y barbero anteriores; el barbero ya no ve el historial de una cita reasignada a otra agenda; iniciar 20 min antes → 409; "no vino" libera y se agenda otra a las 09:10; iniciar, completar; cancelar completada → 409 |
| `TestAppointmentChangesOnPostgres` | ✅ | 8 rondas cancelar ∥ reprogramar la misma cita: estado y versión coherentes, un evento por cambio; 5 rondas de reasignaciones cruzadas Juan ↔ Pedro en paralelo sin deadlock; reprogramación ∥ 6 reservas por las 10:00 → 1 gana; "no vino" libera el horario (10:50 rechazada antes, aceptada después) |
| Mutación: sin orden fijo de bloqueos al mover | ✅ detectada | `deadlock detected (SQLSTATE 40P01)` en la 1.ª ronda; código restaurado |
| `TestContractAppointmentChanges` (contra OpenAPI) | ✅ | Cliente: confirmar, ajena 404, reprogramar; personal: reprogramar → pendiente, cancelar sin motivo 400, agenda ajena 403, reasignar solo responsable, historial; `CUTOFF_PASSED`, `TOO_EARLY`, iniciar y completar, `INVALID_TRANSITION`, `APPOINTMENT_STARTED`, "no vino". **80 rutas documentadas** |
| `TestEveryBusinessRouteIsTenantIsolated` / credenciales | ✅ | **38 operaciones** con otro negocio y **24** con ids ajenos (incluye `{appointmentId}`); la agenda y el historial de Bob no cambian; **65 operaciones autenticadas** |
| Postman + Newman (entorno local temporal) | ✅ 2 corridas | 48 peticiones, 86 comprobaciones, 0 fallos |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #53 — ci + security | ✅ 6/6 | Squash `3106e54` |
| `release-qa` | ✅ | Migración 18 aplicada, smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **185/185** | Confirmar y repetir, reprogramar cliente (sigue confirmada) y barbero (pendiente), historial, 400 sin motivo, 409 `TOO_EARLY`, cancelar con motivo y repetir, confirmar cancelada 409 |

## 2026-11-24 · Fase 6 · T6.7 — Coherencia de la agenda

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestAgendaChangesAgainstAppointments` (professionals) | ✅ | Ausencia sobre una cita → `ConflictError` con la lista y sin auditoría; justo después → OK; estirarla encima → rechazo; cambiar solo el motivo → OK; horario sin el martes → rechazo; horario que lo conserva → OK |
| `TestAgendaGuard` (scheduling) | ✅ | Cruce con el servicio (no con la preparación); fuera de horario: inicio tardío, descanso encima, día libre, franja partida; justo al borde → OK; una cancelada no cuenta |
| `TestToReassign` (scheduling) | ✅ | Vacía mientras el barbero es reservable; al irse, su cita aparece con `needsReassignment` (también en la agenda); barbero → 403, ajeno → 404; tras reasignar → vacía |
| `TestAgendaCoherenceOnPostgres` | ✅ | **Reserva ∥ ausencia sobre la misma hora, 10 rondas: exactamente una gana** (con pausa de 100 ms entre verificación y escritura); horario que deja una cita fuera → rechazo con la cita; barbero INACTIVE → por reasignar → reasignada a Pedro → vacía |
| Mutación: ausencias sin el bloqueo del profesional | ✅ detectada | "booking <nil>, absence <nil>" en la 1.ª ronda; código restaurado. (Sin la pausa la carrera no se reproducía: se corrigió la prueba) |
| `TestContractAgendaCoherence` (contra OpenAPI) | ✅ | 409 `AGENDA_CONFLICTS` con `conflicts`; ausencia sin cruce 201; horario 409; por reasignar vacío / con la cita tras desactivar; bandera en la agenda; no miembro 404. **81 rutas documentadas** |
| `TestEveryBusinessRouteIsTenantIsolated` / credenciales | ✅ | 39 operaciones con otro negocio y 24 con ids ajenos; 66 operaciones autenticadas |
| Postman + Newman (entorno local temporal) | ✅ 2 corridas | 53 peticiones, 93 comprobaciones, 0 fallos |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #54 — ci + security | ✅ 6/6 | Squash `b112f37` |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **194/194** | Ausencia sobre una cita → 409 `AGENDA_CONFLICTS` con la cita; el barbero quita el martes → 409; desactivar → sus citas por reasignar; reactivar → vacío |

## 2026-11-24 · Fase 6 · T6.8 — Cierre de la Fase 6

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| Revisión de las 7 pruebas de concurrencia (`availability-engine.md` §7) | ✅ 7/7 | Ver la tabla en `docs/plan/phase-6-closure.md` §2; en CI la integración corre con `-race` |
| `TestConcurrentConfirmationsOnPostgres` (nueva) | ✅ | 12 confirmaciones simultáneas (cliente y personal): todas OK, versión 2, un solo evento CONFIRMED |
| `TestBookingAcrossDST` (nueva) | ✅ | Negocio en Nueva York: primavera 01:00–05:00 = 3 horas reales (sin 02:00), reserva 03:00 EDT; otoño 00:00–03:00 = 4 horas, las dos 01:00 (EDT y EST) son instantes distintos y ambas se reservan; sin horas duplicadas |
| `TestBookingPoliciesAndSummaries` (nueva, negocios) | ✅ | Política pública por id (vencido / inexistente → no encontrado), política del personal aunque no sea público, resúmenes sin duplicados. Aplicación de negocios 81,9 % → 90,2 % |
| Aislamiento / IDOR / credenciales | ✅ | 39 operaciones con otro negocio, 24 con ids ajenos, 66 autenticadas (guiadas por el contrato) |
| `task check` (lint 0, vuln) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #55 — ci + security | ✅ 6/6 | Squash `f7ce6f9` |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **194/194** | |

## 2026-11-24 · Fase 7 · T7.1 — Base de notificaciones

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPlan` / `TestStale` / `TestMaskPhone` (dominio) | ✅ 100 % | Mensaje solo para creada, reprogramada, reasignada y cancelada por el negocio; descarte por cita empezada, cancelada o movida (una confirmación no invalida); número enmascarado |
| Pruebas de aplicación (`notifications`) | ✅ 92,2 % | Un mensaje y un job por evento, evento repetido sin duplicar; envío y job repetido sin reenviar; descartes (canal apagado, fuera de lista blanca, cita inexistente, cancelada, movida); error temporal → reintento, permanente o último intento → `FAILED` |
| `TestEveryChangeTellsTheListener` / `TestAppointmentsContract` (scheduling) | ✅ | Reserva, cita manual (sin cuenta), reasignación y cancelación del negocio llegan al listener; un listener que falla deshace la reserva; estado de la cita y acceso como el historial |
| `TestNotificationsOnPostgres` | ✅ | Mensaje y job en la transacción de la reserva; enviado una sola vez; creada y luego movida → la primera `SKIPPED APPOINTMENT_CHANGED`, la nueva `SENT`; un cambio que falla después no deja mensajes ni jobs |
| `TestNotificationsWorkerOnPostgres` | ✅ | El worker River real entrega el mensaje (`SENT`) |
| Mutación: el store no se une a la transacción del cambio | ✅ detectada | Falla la inserción (no ve el evento); código restaurado |
| `TestContractAppointmentNotifications` (contra OpenAPI) | ✅ | Mensaje `PENDING` con número enmascarado; no miembro 404. **82 rutas documentadas** |
| Aislamiento / credenciales (guiadas por el contrato) | ✅ | 40 operaciones con otro negocio, 25 con ids ajenos; 67 autenticadas |
| `TestNotificationsConfig` | ✅ | Valores por defecto, lista E.164, booleano inválido y número inválido sin repetir el valor |
| `task check` (lint 0; `govulncheck` sin vulnerabilidades, tras reintentar por DNS) / `sqlc:check` / `shellcheck` / `secrets` / integración completa | ✅ | |
| PR #56 — ci + security | ✅ 6/6 | Squash `20ed86f`; `-race` en CI |
| `release-qa` | ✅ | Smoke test ✅ |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **197/197** | Mensajes de la cita: creada y las dos reprogramaciones (la cancelación del cliente no genera); WhatsApp al cliente con el número enmascarado; el worker los procesó y el último quedó `SENT` (envío simulado). Dos corridas previas tuvieron fallos `000` (DNS intermitente de la máquina local, la petición no salió); con `CURL_EXTRA=--resolve …` 0 fallos |

## 2026-11-25 · Fase 7 · T7.2 — Cliente de WhatsApp Cloud API

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestSendTemplate` (Meta falso) | ✅ | Ruta `/v23.0/{phone-number-id}/messages`, `Bearer`, plantilla `hello_world` `en_US`, número sin `+`; devuelve el wamid |
| `TestSendFailures` | ✅ | 11 respuestas de Meta → código y reintento esperados (token, límites, destinatario no permitido, no disponible, plantilla, caída, permiso, parámetro, 200 sin id); ningún error contiene el token ni el número aunque Meta lo cite |
| `TestSendUnreachable` | ✅ | Sin conexión y timeout → reintento, sin la URL en el error; endpoint inválido y tipo sin plantilla → permanentes. Cobertura 94,1 % |
| `TestWhatsAppCloudAPIConfig` | ✅ | Valores por defecto; QA con token sin lista blanca → error; PROD sin lista → OK; formatos inválidos reportados sin repetir el token; http solo fuera de QA/PROD |
| `TestWhatsAppSenderChoice` | ✅ | Sin credenciales → simulado; con credenciales → Meta; el log no tiene token ni número |
| `TestWhatsAppClientOnPostgres` | ✅ | Reserva → mensaje → cliente de Meta (falso) → `SENT` con el wamid; Meta rechaza al destinatario → `FAILED RECIPIENT_NOT_ALLOWED` en el primer intento |
| `docker compose config` (QA) | ✅ | Credenciales solo en el worker |
| `task check` (lint 0, sin vulnerabilidades) / `shellcheck` / integración completa | ✅ | |
| PR #57 — ci + security | ✅ 6/6 | 1.ª corrida: gitleaks marcó el token **falso** de una prueba (`AccessToken: "EAAtest…"`, regla genérica); corregido con una constante y la huella revisada en `.gitleaksignore`. Squash `d9b29bb` |
| `release-qa` | ✅ | El worker arranca con `whatsapp sender provider=meta allowlist=1` |
| `scripts/e2e-qa.sh` (admin, `WHATSAPP_TEST_PHONE`) en QA | ✅ **203/203** | Mensajes sintéticos → `SKIPPED NOT_ALLOWLISTED`; **cita con el celular de prueba → Meta aceptó el WhatsApp (`SENT`, 1 intento)**; la cancelación del cliente no envía un segundo mensaje. El número no aparece en el log |

## 2026-11-25 · Fase 7 · T7.3 — Mensajes de la cita

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPlan` / `TestWhenES` (dominio) | ✅ 100 % | La cita del negocio es `APPOINTMENT_CREATED_BY_BUSINESS`; fecha en español con a. m./p. m., mediodía, medianoche, cambio de día por zona |
| `TestTemplates` / `TestParamsAreSafeForMeta` (cliente) | ✅ 94,4 % | Nombre, idioma `es` y orden de los 5 parámetros de cada plantilla; limpieza de parámetros; motivo vacío |
| `TestSendFillsTheContent` (aplicación) | ✅ 92,6 % | Nombres actuales y la hora en la zona del negocio; zona inválida → UTC |
| `TestAppointmentsContract` (scheduling) | ✅ | El estado trae cliente, servicio, negocio, zona y barbero |
| `TestWhatsAppClientOnPostgres` | ✅ | Con PostgreSQL real: `cita_agendada/es: … | Barbería Reservas | Corte | Juan | martes 5 de marzo, 9:00 a. m.`; cita manual → `cita_agendada_negocio` con el cliente escrito |
| `TestAppDownload` / `TestAppDownloadURLConfig` | ✅ | Aviso HTML sin URL; 302 con URL; solo https |
| Migración 00020 (up/down) / `sqlc:check` | ✅ | |
| `task check` (lint 0, sin vulnerabilidades) / `shellcheck` / integración completa | ✅ | 83 rutas documentadas |
| PR #58 — ci + security | ✅ 6/6 | Squash `641e69c` (merge autorizado sin esperar la aprobación de Meta) |
| `release-qa` | ✅ | Worker con `provider=meta` |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **198/198** | `/app` → 200 con el aviso "muy pronto"; mensajes sintéticos `SKIPPED NOT_ALLOWLISTED`. Sin la sección de envío real: las plantillas siguen "En revisión" en Meta |
| E2E con `WHATSAPP_TEST_PHONE` (plantilla real a tu celular) | ⏳ | Cuando Meta apruebe las plantillas |

## 2026-11-25 · Fase 7 · T7.4 — Recordatorios

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPlanReminder` / `TestStaleReminder` (dominio) | ✅ 100 % | Pendiente → recordatorio 24 h antes; dentro de la ventana → inmediato; reprogramada/reasignada pendiente → nuevo; confirmada, desactivado, cancelación, confirmación o ya empezada → ninguno. Al vencer: confirmada → `APPOINTMENT_CONFIRMED`, cancelada, movida |
| `TestReminders` (aplicación) | ✅ 92,3 % | Mensaje y job del recordatorio; confirmada al vencer → descartado; cambios sin recordatorio no leen la configuración; configuración ilegible → el cambio falla |
| `TestTemplates` (cliente) | ✅ | `recordatorio_cita` con sus 5 parámetros (la prueba exige probar toda plantilla del mapa) |
| `TestRemindersOnPostgres` | ✅ | Job River `scheduled` exactamente 24 h antes del inicio; confirmada → `SKIPPED APPOINTMENT_CONFIRMED`; cita manual confirmada → sin recordatorio |
| Pruebas de T7.1/T7.2 con recordatorios | ✅ | Encontraron que los avisos inmediatos se programaban con la hora del cambio (reloj fijo de prueba → job en el futuro): ahora solo se programa lo que vence después |
| Expresión jq del E2E (vencimiento del recordatorio) | ✅ | Probada con datos de ejemplo: 24 h antes → `true`; inmediato dentro de la ventana → `true`; incoherente → `false` |
| `task check` (lint 0; `govulncheck` tras reintento por DNS) / `sqlc:check` / `shellcheck` / integración completa (migración 00021 up/down) | ✅ | |
| PR #59 — ci + security | ✅ 6/6 | Squash `195faad` |
| `release-qa` | ✅ | Migración 00021 aplicada |
| `scripts/e2e-qa.sh` (admin) en QA | ✅ **200/200** | 2 recordatorios (al reservar y al mover el barbero; ninguno con la cita confirmada); el último vence 24 h antes; avisos procesados |

## 2026-11-25 · Fase 7 · T7.5 — Webhook de WhatsApp

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestWebhookVerification` / `TestWebhookReceive` (HTTP) | ✅ | Desafío correcto → 200 (solo número); token, modo o desafío inválidos → 403/400; sin configurar → 404. Firma válida → elementos (delivered, read, failed con código, botón); `sent`, textos y otros campos ignorados. Firma ausente, de otro secreto, de otro cuerpo, no hex o sin prefijo → 401 sin llegar al servicio; no JSON → 400; error interno → 500; > 1 MiB → 413 |
| Pruebas de los botones (aplicación) | ✅ 90,5 % | Códigos de 43 caracteres, solo hash guardado, uno por botón, vencen al inicio; confirmar y cancelar con su respuesta; el otro botón y el mismo otra vez → inválido; límite de cancelación, cita inactiva o inexistente, otro número, después del inicio, cita movida → respuesta adecuada sin aplicar; falla temporal → reintento sin gastar el código; webhook repetido → una vez; estados solo hacia adelante; respuesta descartada si el canal está apagado o fuera de la lista |
| `TestActionTokens` / `TestReplyTexts` (dominio) | ✅ 100 % | |
| `TestRepliesAsCustomer` (scheduling) | ✅ | Otro negocio → no encontrada; confirma con el actor del cliente; límite de 2 h → `ErrCutoffPassed`; cancelada → `ErrNotActive` |
| `TestReminderButtonsAndTextReplies` (cliente) | ✅ 94,9 % | Botones `quick_reply` con índice y payload; respuesta de texto |
| `TestWhatsAppButtonsOnPostgres` | ✅ | Recordatorio con botones por el cliente de Meta (falso) → webhook dos veces → 2 jobs (sin duplicados) → "Confirmar cita" confirma la cita y encola la respuesta; recordatorio `READ`; "Cancelar cita" ya gastado; el código crudo no se guarda |
| `TestWhatsAppButtonsConcurrently` | ✅ | **Confirmar ∥ Cancelar del mismo recordatorio, 5 rondas: exactamente uno se aplica** |
| Mutación: sin el bloqueo de los códigos del recordatorio | ✅ detectada | Se aplicaron los dos (cancelada y luego "ya no está activa") en la 1.ª ronda; código restaurado |
| `TestContractWhatsAppWebhook` | ✅ | Contra OpenAPI; **85 rutas documentadas** |
| `task check` (lint 0 tras corregir G705, sin vulnerabilidades) / `sqlc:check` / `shellcheck` / integración completa (migración 00022) | ✅ | |
| Despliegue y E2E en QA (PR #60, `8363924`) | ✅ 203/203 | CI 6/6 (incl. `-race` e integración); release-qa ✅; `scripts/e2e-qa.sh` 203 PASS, 0 FAIL: webhook con token equivocado → 403, sin firma → 401 `INVALID_SIGNATURE`; recordatorios 2; worker: avisos SKIPPED `NOT_ALLOWLISTED` (cliente de prueba fuera de la lista blanca) |
| Registro del webhook en Meta (parte B) | ✅ | `GET /webhooks/whatsapp` desde Meta → 200 (12:56 UTC); suscripción a `messages` |

## 2026-11-25 · Fase 7 · T7.6 — Correos de citas (Booka)

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPlan` / `TestPlanReminder` / `TestMaskEmail` / `TestDateAndTimeES` (dominio) | ✅ 100 % | Cada cambio: WhatsApp y correo al cliente; al barbero solo lo que hizo el cliente; la cancelación del cliente: comprobante por correo, sin WhatsApp; recordatorio por los dos canales |
| `TestEmailRecipients` / `TestEmailSwitchAndFailures` / `TestEmailReminderHasNoButtons` (aplicación) | ✅ 91,6 % | Correo de la ficha antes que el de la cuenta; barbero por su cuenta; sin dirección → `NO_RECIPIENT`; canal apagado → `CHANNEL_DISABLED`; fallas del directorio o de la política → reintento; el recordatorio por correo no emite códigos |
| `TestRenderEveryEmail` / `TestRenderEscapes` / `TestSender` (plantillas) | ✅ 95 % | Asuntos y textos aprobados; `<script>`, `<img onerror>` y saltos de línea en el asunto neutralizados; mapa por coordenadas o dirección; SMTP 5xx y dirección inválida → permanentes, el resto se reintenta |
| `TestBuildHTMLMessage` (SMTP) | ✅ | `multipart/alternative`: texto primero y HTML después, UTF-8 |
| `TestAccountEmail` (identity), `TestAppointmentsContract` (scheduling), `TestNotificationsConfig` | ✅ | Cuenta suspendida o inexistente → sin dirección; correo de la ficha de una cita manual |
| `TestAppointmentEmailsOnPostgres` | ✅ | Reserva del cliente → correo al cliente (dirección de su cuenta) y al barbero, direcciones guardadas, contenido real (estado, fecha, políticas, Booka); cancelación del cliente → correo a los dos, sin WhatsApp; recordatorio por correo de la cita cancelada → `SKIPPED` |
| Mutaciones: al barbero todo cambio / códigos también en el recordatorio por correo | ✅ detectadas | `TestPlan` y `TestEmailReminderHasNoButtons` fallaron; código restaurado |
| Vista previa en Chrome sin interfaz (correo del cliente y del barbero) | ✅ | Revisión visual del diseño |
| `task check` (lint 0, sin vulnerabilidades) / `sqlc:check` / `shellcheck` / integración completa (migración 00023) / gitleaks | ✅ | |
| Despliegue y E2E en QA (PR #61, `3cf9b43`) | ✅ 209/209 | CI 6/6; el primer despliegue falló por el droplet apagado (el usuario lo apagó para cambiar recursos), relanzado con éxito tras encenderlo; migración 00023 aplicada. Mailpit: al cliente "está agendada", dos "cambió de hora" y "fue cancelada"; al especialista "Nueva cita", "Cita reprogramada" y "Cita cancelada"; direcciones enmascaradas. Diseño aprobado por el usuario |

## 2026-11-25 · Integración frontend · portales web en QA

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `shellcheck -S style` (web-deploy.sh, qa-env.sh, deploy.sh) | ✅ | Tras corregir SC2015 en `env_value` |
| `caddy validate` (Caddyfile con los dos portales) | ✅ | "Valid configuration" |
| Compose web local con la imagen real de business-web en la red `booking-qa_default` | ✅ | `healthy`, raíz de solo lectura, caché en tmpfs escribible, `/healthz` por el alias `business-web:3000`, 116 MiB de 192 MiB |
| Despliegue real en QA | ⏳ | Droplet activo; tras el merge, pasos de operador (`qa-deployment.md` §3B) |

## 2026-11-25 · Integración frontend · slugs reservados (ADR-F006 del frontend)

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/modules/businesses/...` | ✅ | Nuevos casos: rutas del portal → `ErrReservedSlug`; "Contacto" → `negocio-contacto`; toda palabra reservada tiene forma de slug |
| `task check` (build, vet, lint 0 issues, test, govulncheck) | ✅ | govulncheck falló una vez por DNS local (`vuln.go.dev: no such host`); reintento: sin vulnerabilidades |
| Verificar en QA que ningún slug existente quede reservado | ✅ | 4 ciudades, 0 negocios públicos: sin conflictos (los no públicos no son visibles por la API pública) |

## 2026-11-25 · Fase 7 · T7.7 — Acciones desde el correo

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestEmailLinksIssued` / `TestLinkTwoTaps` / `TestLinkRejections` / `TestLinkReschedule` (aplicación) | ✅ 91,0 % | 3 enlaces si está pendiente, 2 si confirmada, ninguno al especialista ni en cancelaciones; el primer toque no cambia nada; confirmar gasta solo su enlace (luego se puede cancelar); código desconocido, de otra acción, usado, vencido, cita movida o inexistente → inválido; horario tomado o límite → el enlace sigue válido; reagendar gasta todos |
| `TestRenderButtons` (correo) | ✅ 95,8 % | Enlaces a `/citas/*` con el código escapado; reagendar al frontend si está configurado; sin URL base no hay botones; texto plano con los enlaces |
| `TestLinkPages` / `TestLinkPageOutcomes` / `TestLinksAPI` (HTTP) | ✅ 83,9 % | CSP, `no-referrer`, `no-store`, `noindex`; nombre con `<script>` escapado; sin código o > 100 caracteres → página inválida sin consultar el servicio; falla interna sin filtrar el error; validación de la API (acción, código, `startAt`), 415 sin JSON, enlace inválido sin datos de la cita |
| `TestContractAppointmentLinks` | ✅ | Contra OpenAPI: primer toque, segundo toque, reuso, `lookup` y `apply` (mueve la cita e invalida el resto). **92 rutas documentadas** |
| `TestEmailLinksOnPostgres` | ✅ | Primer toque sin cambios; confirmar; horario tomado → `SLOT_UNAVAILABLE`; reagendar como el cliente (evento con su cuenta) y aviso al especialista; códigos crudos no guardados |
| `TestEmailLinksConcurrently` | ✅ | **Cancelar ∥ Reagendar del mismo correo, 4 rondas: exactamente uno se aplica** |
| Mutaciones: confirmar gasta todos los enlaces / sin el bloqueo de los enlaces | ✅ detectadas | `TestLinkTwoTaps` falló; sin bloqueo se aplicaron los dos (cancelada y movida) en la 1.ª ronda; código restaurado |
| `TestEmailLinksConfig`, `TestAppointmentsContract` | ✅ | `PUBLIC_API_URL` https fuera de DEV; `APPOINTMENT_RESCHEDULE_URL` https sin query |
| Vista previa en Chrome sin interfaz (recordatorio con botones, páginas de confirmar, cancelar e inválido) | ✅ | |
| `task check` (lint 0, sin vulnerabilidades) / `sqlc:check` / `shellcheck` / gitleaks | ✅ | |
| Integración completa (migración 00024) | ✅ | Una 1.ª corrida falló en `identity/infrastructure` bajo carga (Docker con 2 GB); pasó 4/4 aislado y la suite completa pasó al repetirla |
| Despliegue y E2E en QA (PR #66) | ✅ 214/214 | CI 6/6 tras integrar `qa` (portales web, slugs reservados); `release-qa` ✅. Enlaces reales del correo en Mailpit: primer toque sin cambios, cabeceras de seguridad, segundo toque confirma, reuso → "ya no es válido", `lookup`/`apply` mueve la cita como el cliente, el enlace de cancelar queda gastado. 1.ª corrida: el script contaba 2 líneas con "ya no es válido" (título y encabezado) → `grep -m1`; dos `000` por red local. 2.ª corrida: un `000` en el login del barbero arrastró 83 fallos en cascada. 3.ª corrida con `CURL_EXTRA=--resolve …` (DNS local): 0 fallos |

## 2026-11-25 · Fase 7 · T7.8 — Límite de reprogramaciones por cita

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestPolicies` (negocios) | ✅ | Por defecto 2; 0 y 10 válidos; −1 y 11 → `bookingPolicy.maxCustomerReschedules` |
| `TestRescheduleLimit` (dominio) | ✅ | Dos cambios del cliente cuentan, el tercero → `ErrRescheduleLimit`; subir el límite o 0 lo permite; el negocio mueve sin límite y no cuenta; dentro del límite de horas se informa `CUTOFF_PASSED` primero |
| `TestCustomerRescheduleLimit` (aplicación) | ✅ | Límite 1: segundo cambio rechazado, también tras un cambio del dueño; cancelar sigue permitido |
| `TestLinkRescheduleLimit` / `TestLinkPageOutcomes` / `TestLinkPages` / `TestLinksAPI` | ✅ | Al límite: `lookup` y `apply` → `RESCHEDULE_LIMIT_REACHED` sin mover ni gastar (luego cancela o confirma); rechazo de scheduling → mismo resultado; correo nuevo sin "Reagendar"; página y nota "Te queda 1 cambio…"; JSON con `maxCustomerReschedules`/`customerReschedules` |
| `TestRescheduleLimitOnPostgres` | ✅ | Por defecto 2 en la base; contador guardado y leído; cambio del dueño no cuenta; **límite 1 y dos cambios del cliente a la vez, 4 rondas: exactamente uno se aplica**; migración 00025 abajo y arriba: el contador se reconstruye desde el historial (2, sin el cambio del dueño) |
| Mutaciones: `>=` → `>` en el límite / contador no guardado | ✅ detectadas | Fallaron `TestCustomerRescheduleLimit` y `TestRescheduleLimitOnPostgres` ("third move: <nil>"; "move 2"); código restaurado |
| `task check` (lint 0, sin vulnerabilidades) / `sqlc:check` / `shellcheck` | ✅ | |
| Integración completa (`task test:integration`) | ✅ | 45 paquetes |
| Despliegue y E2E en QA (PR #68) | ✅ 218/218 | CI 6/6; `release-qa` ✅ (migración 00025 aplicada). Solo cuenta el cambio del cliente (1, no el del barbero); el dueño fija 1 → segundo cambio del cliente 409 `RESCHEDULE_LIMIT_REACHED`; vuelve a 2. Con `CURL_EXTRA=--resolve …` (DNS local), 1.ª corrida sin fallos |

## 2026-11-25 · Despliegue QA · carrera en el smoke test de Caddy

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| Despliegue de #63 (`86a4dad`) | ❌ → rollback ✅ | Caddy recreado/reiniciado por el Caddyfile nuevo; `curl` a 443 "Couldn't connect … after 0 ms" en el mismo instante; el rollback dejó QA sano (`4437175`) |
| Corrección: reintentos de la verificación por Caddy (60 s) en `deploy.sh` | ✅ `bash -n` | shellcheck en CI (el contenedor local se colgó) |

## 2026-11-25 · Fase 7 · T7.9 — Preferencias de notificación y control de costos

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestCustomerPreferences` / `TestMonthAndDayOf` (dominio) | ✅ | Al menos un canal; WhatsApp apagado → `CUSTOMER_OPTED_OUT`; recordatorio sin recordatorios → `REMINDERS_OFF`; mes y día en UTC (31-dic 21:00 Bogotá ya es enero) |
| `TestPreferencesAtSend` (aplicación) | ✅ | Cliente solo correo y sin recordatorios: WhatsApp y recordatorios omitidos, correo enviado; correos del especialista apagados → `PROFESSIONAL_OPTED_OUT`; cliente sin elegir o sin cuenta → todo enviado |
| `TestWhatsAppCaps` | ✅ | Tope por defecto 1: el 2.º WhatsApp → `MONTHLY_LIMIT_REACHED`, el correo sale; tope propio 5 gana; tope propio 0 → sin WhatsApp; por defecto 0 → sin tope; tope diario 1 → `DAILY_LIMIT_REACHED` |
| `TestPreferenceUseCases` | ✅ | Sin canal → `ErrNoChannel`; especialista: dueño y el propio sí, otro especialista 403, no miembro y perfil desconocido 404, sin cuerpo y extraño → 404 (autoriza antes de validar); tope: solo admin, 0–100000, negocio desconocido 404 |
| `TestContractNotificationPreferences` (OpenAPI) | ✅ | Las 6 operaciones; 400, 401, 403 y 404 |
| Suite de seguridad: aislamiento entre negocios | ✅ | Detectó 400 en vez de 404 para un extraño (validación antes de autorizar) y 500 sin autorizador en la suite; corregidos |
| `TestPreferencesOnPostgres` | ✅ | Con SQL real: tope por defecto 1 → 2.º WhatsApp omitido; consumo del mes 1; tope propio 10; cliente sin WhatsApp → `CUSTOMER_OPTED_OUT`; preferencia del especialista guardada; restricciones: sin canales, perfil inexistente y tope 100001 rechazados |
| Mutaciones: sin el filtro al enviar / `>=` → `>` en el tope | ✅ detectadas | Fallaron `TestPreferencesAtSend` y `TestWhatsAppCaps`; código restaurado |
| `TestWhatsAppLimitsConfig` | ✅ | 500 y 0 por defecto; −1, 100001 y texto rechazados |
| `task check` (lint 0, sin vulnerabilidades) / `sqlc:check` / `shellcheck` | ✅ | govulncheck falló una vez por DNS local; reintento OK |
| Integración completa | ✅ | 45 paquetes. 1.ª corrida: `TestRescheduleLimitOnPostgres` suponía 00025 como última migración → ahora baja hasta deshacerla |
| Despliegue y E2E en QA (PR #70) | ✅ 231/231 | CI 6/6; `release-qa` ✅ (migración 00026). Preferencias: todo activo por defecto, sin canal → 400 `AT_LEAST_ONE_CHANNEL`; correo del cliente `CUSTOMER_OPTED_OUT`, correo del barbero `PROFESSIONAL_OPTED_OUT`, su WhatsApp no bloqueado por las preferencias (`NOT_ALLOWLISTED` por la lista blanca de QA); el tope solo para el admin (403). Las comprobaciones del admin no corrieron (sin `ADMIN_EMAIL`); cubiertas por las pruebas de contrato |

## 2026-11-25 · Fase 7 · T7.10 — Cierre de la fase

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestEmailLinksGiveNothingAway` (seguridad, handler compuesto) | ✅ | Cinco aperturas de cada enlace sin cambios; código desconocido, de otra acción, gastado, de un correo gastado y vencido → misma página y mismo JSON; URL con `<script>` no reflejada; segundo toque de un código vencido igual; ningún código en los logs; CSP y `no-referrer` |
| Mutación: código vencido responde `NOT_ACTIVE` | ✅ detectada | "an expired code answers differently"; código restaurado |
| `TestEmailLinksAreRateLimited` | ✅ | Con el limitador de `cmd/api`: 429 con `Retry-After` antes de 60 intentos |
| `TestWebhookRejectsForgeries` | ✅ | Sin firma, otra clave, cuerpo cambiado tras firmar, cabecera sha1, firma mal formada → 4xx y nada encolado; firmado → 1 evento; verify token incorrecto → 403 sin el reto |
| Suite de seguridad completa | ✅ | Aislamiento en 42 operaciones (27 con ids ajenos); credenciales inválidas en 73 operaciones autenticadas |
| Cobertura | ✅ | notifications dominio 100 %, aplicación 90,9 %, correo 96,0 %, WhatsApp 94,9 %, worker 94,4 %, HTTP 59,3 %; scheduling dominio 100 %, aplicación 85,6 %; negocios aplicación 90,3 %, dominio 95,1 % |
| Plantillas de WhatsApp en Meta (consulta desde el servidor, sin mostrar el token) | ⏳ | Las 6 de Booka `PENDING`; la prueba con mensajes reales espera la aprobación |

## 2026-11-26 · Fuera de fase · P-6 — Correos de identidad con la marca Booka

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `go test ./internal/platform/mail/brand` | ✅ | HTML y texto con título, saludo, botón, enlace como texto, notas y pie; marcado escapado (`<script>`, `<img onerror>`); sin botón no hay enlaces; `javascript:` y correo sin título rechazados. Cobertura 80,0 % |
| `TestAccountEmailsAreBranded` (identity) | ✅ | Verificación y recuperación: asunto sin token, HTML con botón y `href` al portal (`PUBLIC_APP_URL`), enlace completo en el texto, vigencia (24 horas / 30 minutos); contraseña cambiada: «19 de noviembre de 2026 a las 7:00 a. m. (hora de Colombia)» sin enlaces ni token. Cobertura de identity/application 84,0 % |
| Mutación: enviar sin HTML | ✅ detectada | 10 comprobaciones fallan; código restaurado |
| Pruebas existentes de identity, contrato, seguridad e integración (`-tags integration`, PostgreSQL real) | ✅ | Sin cambios de comportamiento |
| `task check` | ✅ | build, vet, golangci-lint, pruebas; govulncheck sin vulnerabilidades (reintento por DNS) |
| Despliegue y Mailpit de QA (PR #73) | ✅ | CI 6/6; `release-qa` ✅. Cuenta de prueba `p6-…@example.com`: registro 202 → «Confirma tu correo electrónico» en HTML + texto con la marca; verificar 200; olvido 202 → «Restablece tu contraseña» en HTML + texto; restablecer 200 → «Tu contraseña fue cambiada» sin enlaces, «26 de noviembre de 2026 a las 1:52 a. m. (hora de Colombia)». Token nunca en el asunto. Enlaces aún al dominio de la API: `/reset-password` del portal QA responde 404, `PUBLIC_APP_URL` sin cambiar |
| `PUBLIC_APP_URL` de QA → `https://booka-app-qa.duckdns.org` (autorizado por el usuario) | ✅ | `/verify-email` y `/reset-password` del portal → 200 («Crear una nueva contraseña · Booka»); `.env` con copia `.env.bak-p6`, `api` y `worker` recreados, `/readyz` ready. Repetido en Mailpit: los botones y el enlace en texto apuntan a `https://booka-app-qa.duckdns.org/verify-email` y `/reset-password`; verificar 200, restablecer 200. El token se usó contra la API, no desde la página del portal |

## 2026-11-26 · Fuera de fase · P-9 y P-10 — Horas libres del personal y citas de un cliente

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestStaffAvailability` | ✅ | Hoy con 60 min de anticipación: pública desde 13:00, personal desde 12:00 (6 horas más); el cliente no puede reservar "ahora" (409), la reserva manual de la primera hora del personal sí; la cita agendada bloquea su hora salvo con `excludeAppointmentId`; barbero con otro profesional → 403; ajeno → 404 también con parámetros inválidos; ayer y más allá del horizonte → `DATE_OUTSIDE_BOOKING_WINDOW`, último día del horizonte → 200; parámetros inválidos → 400; negocio sin publicar: pública 404, personal con las mismas horas |
| `TestCustomerAppointments` | ✅ | Dueño: 2 próximas / 2 pasadas; Juan: solo su agenda (1 / 2); Ana 1 / 0; miembro sin perfil 0; orden ascendente y descendente, nombres, cursor; otro negocio no aparece; cliente desconocido → 404; ajeno → 404; cursor y límite inválidos → 400. Cobertura scheduling/application 86,0 % |
| Mutaciones | ✅ detectadas | Mantener la anticipación mínima en la lista del personal; un PROFESSIONAL viendo las citas de otras agendas; código restaurado |
| `TestStaffViewsOnPostgres` (`-tags integration`, PostgreSQL real) | ✅ | Consultas SQL nuevas: filtros, orden, cursor; hora del personal "ahora" agendable; `excludeAppointmentId`; 403 a otro profesional |
| `TestContractAgenda` (contrato OpenAPI) | ✅ | Las 2 operaciones con respuestas reales validadas contra el esquema (200, 400, 403, 404); cobertura de `scheduling/interfaces/http` 83,9 % |
| Suite de seguridad | ✅ | Aislamiento en **44 operaciones** (28 con ids ajenos); primero falló (400 antes de autorizar) y se corrigió |
| `task check` | ✅ | build, vet, golangci-lint (0), pruebas; govulncheck sin vulnerabilidades (reintento por DNS); `sqlc vet/diff` al día; shellcheck del E2E |
| Despliegue y E2E en QA (PR #76) | ✅ **241/241** | CI 6/6; `release-qa` ✅ (`499e64f`). Con `APP_URL=https://booka-app-qa.duckdns.org`: enlaces de verificación y recuperación al portal; horas del personal = públicas para el martes (23), mover la cita de las 10:00 libera su hora, barbero con las horas del dueño → 403, sin sesión → 401; citas del cliente: 2 próximas (la primera pendiente), el barbero las ve (su agenda), 0 pasadas, cliente desconocido → 404. Una primera corrida falló en cascada (178) porque el túnel a Mailpit estaba cerrado; repetida con túnel propio |

## 2026-11-27 · Operación QA · `APPOINTMENT_RESCHEDULE_URL`

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `curl https://booka-app-qa.duckdns.org/citas/reagendar` | ✅ 200 | `Referrer-Policy: no-referrer`; una ruta inexistente da 404 (la página existe en el portal) |
| `docker compose up -d api worker` tras editar el `.env` | ❌ sin efecto | Los contenedores no se recrearon: el compose no pasaba la variable (hallazgo) |
| `docker compose config` con el compose corregido | ✅ | `APPOINTMENT_RESCHEDULE_URL` y `APP_DOWNLOAD_URL` llegan a `api` y `worker` (y vacías si faltan) |
| `task shellcheck` | ✅ | Con la nueva comprobación `RESCHEDULE_URL` del E2E |
| Despliegue y E2E en QA (PR #78) | ✅ **239 PASS, 0 fallos** | CI 6/6; `release-qa` ✅ (`efd1ea6`); el worker y la API tienen la variable (`docker inspect`). Nueva comprobación «Reagendar points to APPOINTMENT_RESCHEDULE_URL» ✅. Omitidos: alta de negocios por el admin (sin `ADMIN_EMAIL`) y límite de peticiones (`SKIP_RATE_LIMIT=1`) |

## 2026-11-27 · Fuera de fase · P-11 — Un dueño puede crear más negocios

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestSelfServiceOnboarding`, `TestSelfServiceIdempotency` | ✅ | Segundo negocio con su demo; el tercero → `ErrOwnedBusinessLimit` sin persistir nada; misma clave → el mismo negocio; otros datos → `ErrIdempotencyKeyReused`; la clave es por cuenta; una petición fallida no guarda la clave; claves inválidas |
| `TestOwnedBusinessLimitConfig` | ✅ | Por defecto 2; 5 aceptado; 0, -1 y texto rechazados |
| `TestContractBusinessEndpoints` (contrato OpenAPI) | ✅ | 201 con `Idempotency-Key`, repetición 201 con el mismo id, 422 `IDEMPOTENCY_KEY_REUSED`, 400 clave inválida, 409 `OWNED_BUSINESS_LIMIT` |
| Suite de seguridad (`TestSelfServiceOnboardingRules`) | ✅ | Segundo negocio 201; tercero 409 sin persistir |
| `TestSelfServiceOnboardingOnPostgres` (`-tags integration`) | ✅ | 3 peticiones simultáneas → 2 negocios con demo y 1 rechazo; 3 simultáneas con la misma clave → 1 negocio; otros datos con la clave → rechazo |
| Mutaciones | ✅ detectadas | Tope `>` en vez de `>=` (falla la suite de seguridad); repetición sin comparar los datos (falla `TestSelfServiceIdempotency`); código restaurado |
| `task check` | ✅ | build, vet, golangci-lint (0), pruebas; govulncheck sin vulnerabilidades (reintento por DNS); `sqlc vet/diff` al día; shellcheck del E2E |
| E2E en QA | ✅ | Ver P-12: mismo PR #80 |

## 2026-11-27 · Fuera de fase · P-12 — Estadísticas del negocio y consolidadas

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestGrouping`, `TestStatsFigures`, `TestWorkingMinutes` (dominio) | ✅ | Periodos por día, mes y año; cada regla de `StatsFigures` con citas conocidas; por profesional suman el total; ocupación redondeada, máximo 100, nula sin horario; consolidado sin mezclar clientes; horario menos descansos y ausencias; día de cambio de hora (Madrid) |
| `TestStats` (aplicación) | ✅ | Noviembre 2026: 3 citas, 1 cancelada, 1 ausencia del cliente, $45.000, 1 cliente nuevo, 810 min disponibles (ausencia del día 17, perfil creado el 15), ocupación 6 %; por día 30 periodos; filtro por profesional; el barbero solo lo suyo y sin `byProfessional`; 403, 404 antes de validar, rangos (367 días por día, más de 5 años, 366 en año bisiesto), parámetros inválidos; consolidado del dueño y en cero para el barbero |
| `TestContractAgenda` (contrato OpenAPI) | ✅ | Las 2 operaciones con respuestas reales validadas contra el esquema (200, 400 `INVALID_RANGE`, 403, 404); ocupación 17 % con 60 de 360 min |
| Suite de seguridad | ✅ | Aislamiento en **45 operaciones** (28 con ids ajenos): la ruta nueva entra sola desde el OpenAPI |
| `TestStatsOnPostgres` (`-tags integration`) | ✅ | SQL real, horarios y ausencias reales: marzo 2030, $5.000.000, 1 cliente nuevo (el otro vino en febrero), 1260 min disponibles, ocupación 6 %; Juan 540 min por su ausencia; serie diaria de 31 días |
| `task test:integration` | ✅ | Toda la suite de integración (con P-11) |
| Mutaciones | ✅ detectadas | Contar las ausencias del cliente como minutos ocupados; ignorar las ausencias del profesional; el barbero viendo todo el negocio; código restaurado |
| `task check` | ✅ | build, vet, golangci-lint (0), pruebas; govulncheck sin vulnerabilidades (reintento por DNS); `sqlc vet/diff` al día; shellcheck del E2E |
| Despliegue en QA (PR #80) | ✅ | CI 6/6; `release-qa` ✅ (`fef9221`); migraciones 00027 y 00028 aplicadas; `/readyz` listo |
| E2E en QA, 1.ª corrida | ❌ 238 PASS, 10 fallos | Todas las comprobaciones nuevas de P-11 y P-12 pasaron. Fallaron las del bloque de enlaces del correo: el script leyó el correo anterior (la cancelación del paso previo llegó tarde). Verificado en Mailpit que el correo de la reserva traía los tres enlaces y «Reagendar» hacia el portal: falla del script, no del backend |
| E2E en QA, 2.ª corrida (script corregido: espera el correo con los enlaces) | ✅ **248 PASS, 0 fallos** | Con `APP_URL` y `RESCHEDULE_URL`; omitidos el alta por el admin (sin `ADMIN_EMAIL`) y el límite de peticiones (`SKIP_RATE_LIMIT=1`) |

## 2026-11-27 · Fuera de fase · Imágenes del negocio y portafolio de los profesionales

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestProcess`, `TestLocalStore`, `TestFromRequest` (`internal/platform/media`) | ✅ | PNG con transparencia sigue PNG, opaco pasa a JPEG; reducción sin agrandar; orientación EXIF 6 corregida y sin metadatos en la salida; PDF, script, GIF, JPEG truncado y 12001 px rechazados; guardar, servir (tipo, `cross-origin`, `immutable`), claves inválidas y `../` no se sirven; borrar dos veces; lectura del formulario, campo faltante, 5 MiB + 1, cuerpo JSON |
| `TestBusinessImages` | ✅ | Solo el OWNER (PROFESSIONAL 403, ajeno 404) y **sin leer el archivo antes de autorizar**; reemplazar borra el archivo viejo; si la actualización falla no queda archivo; quitar; auditoría |
| `TestPortfolio` | ✅ | El barbero el suyo, no el de otro (403 sin leer el archivo); el dueño cualquiera; leyenda (recorte, 141 caracteres); PDF rechazado; **10 como máximo** y la 11.ª no deja archivo; orden; editar, validar `sortOrder`; borrar quita el archivo; auditoría |
| `TestContractImages` (contrato OpenAPI, HTTP real) | ✅ | Logo 1200×600 → JPEG 512×256 servido `cross-origin`; reemplazo borra el anterior; `logoUrl`/`coverUrl` en la vista pública y la administrada; 403/404 (también con cuerpo JSON en negocio ajeno); PDF y script → 400 `UNSUPPORTED_IMAGE`; JSON → 415; 5 MiB + 10 → 413; portafolio 3000×2000 → 1600×1066; 11.ª → 409 `PORTFOLIO_FULL`; catálogo público con las 10; borrar y `../` |
| `TestImagesOnPostgres` (`-tags integration`) | ✅ | Claves en todas las lecturas (por id, por slug y búsqueda pública); `CHECK` rechaza una clave inválida; **12 subidas simultáneas → 10 guardadas y 2 rechazadas**; orden y borrado |
| Suite de seguridad | ✅ | Aislamiento en las rutas nuevas (las de imágenes entran solas desde el OpenAPI); campos públicos permitidos actualizados (`logoUrl`, `coverUrl`, `portfolio`) |
| Mutaciones | ✅ detectadas | Sin el límite de 10; un barbero subiendo al portafolio de otro; sin borrar el archivo si falla la transacción; código restaurado |
| Imagen Docker | ✅ | `task docker:build`; un volumen nuevo montado en `/data/media` queda a nombre de `65532:65532` (nonroot) |
| `task check` | ✅ | build, vet, golangci-lint (0), pruebas; govulncheck sin vulnerabilidades (reintento por DNS); `sqlc vet/diff` al día; shellcheck del E2E |
| E2E en QA | ⏳ | Tras el merge y el despliegue (requieren autorización): 10 comprobaciones nuevas (logo, servirlo, permisos, PDF disfrazado, portafolio, catálogo, borrar) |

## 2026-11-28 · Fuera de fase · Reseñas y calificaciones (ADR-0021)

| Comando / verificación | Resultado | Observaciones |
|---|---|---|
| `TestCanReview`, `TestAuthorName`, `TestCleanText`, `TestRatingAverage` (dominio) | ✅ | Solo COMPLETED y de la misma cuenta; hasta 30 días exactos (un segundo después no); no asistida, cancelada y sin cuenta rechazadas; «Ana María Gómez» → «Ana M.»; largos en caracteres, no bytes; sin caracteres de control; promedio con un decimal (14/3 → 4,7) |
| `TestReviews`, `TestReviewPages` (casos de uso) | ✅ | Antes de completar → `NOT_REVIEWABLE`; otro cliente → 404 aunque el cuerpo sea inválido; validación; crear y luego cambiar (misma reseña, conserva la respuesta); `MyAppointment` con la reseña y el plazo; públicas y promedios del negocio y del profesional; solo el OWNER responde (barbero 403, ajeno 404 antes de validar); el barbero ve las suyas, otro barbero 403; solo el administrador oculta, con motivo; oculta sale de lo público y de los promedios, el dueño la ve marcada; mostrar de nuevo; pasados 30 días → `REVIEW_WINDOW_CLOSED`; borrar; páginas por cursor |
| `TestPlanReviewRequest`, `TestStaleReviewRequest`, `TestRenderReviewRequest` (notificaciones) | ✅ | Al completar, un correo al cliente con cuenta (sin WhatsApp, nada sin cuenta ni en no asistida); se omite si la cita dejó de estar completada; «Calificar» hacia `APPOINTMENT_REVIEW_URL?appointmentId=…`; sin la URL remite a «Mis citas»; sin políticas; nada al barbero |
| `TestContractReviews` (contrato OpenAPI, HTTP real) | ✅ | Todo el flujo validado contra el documento: 409, 400, 201/200, 404 de otra cuenta, `reviewableUntil`; correo de invitación en cola; lista pública sin id de la cita; `rating` en el detalle público, la búsqueda y el catálogo; personal, respuesta (403/400/404/200), ocultar (403/400/404/200), mostrar, borrar |
| `TestReviewsOnPostgres` (`-tags integration`) | ✅ | Mensaje `APPOINTMENT_REVIEW_REQUEST` con su job (restricción de tipos); **40 envíos simultáneos → 1 reseña creada, 0 errores**; todas las lecturas; promedios; respuesta y ocultamiento en la fila; los `CHECK` rechazan 6 estrellas, respuesta sin fecha, oculta sin autor, comentario de 1001; `UNIQUE` rechaza una segunda reseña; no asistida → `NOT_REVIEWABLE`. Estable con `-count=3` |
| Suite de seguridad | ✅ | Aislamiento en las rutas nuevas (entran solas desde el OpenAPI); campo público `rating` permitido |
| Mutaciones | ✅ detectadas | Otro cliente reseñando; reseñas ocultas en la lista pública; **sin el bloqueo de la cita** (16 reseñas creadas y errores `duplicate key`: hizo falta abrir todas las conexiones del pool antes de soltar los envíos para que de verdad se crucen). Código restaurado |
| `task check` | ✅ | build, vet, golangci-lint (0), pruebas, govulncheck sin vulnerabilidades |
| `task sqlc:check`, `task shellcheck` | ✅ | Código SQLC al día; script E2E sin hallazgos |
| `task test:integration` (completa) | ✅ | Toda la suite con PostgreSQL real |
| E2E en QA | ⏳ | Tras PR, merge y despliegue (requieren autorización): 9 comprobaciones nuevas (pendiente no calificable, cita ajena, `rating` público y del catálogo, listas del personal, permisos de ocultar). El flujo completo necesita una cita completada: lo cubren el contrato y la integración |
