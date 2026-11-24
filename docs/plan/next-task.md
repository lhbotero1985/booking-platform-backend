# Siguiente tarea

## Última tarea completada
T4.5 — Miembros del negocio (2026-11-24), en `feature/t4.5-business-members` (commit y push;
PR y merge pendientes de autorización).

**Fase 4 en curso** (desglose aprobado 2026-11-23 y ajustado 2026-11-24, ADR-0018).

## Flujo de ramas (ADR-0017)

`feature/*` (nace de `qa`) → PR → `qa` → despliegue automático a QA → validación. `qa` → `main`
solo al pasar a producción.

## Próxima tarea: T4.6 — Búsqueda pública de negocios

- `GET /api/v1/businesses?cityId=&q=&cursor=` (público): solo negocios `ACTIVE` con suscripción
  habilitada (demo vigente o pagada), paginación por cursor; `GET /api/v1/businesses/{slug}`.
- Sin datos internos (sin teléfono del dueño, sin políticas internas que no aplican al cliente).
- La regla "al menos un servicio activo" (ADR-0018) se agrega cuando exista el catálogo (Fase 5).
