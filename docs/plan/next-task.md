# Siguiente tarea

## Última tarea completada
T2.7 — Rate limiting y defensa contra fuerza bruta (2026-11-19), en
`feature/t2.7-rate-limiting` (commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.7-rate-limiting` → `qa` y merge si el CI queda en verde.

## Próxima tarea: T2.8 — Auditoría básica

- **Objetivo:** registrar eventos de seguridad en `audit_log` (tabla ya creada en la migración
  00003; consulta `InsertAuditEntry` existente) sin datos sensibles.
- **Diseño propuesto:**
  - Puerto `AuditLog` en la aplicación de identity; escritura **en la misma transacción** que el
    cambio auditado cuando lo hay (cambio/restablecimiento de contraseña, verificación), y en
    transacción propia para eventos sin cambio de datos (login fallido, 429 de backoff).
  - Eventos: `auth.login` (SUCCESS/FAILURE/DENIED por backoff), `auth.logout`,
    `auth.logout_all`, `auth.email_verified`, `auth.password_reset_requested`,
    `auth.password_reset`, `auth.password_changed`, `authz.denied` (403).
  - Campos: actor (si se conoce), IP, `outcome`, `metadata` mínima (p. ej. `clientType`,
    motivo). **Nunca** email en claro de intentos fallidos con cuentas inexistentes (hash), ni
    contraseñas, tokens o user agents completos.
  - Retención y consulta por PLATFORM_ADMIN: se definen en la Fase 12 (solo escritura ahora).
- **Pruebas:** cada evento registrado con el resultado correcto; ausencia de secretos en
  `metadata`; atomicidad con el cambio; integración PostgreSQL.
