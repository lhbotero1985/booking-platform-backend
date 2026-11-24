# Siguiente tarea

## Última tarea completada
T4.3 — Alta de negocios por el administrador (2026-11-24), en
`feature/t4.3-business-onboarding` (commit y push; PR y merge pendientes de autorización). Tras
el despliegue: E2E en QA con `ADMIN_EMAIL`/`ADMIN_PASSWORD`.

**Fase 4 en curso** (desglose aprobado 2026-11-23, ver `roadmap.md`).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.4 — Gestión del negocio por su OWNER/MANAGER

- `GET /api/v1/business/{businessId}` (staff del negocio), `PATCH` de datos, ubicación,
  política de reservas y recordatorios (OWNER; MANAGER solo lo operativo si se decide), otro
  negocio → 404, auditoría `business.updated` con campos cambiados.
- Decisión a proponer: qué puede editar MANAGER (security.md: configuración = OWNER).
