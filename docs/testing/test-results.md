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
