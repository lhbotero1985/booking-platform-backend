# Siguiente tarea

## Última tarea completada
T4.3b — Alta autoservicio con demo de 7 días (2026-11-24), en
`feature/t4.3b-self-service-trial` (commit y push; PR y merge pendientes de autorización).

**Fase 4 en curso** (desglose aprobado 2026-11-23 y ajustado 2026-11-24, ADR-0018).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.4 — Gestión del negocio

- `GET /api/v1/business/{businessId}` (OWNER, MANAGER, PROFESSIONAL y PLATFORM_ADMIN), con la
  suscripción del negocio (estado y fin de la demo) para el dueño.
- `PATCH` de datos, ubicación, política de reservas y recordatorios: OWNER y PLATFORM_ADMIN;
  MANAGER solo lectura (decisión 2026-11-24); otro negocio → 404; auditoría `business.updated`
  con campos cambiados.
- Autorizador: `PLATFORM_ADMIN` pasa `RequireBusinessRole` en cualquier negocio (ADR-0018),
  auditado con el administrador como actor.
