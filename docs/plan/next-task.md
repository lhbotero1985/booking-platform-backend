# Siguiente tarea

## Última tarea completada
T1.1 — Toolchain y esqueleto del proyecto (2026-11-18).

## Próxima tarea: T1.2 — Plataforma HTTP base

- **Objetivo:** servidor HTTP funcional con configuración validada, logging estructurado,
  errores RFC 9457 y middleware base. Sin base de datos todavía.
- **Archivos:**
  - `internal/platform/config/` — carga y validación de variables de entorno (`APP_ENV`,
    `HTTP_ADDR`, `LOG_LEVEL`, `CORS_ALLOWED_ORIGINS`, timeouts).
  - `internal/platform/logging/` — `slog` JSON, nivel configurable, redacción de campos sensibles.
  - `internal/platform/httpx/` — router stdlib, `problem.go` (RFC 9457), middleware:
    request-id, recover, access log, límite de body (1 MiB), CORS por lista blanca, cabeceras de
    seguridad.
  - `cmd/api/main.go` — servidor con timeouts (`ReadHeaderTimeout`, etc.) y apagado ordenado
    (SIGINT/SIGTERM).
  - `GET /healthz` (sin BD; `/readyz` llega en T1.4).
- **Dependencias:** ninguna externa nueva prevista (stdlib).
- **Pruebas:** unitarias de config (válida/inválida), cada middleware (`httptest`), problem+json,
  recover sin filtrar stack trace, CORS con origen permitido/denegado, body excesivo → 413.
- **Comandos de validación:** `task check`.
- **Criterios de aceptación:**
  - Config inválida o faltante → el proceso termina con error claro al iniciar.
  - `GET /healthz` → 200 JSON; ruta inexistente → 404 `application/problem+json`.
  - Pánico en handler → 500 problem+json sin detalles internos; queda en el log con request-id.
  - Lint, vet, vuln y pruebas en verde.
- **Bloqueos:** ninguno.

## Pendientes del usuario (no bloquean T1.2)

- Usuario u organización de GitHub (para la ruta del módulo Go) y URL del repo privado vacío
  `booking-platform-backend` + autorización de push.
- Iniciar Docker Desktop (requerido en T1.3/T1.4 y para gitleaks).
- Autorizar instalación de `pre-commit` (Python) — opcional.
