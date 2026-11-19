# Siguiente tarea

## Última tarea completada
T2.5 — Recuperación y cambio de contraseña (2026-11-19), en `feature/t2.5-password-reset`
(commit y push hechos; PR y merge pendientes de autorización).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue y validación QA → PR `qa` → `main` →
PROD con el mismo artefacto. Commit y push en `feature/*`: autorización permanente (sin
preguntar). PR, merge, borrado de ramas y despliegues: autorización explícita por tarea.

## Pendiente de autorización

1. PR `feature/t2.5-password-reset` → `qa` y merge si el CI queda en verde.
2. Borrar la rama fusionada `feature/t2.4-login-sessions` (local y remota).

## Próxima tarea: T2.6 — RBAC y autorización por recurso

- **Objetivo:** base de autorización reutilizable por todos los módulos (ADR-0007).
- **Diseño:**
  - `identity/contract`: puerto `Memberships` (roles activos del principal por negocio,
    consultados por request, sin caché en la sesión) y funciones de política puras:
    `RequireBusinessRole(principal, businessID, roles...)` → `nil` | `ErrForbidden` (mismo
    negocio, rol insuficiente) | `ErrNotFound` (sin membresía: el recurso "no existe", patrón
    anti-IDOR) ; `RequirePlatformAdmin`.
  - Middleware/helper HTTP para mapear a 403 `FORBIDDEN` / 404 `NOT_FOUND`.
  - Endpoint mínimo para probar el patrón de extremo a extremo sin adelantar la Fase 4:
    `GET /api/v1/me/memberships` (lista negocios y roles del usuario).
  - Matriz de políticas documentada (OWNER, MANAGER, PROFESSIONAL, CUSTOMER, PLATFORM_ADMIN).
- **Pruebas:** tabla rol × acción × mismo/otro negocio; membresía revocada pierde acceso en la
  siguiente petición; cross-tenant siempre 404; contrato; integración.
