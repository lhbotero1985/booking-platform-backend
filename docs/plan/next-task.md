# Siguiente tarea

## Última tarea completada
T2.4 — Login, sesiones y middleware de autenticación (2026-11-19), en
`feature/t2.4-login-sessions` (commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.4-login-sessions` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t2.3-register-verify-login` (local y remota).

## Próxima tarea: T2.5 — Recuperación de contraseña

- **Objetivo:** restablecer la contraseña por email sin revelar cuentas (ADR-0006).
- **Diseño (reutiliza T2.3):**
  - `POST /api/v1/auth/password/forgot {email}` → siempre 202; si la cuenta existe y está
    activa, encola job; el worker genera token `bp_pr_` (solo se guarda el hash, válido 30 min,
    un solo uso, invalida los anteriores) y envía el enlace `PUBLIC_APP_URL/reset-password?token=`.
    Coalescido a 1 por minuto por cuenta.
  - `POST /api/v1/auth/password/reset {token, newPassword}` → valida la política, cambia el
    hash, **revoca todas las sesiones** y consume el token; tokens inválidos → `INVALID_TOKEN`.
  - Opcional: `POST /api/v1/me/password {currentPassword, newPassword}` (cambio autenticado,
    revoca las demás sesiones).
  - Email de aviso "tu contraseña cambió" al titular.
- **Archivos:** reutiliza la tabla `password_reset_tokens` (00003), consultas nuevas, casos de
  uso, job, handlers, OpenAPI.
- **Pruebas:** anti-enumeración (respuesta idéntica), un solo uso, expiración, rotación,
  revocación de sesiones tras el reset, política de contraseña, contrato, integración, E2E con
  Mailpit.
