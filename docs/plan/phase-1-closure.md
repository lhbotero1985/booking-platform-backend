# Cierre de la Fase 1 — Cimientos técnicos

Fecha: 2026-11-19 · Estado: **pendiente de aprobación del usuario**

## 1. Entregables exigidos por el prompt maestro

| Entregable | Estado | Evidencia |
|---|---|---|
| Proyecto Go | ✅ | `go.mod` (Go 1.27.4), `cmd/api`, `cmd/worker` |
| Monolito modular | ✅ (estructura) | `internal/platform`, `internal/app`; reglas `depguard` de pureza del dominio. Los módulos de negocio nacen en Fase 2+ |
| Docker | ✅ | `deploy/docker/Dockerfile` multi-stage, distroless nonroot, 12.4 MB |
| Docker Compose | ✅ | `deploy/compose/docker-compose.dev.yml` (postgres, api, worker) |
| PostgreSQL + PostGIS | ✅ | PostGIS 17-3.5; extensiones postgis, btree_gist, citext |
| pgx | ✅ | `internal/platform/db` (pool, `WithTx`, errores PG) |
| SQLC | ✅ | `sqlc.yaml`, `platformq`, `sqlc diff` en CI |
| Goose | ✅ | `db/migrations` embebidas; `api migrate up\|down\|status` |
| River | ✅ | `internal/platform/jobs`, `00002_river.sql`, `cmd/worker` |
| Variables de entorno | ✅ | `internal/platform/config` (validación agregada, sin filtrar credenciales), `.env.example` |
| Logging | ✅ | `slog` JSON con redacción de claves sensibles |
| Health checks | ✅ | `/healthz` (liveness), `/readyz` (BD + migraciones) |
| Manejo de errores | ✅ | RFC 9457 (`Problem` con `code` y `requestId`), recover sin filtrar internos |
| OpenAPI | ✅ | `api/openapi.yaml` 3.1 + pruebas de spec y de contrato |
| GitHub Actions | ✅ | `ci.yml` y `security.yml`, verdes en PR #9 y en `qa` |

## 2. Pruebas exigidas

| Tipo | Estado | Evidencia (`docs/testing/test-results.md`) |
|---|---|---|
| Unitarias aplicables | ✅ | Todas las capas de plataforma; `-race` en CI sin carreras |
| Integración | ✅ | testcontainers PostGIS: db, migrate, jobs, readyz (local y CI) |
| Migraciones | ✅ | Up → Down → Up de 00001 y 00002 (River) |
| Health checks | ✅ | Unitarias, integración (503→200→503) y E2E en Compose |
| Seguridad básica | ✅ | `tests/security` (T1.9) + gitleaks, gosec, govulncheck, Trivy |

## 3. Checklist de seguridad de la Fase 1

| Control | Verificación |
|---|---|
| Cabeceras de seguridad en toda respuesta (200/404/405/413/503) | `TestSecurityHeadersOnEveryResponse` |
| Sin divulgación de tecnología (`Server`, `X-Powered-By`) | ídem |
| Errores sin trazas, rutas, IPs internas ni credenciales | `TestErrorsDoNotLeakInternals`, `TestRecoverReturnsProblemWithoutInternals` |
| CORS solo lista blanca exacta (sin `*`, `null`, look-alikes, http) | `TestCORSIsAllowlistOnly`, validación de config |
| Límite de cuerpo (Content-Length y chunked) | `TestLimitBody`, `TestBodyLimitStreaming` |
| Límite de cabeceras (431) | `TestOversizedHeadersRejected` |
| Slowloris (`ReadHeaderTimeout`) | `TestSlowHeadersAreCutOff` |
| Inyección en logs vía `X-Request-ID` | `TestRequestIDCannotInjectLogs` |
| Credenciales de BD nunca en errores ni logs | pruebas de config, readyz, E2E |
| JSON estricto (campos desconocidos, datos extra) | `TestDecodeJSONRejectsInvalidInput` |
| SQL solo parametrizado | SQLC + gosec (G201/G202) en CI |
| Secretos fuera de Git | gitleaks (historial completo) local y CI |
| Dependencias sin vulnerabilidades alcanzables | govulncheck local y CI |
| Imagen sin HIGH/CRITICAL corregibles; contenedor nonroot, read-only, sin capabilities | Trivy local y CI; `docker inspect` |
| Cadena de suministro CI | Acciones por SHA, escáneres por digest, permisos mínimos |
| `migrate down` prohibido en QA/PROD | prueba E2E con `APP_ENV=qa` |

## 4. Riesgos y pendientes conocidos (no bloqueantes)

| # | Pendiente | Plan |
|---|---|---|
| 1 | Roles de BD `app_migrator` / `app_runtime` | Fase 3 (infra QA) |
| 2 | Publicación de imagen en GHCR, SBOM, firma | Fase 3 (`release-image.yml`) |
| 3 | Sin protección de ramas (decisión del usuario) | Disciplina + CI; reevaluar |
| 4 | `main` sigue en T1.3; Dependabot lee su config desde `main` | PR `qa` → `main` tras validar en QA (Fase 3) |
| 5 | Entorno local: DNS intermitente, Docker Desktop se detiene | Reintentos; CI como referencia |
| 6 | `pre-commit` no instalado localmente (requiere Python) | Opcional; gitleaks corre en CI |
| 7 | Imágenes de escáneres dentro de `run:` no las actualiza Dependabot | Revisión manual en cada cierre de fase |
| 8 | Cobertura unitaria total 52 % (db 38 % unitario; resto cubierto por integración) | Metas de cobertura por dominio desde Fase 2 |

## 5. Decisión requerida

Aprobar el cierre de la Fase 1 y autorizar el inicio de la **Fase 2 — Autenticación y
autorización** (desglose en `docs/plan/roadmap.md`).
