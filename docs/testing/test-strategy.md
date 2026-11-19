# Estrategia de pruebas

Estado: **Propuesta — Fase 0.** Regla: **ninguna prueba se declara exitosa sin haberse
ejecutado; los resultados reales se registran en `test-results.md`.**

## 1. Niveles

| Nivel | Qué cubre | Dónde | Herramientas | Cuándo corre |
|---|---|---|---|---|
| Unitarias | Dominio, VO, invariantes, máquina de estados, `AvailabilityCalculator`, políticas de autorización, casos de uso con fakes | `*_test.go` junto al código | `go test`, tablas de casos, fuzzing (`go test -fuzz`) para parsers/VO | Cada commit (local y CI) |
| Integración | Repositorios SQLC, migraciones up/down, transacciones, River, endpoints HTTP completos, sesiones | `internal/**/infrastructure/*_test.go`, `tests/integration/` | `testcontainers-go` (postgis), `httptest`, build tag `integration` | CI en cada PR; local con Docker |
| Contrato | Respuestas conformes a `api/openapi.yaml` | Dentro de las pruebas de integración HTTP | Validador OpenAPI 3.1 | CI |
| Seguridad | Acceso sin auth, tokens inválidos/expirados/revocados, escalamiento de privilegios, IDOR, cross-tenant, inyección SQL (payloads en todos los parámetros), validación de entradas, webhooks falsificados/repetidos, rate limiting | `tests/security/` | `go test`, matrices rol × endpoint | CI en cada PR |
| Concurrencia | Doble reserva, cancelación/reprogramación concurrente, confirmaciones repetidas, idempotency keys | `tests/concurrency/` | Goroutines + `sync.WaitGroup` contra PG real, `-race` | CI en cada PR (desde Fase 6) |
| Estáticas | Formato, lint, SAST, vulnerabilidades, secretos | — | `gofmt`, `go vet`, `golangci-lint` (gosec, staticcheck, depguard…), `govulncheck`, gitleaks, Trivy | Pre-commit + CI |
| E2E frontend | Flujos de usuario | Repos frontend | Playwright (web), Maestro/Detox (móvil) | Fases 8–12 |
| Carga controlada | Disponibilidad y reserva bajo concurrencia realista | QA | k6 | Fase 12 |
| Smoke | Health, login sintético, versión | QA/PROD tras despliegue | Script en pipeline | Cada despliegue |

## 2. Matriz de seguridad obligatoria por endpoint

Para cada endpoint protegido, pruebas generadas por tabla:

| Caso | Esperado |
|---|---|
| Sin token | 401 |
| Token malformado / inexistente | 401 |
| Sesión expirada o revocada | 401 |
| Usuario suspendido | 401 |
| Rol insuficiente en el mismo negocio | 403 |
| Recurso de otro negocio | 404 |
| ID inexistente | 404 |
| Body inválido / campos desconocidos / tamaño excesivo | 400 / 413 |

## 3. Datos de prueba

- Solo datos sintéticos (builders/fixtures en Go; seed para DEV y QA).
- Cada prueba de integración crea sus propios datos; sin dependencias de orden.
- Reloj inyectado (`Clock`) para pruebas deterministas de tiempo y DST.

## 4. Metas de cobertura (orientativas, no un fin en sí)

- Dominio de Scheduling y políticas de autorización: ≥ 90 %.
- Resto del dominio y aplicación: ≥ 80 %.
- Infraestructura: cubierta por integración.

## 5. Comandos (desde Fase 1)

```
task test              # go test ./... (unitarias)
task test:race         # go test -race ./...
task test:integration  # go test -tags=integration ./...  (requiere Docker)
task lint              # golangci-lint run
task vuln              # govulncheck ./...
task secrets           # gitleaks
task check             # todo lo anterior
```

## 6. Criterios de salida por fase

Una fase no se aprueba si: hay pruebas fallando, hay hallazgos HIGH/CRITICAL de seguridad sin
plan aprobado, o (Fase 6 en adelante) existe cualquier riesgo conocido de doble reserva.
