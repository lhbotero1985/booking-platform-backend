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
| Despliegue y E2E en QA | ⏳ | Tras el merge (E2E ampliado con 9 comprobaciones de catálogo) |
