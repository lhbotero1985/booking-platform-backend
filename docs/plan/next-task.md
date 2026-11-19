# Siguiente tarea

## Última tarea completada
T1.6 — River (2026-11-19), en la rama local `feature/t1.6-river` (sin commit: pendiente de
autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Hotfix: `hotfix/*` → `main` → back-merge a `qa`.
Ningún merge, commit, push, PR ni despliegue sin autorización explícita del usuario.
Sin protección de ramas (decisión del usuario, P23).

## Acciones pendientes de autorización

1. Commit, push y PR de `feature/t1.6-river` → `qa`.
2. Merge del PR #6 (`feature/dependabot-qa` → `qa`).

## Próxima tarea: T1.7 — OpenAPI 3.1 base

- **Objetivo:** contrato `api/openapi.yaml` como fuente de verdad (ADR-0011), validado en CI, y
  pruebas que verifiquen que las respuestas reales cumplen el contrato.
- **Archivos:**
  - `api/openapi.yaml` — info, servers (DEV/QA/PROD), `GET /healthz`, `GET /readyz`, esquema
    `Problem` (RFC 9457 + `code`, `requestId`, `errors[]`), cabecera `X-Request-ID`, esquemas de
    seguridad declarados (Bearer opaco y cookie) para fases siguientes.
  - Validación del spec (linter OpenAPI 3.1 vía contenedor, p. ej. Redocly CLI o Spectral) en
    `task openapi:lint`.
  - Validador de respuestas en pruebas (librería con soporte 3.1, p. ej.
    `pb33f/libopenapi-validator`) aplicado a `/healthz`, `/readyz` (200 y 503) y 404.
- **Pruebas:** lint del spec sin errores; pruebas de contrato en verde; una respuesta que no
  cumple el contrato debe hacer fallar la prueba (verificación negativa).
- **Comandos de validación:** `task openapi:lint`, `task check`, `task test:integration`.
- **Criterios de aceptación:** los anteriores en verde y documentados.
- **Bloqueos:** ninguno.
