# Siguiente tarea

## Última tarea completada
T4.4 — Gestión del negocio (2026-11-24), en `feature/t4.4-business-management` (commit y push;
PR y merge pendientes de autorización).

**Fase 4 en curso** (desglose aprobado 2026-11-23 y ajustado 2026-11-24, ADR-0018).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.5 — Miembros del negocio

- `GET /api/v1/business/{businessId}/members` (OWNER, MANAGER: lista con nombre, email, rol).
- `POST …/members` (OWNER: agregar por email un usuario registrado y verificado, rol MANAGER o
  PROFESSIONAL; reutiliza `MemberAdder`).
- `PATCH …/members/{userId}` (cambiar rol) y `DELETE …/members/{userId}` (revocar): solo OWNER;
  no se puede revocar ni degradar al último OWNER; revocación inmediata; auditoría.
- Transferencia de la propiedad (OWNER → otro miembro): proponer si se incluye ahora.
