# Siguiente tarea

## Última tarea completada
T1.7 — Contrato OpenAPI 3.1 (2026-11-19), en la rama local `feature/t1.7-openapi` (sin commit:
pendiente de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Hotfix: `hotfix/*` → `main` → back-merge a `qa`.
Ningún merge, commit, push, PR ni despliegue sin autorización explícita del usuario.

## Acciones pendientes de autorización

1. Commit, push, PR y merge de `feature/t1.7-openapi` → `qa`.
2. Borrar la rama ya fusionada `feature/t1.6-river` (local y remota).

## Próxima tarea: T1.8 — GitHub Actions (CI y seguridad)

- **Objetivo:** pipeline obligatorio en cada PR hacia `qa`/`main` y en cada push a esas ramas.
- **Archivos:** `.github/workflows/ci.yml`, `.github/workflows/security.yml` (reemplaza a
  `secret-scan.yml`), `.github/dependabot.yml` sin cambios.
- **Jobs previstos (Ubuntu, Go 1.27.4, acciones fijadas por SHA, `permissions: contents: read`):**
  - `build-test`: `go build`, `go vet`, `go test -race` (primera ejecución real del detector de
    carreras), cobertura como artefacto.
  - `integration`: `go test -tags=integration` (testcontainers con el Docker del runner).
  - `lint`: golangci-lint (versión fijada), `sqlc diff`/`sqlc vet`, `task openapi:lint`.
  - `security`: gitleaks (historial completo), govulncheck, Trivy `config` y build + Trivy
    `image` (HIGH/CRITICAL con fix → falla).
- **Pruebas:** el pipeline debe pasar en el PR; verificación negativa en una rama temporal
  (p. ej. un fallo de lint) para comprobar que bloquea — solo con autorización.
- **Criterios de aceptación:** todos los jobs en verde en el PR de T1.8; tiempos razonables
  (< 10 min); sin secretos requeridos.
- **Bloqueos:** ninguno.
